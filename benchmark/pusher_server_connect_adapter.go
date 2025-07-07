package benchmark

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

var PusherConnectConfig struct {
	Channel string
}

type PusherServerConnectAdapter struct {
	conn      *websocket.Conn
	connected bool
	mu        sync.Mutex
	socketId  string
	initTime  time.Time
	socketID  string

	// skip Unsubscribe for AnyCable
	skipUnsubscribe bool
}

func (psca *PusherServerConnectAdapter) Startup() error {
	if !channelNameRegexp.MatchString(PusherConnectConfig.Channel) {
		return fmt.Errorf("invalid channel name %q", PusherConnectConfig.Channel)
	}
	psca.connected = false
	return nil
}

func (psca *PusherServerConnectAdapter) Connected(ts time.Time) error {
	psca.initTime = ts
	psca.connected = false
	return nil
}

func (psca *PusherServerConnectAdapter) Receive() (*serverSentMsg, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
	defer cancel()
	if err := psca.EnsureConnected(ctx); err != nil {
		return nil, err
	}
	return &serverSentMsg{Type: MsgServerEcho, Payload: &Payload{SendTime: psca.initTime}}, nil
}

func (psca *PusherServerConnectAdapter) SendEcho(payload *Payload) error      { return nil }
func (psca *PusherServerConnectAdapter) SendBroadcast(payload *Payload) error { return nil }

func (psca *PusherServerConnectAdapter) EnsureConnected(ctx context.Context) error {
	psca.mu.Lock()
	defer psca.mu.Unlock()

	if psca.connected {
		return nil
	}

	res := make(chan error, 1)
	go func() {
		sockID, err := waitConnectionEstablished(psca.conn)
		if err != nil {
			res <- err
			return
		}
		psca.socketID = sockID

		sub := map[string]interface{}{"event": SubscribeType, "data": map[string]string{"channel": PusherConnectConfig.Channel}}
		if err := websocket.JSON.Send(psca.conn, sub); err != nil {
			res <- err
			return
		}

		for {
			msg, err := receiveIgnoringPing(psca.conn)
			if err != nil {
				res <- err
				return
			}
			if (msg.Event == InternalSubscriptionSucceededType) && msg.Channel == PusherConnectConfig.Channel {
				psca.connected = true
				if !psca.skipUnsubscribe {
					_ = websocket.JSON.Send(psca.conn, map[string]interface{}{"event": UnsubscribeType, "data": map[string]string{"channel": PusherConnectConfig.Channel}})
				}
				res <- nil
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
		return errors.New("connection timeout exceeded")
	case err := <-res:
		return err
	}
}
