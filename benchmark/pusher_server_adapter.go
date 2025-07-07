package benchmark

import (
	"context"
	"errors"
	"fmt"
	pusher "github.com/pusher/pusher-http-go/v5"
	"golang.org/x/net/websocket"
	"sync"
)

var PusherConfig struct {
	PusherCommonConfig
	Host string
	Port string
}

type PusherServerAdapter struct {
	conn      *websocket.Conn
	socketID  string
	connected bool

	httpClient *pusher.Client

	mu        sync.Mutex
	muPending sync.Mutex
	pending   []*serverSentMsg // We need it to store the originator Result to send it later
}

func (psa *PusherServerAdapter) Startup() error {
	if !channelNameRegexp.MatchString(PusherConfig.Channel) {
		return fmt.Errorf("invalid channel name %q", PusherConfig.Channel)
	}

	// We use the HTTP API instead of "client‑events" for two reasons:
	// 1. A Pusher client event is not delivered back to its sender, therefore the originator will never see its own message and the
	// benchmark will not be able to calculate RTT
	// https://pusher.com/docs/channels/using_channels/events/#triggering-client-events
	// Client events are not delivered to the originator of the event
	//
	// 2. Because of the first reason, we cannot know
	// when to emulate a MsgServerBroadcastResult.
	//
	// The HTTP trigger has no this limitation: every connection (including the originator) receives the event
	psa.httpClient = &pusher.Client{
		AppID:   PusherConfig.AppID,
		Key:     PusherConfig.AppKey,
		Secret:  PusherConfig.AppSecret,
		Cluster: "",
		Secure:  false,
		Host:    fmt.Sprintf("%s:%s", PusherConfig.Host, PusherConfig.Port),
	}

	return nil
}

func (a *PusherServerAdapter) SendEcho(_ *Payload) error { return nil }

func (psa *PusherServerAdapter) SendBroadcast(payload *Payload) error {
	if !psa.connected {
		ctx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
		defer cancel()

		if err := psa.EnsureConnected(ctx); err != nil {
			return err
		}
	}

	body := map[string]interface{}{
		"action":           "broadcast",
		"sender_socket_id": psa.socketID,
		"payload":          payloadTojsonPayload(payload),
	}

	return psa.httpClient.Trigger(PusherConfig.Channel, ClientBroadcastEvent, body)
}

func (psa *PusherServerAdapter) Receive() (*serverSentMsg, error) {
	if !psa.connected {
		if err := psa.EnsureConnected(context.Background()); err != nil {
			return nil, err
		}
	}

	psa.muPending.Lock()
	if n := len(psa.pending); n > 0 {
		m := psa.pending[0]
		psa.pending = psa.pending[1:]
		psa.muPending.Unlock()
		return m, nil
	}
	psa.muPending.Unlock()

	for {
		msg, err := receiveIgnoringPing(psa.conn)
		if err != nil {
			return nil, err
		}

		message, err := parseBroadcast(msg, psa.socketID)
		if err != nil {
			return nil, err
		}
		if message == nil {
			continue
		}

		if message.Type == MsgServerBroadcastResult {
			psa.muPending.Lock()
			psa.pending = append(psa.pending, message)
			psa.muPending.Unlock()

			return &serverSentMsg{Type: MsgServerBroadcast, Payload: message.Payload}, nil
		}

		return message, nil
	}
}

func (psa *PusherServerAdapter) EnsureConnected(ctx context.Context) error {
	psa.mu.Lock()
	defer psa.mu.Unlock()

	if psa.connected {
		return nil
	}

	res := make(chan error, 1)
	go func() {
		sockID, err := waitConnectionEstablished(psa.conn)
		if err != nil {
			res <- err
			return
		}
		psa.socketID = sockID

		sig := signChannel(psa.socketID, PusherConfig.Channel, PusherConfig.AppSecret)
		auth := PusherConfig.AppKey + ":" + sig

		subscribe := map[string]interface{}{
			"event": SubscribeType,
			"data": map[string]string{
				"channel": PusherConfig.Channel,
				"auth":    auth,
			},
		}

		if err := websocket.JSON.Send(psa.conn, subscribe); err != nil {
			res <- err
			return
		}

		if err := waitSubscriptionSucceeded(psa.conn, PusherConfig.Channel); err != nil {
			res <- err
			return
		}
		psa.connected = true
		res <- nil
	}()

	select {
	case <-ctx.Done():
		return errors.New("connection timeout exceeded")
	case err := <-res:
		return err
	}
}
