package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-redis/redis/v8"
	"golang.org/x/net/websocket"
	"net/http"
	"sync"
)

var AnyCablePusherConfig struct {
	PusherCommonConfig
	RedisAddr string
	HTTPAddr  string
	Backend   string
}

type AnyCablePusherAdapter struct {
	conn      *websocket.Conn
	socketID  string
	connected bool

	redis *redis.Client
	mu    sync.Mutex

	muPending sync.Mutex
	pending   []*serverSentMsg // We need it to store the originator Result to send it later
}

func (apa *AnyCablePusherAdapter) Startup() error {
	if !channelNameRegexp.MatchString(AnyCablePusherConfig.Channel) {
		return fmt.Errorf("invalid channel name %q", AnyCablePusherConfig.Channel)
	}
	if AnyCablePusherConfig.Backend == "redis" {
		apa.redis = redis.NewClient(&redis.Options{Addr: AnyCablePusherConfig.RedisAddr})
	}

	if err := apa.EnsureConnected(context.Background()); err != nil {
		return err
	}

	return nil
}

func (apa *AnyCablePusherAdapter) SendEcho(_ *Payload) error { return nil }

func (apa *AnyCablePusherAdapter) SendBroadcast(payload *Payload) error {
	msgBody, _ := json.Marshal(map[string]interface{}{
		"action":           "broadcast",
		"sender_socket_id": apa.socketID,
		"payload":          payloadTojsonPayload(payload),
	})

	envelope, _ := json.Marshal(map[string]interface{}{
		"event":   ClientBroadcastEvent,
		"channel": AnyCablePusherConfig.Channel,
		"data":    string(msgBody),
	})

	switch AnyCablePusherConfig.Backend {
	case "http":
		return apa.publishViaHTTP(envelope)
	case "redis":
		return apa.publishViaRedis(envelope)
	default:
		return fmt.Errorf("unknown AnyCable backend: %s", AnyCablePusherConfig.Backend)
	}
}

func (apa *AnyCablePusherAdapter) publishViaRedis(envelope []byte) error {
	pub := map[string]interface{}{
		"stream": AnyCablePusherConfig.Channel,
		"data":   string(envelope),
	}
	raw, _ := json.Marshal(pub)
	return apa.redis.Publish(context.Background(), "__anycable__", raw).Err()
}

func (apa *AnyCablePusherAdapter) publishViaHTTP(envelope []byte) error {
	pub := map[string]interface{}{
		"stream": AnyCablePusherConfig.Channel,
		"data":   string(envelope),
	}
	raw, _ := json.Marshal(pub)

	req, _ := http.NewRequest("POST", AnyCablePusherConfig.HTTPAddr,
		bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("broadcast failed: %s", resp.Status)
	}
	return nil
}

func (apa *AnyCablePusherAdapter) Receive() (*serverSentMsg, error) {
	apa.muPending.Lock()
	if n := len(apa.pending); n > 0 {
		m := apa.pending[0]
		apa.pending = apa.pending[1:]
		apa.muPending.Unlock()
		return m, nil
	}
	apa.muPending.Unlock()

	for {
		msg, err := receiveIgnoringPing(apa.conn)
		if err != nil {
			return nil, err
		}

		message, err := parseBroadcast(msg, apa.socketID)
		if err != nil {
			return nil, err
		}
		if message == nil {
			continue
		}

		if message.Type == MsgServerBroadcastResult {
			apa.muPending.Lock()
			apa.pending = append(apa.pending, message)
			apa.muPending.Unlock()

			return &serverSentMsg{Type: MsgServerBroadcast, Payload: message.Payload}, nil
		}

		return message, nil
	}
}

func (apa *AnyCablePusherAdapter) EnsureConnected(ctx context.Context) error {
	apa.mu.Lock()
	defer apa.mu.Unlock()

	if apa.connected {
		return nil
	}

	res := make(chan error, 1)
	go func() {
		sockID, err := waitConnectionEstablished(apa.conn)
		if err != nil {
			res <- err
			return
		}
		apa.socketID = sockID

		subscribe := map[string]interface{}{
			"event": SubscribeType,
			"data": map[string]string{
				"channel": AnyCablePusherConfig.Channel,
			},
		}

		if err := websocket.JSON.Send(apa.conn, subscribe); err != nil {
			res <- err
			return
		}

		if err := waitSubscriptionSucceeded(apa.conn, PusherConfig.Channel); err != nil {
			res <- err
			return
		}
		res <- nil
	}()

	select {
	case <-ctx.Done():
		return errors.New("connection timeout exceeded")
	case err := <-res:
		return err
	}
}
