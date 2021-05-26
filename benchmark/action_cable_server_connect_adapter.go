package benchmark

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

type ActionCableServerConnectAdapter struct {
	conn      *websocket.Conn
	initTime  time.Time
	connected bool
	mu        sync.Mutex
	codec     websocket.Codec
}

func (acsa *ActionCableServerConnectAdapter) Startup() error {
	if CableConfig.Encoding == "msgpack" {
		acsa.codec = MsgPackCodec
	} else if CableConfig.Encoding == "protobuf" {
		acsa.codec = ProtoBufCodec
	} else {
		acsa.codec = websocket.JSON
	}

	return nil
}

func (acsa *ActionCableServerConnectAdapter) Connected(ts time.Time) error {
	acsa.initTime = ts
	acsa.connected = false
	return nil
}

func (acsa *ActionCableServerConnectAdapter) Receive() (*serverSentMsg, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
	defer cancel()

	err := acsa.EnsureConnected(ctx)

	if err != nil {
		return nil, err
	}

	payload := &Payload{}

	payload.SendTime = acsa.initTime

	// use echo type to collect results
	return &serverSentMsg{Type: MsgServerEcho, Payload: payload}, nil
}

func (acsa *ActionCableServerConnectAdapter) SendEcho(payload *Payload) error {
	return nil
}

func (acsa *ActionCableServerConnectAdapter) SendBroadcast(payload *Payload) error {
	return nil
}

func (acsa *ActionCableServerConnectAdapter) receiveIgnoringPing() (*acsaMsg, error) {
	for {
		var msg acsaMsg
		err := acsa.codec.Receive(acsa.conn, &msg)
		if err != nil {
			return nil, err
		}

		if msg.Type == "ping" {
			continue
		}

		return &msg, nil
	}
}

func (acsa *ActionCableServerConnectAdapter) EnsureConnected(ctx context.Context) error {
	acsa.mu.Lock()
	defer acsa.mu.Unlock()

	if acsa.connected {
		return nil
	}

	resChan := make(chan error)

	go func() {
		welcomeMsg, err := acsa.receiveIgnoringPing()
		if err != nil {
			resChan <- err
			return
		}
		if welcomeMsg.Type != "welcome" {
			resChan <- fmt.Errorf("expected welcome msg, got %v", welcomeMsg)
			return
		}

		err = acsa.codec.Send(acsa.conn, &acsaMsg{
			Command:    "subscribe",
			Identifier: CableConfig.Channel,
		})
		if err != nil {
			resChan <- err
			return
		}

		confirmMsg, err := acsa.receiveIgnoringPing()
		if err != nil {
			resChan <- err
			return
		}

		if confirmMsg.Type != "confirm_subscription" {
			resChan <- fmt.Errorf("expected confirm msg, got %v", confirmMsg)
			return
		}

		resChan <- nil
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("Connection timeout exceeded: started at %s, now %s", acsa.initTime.Format(time.RFC3339), time.Now().Format(time.RFC3339))
	case err := <-resChan:
		if err != nil {
			acsa.connected = true
		}
		return err
	}
}
