package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

var CableConfig struct {
	Channel  string
	Encoding string
}

type ActionCableServerAdapter struct {
	conn      *websocket.Conn
	connected bool
	mu        sync.Mutex
	codec     websocket.Codec
}

type acsaMsg struct {
	Type       string      `json:"type,omitempty"`
	Command    string      `json:"command,omitempty"`
	Identifier string      `json:"identifier,omitempty"`
	Data       string      `json:"data,omitempty"`
	Message    interface{} `json:"message,omitempty"`
}

func (acsa *ActionCableServerAdapter) Startup() error {
	acsa.connected = false

	if CableConfig.Encoding == "msgpack" {
		acsa.codec = MsgPackCodec
	} else if CableConfig.Encoding == "protobuf" {
		acsa.codec = ProtoBufCodec
	} else {
		acsa.codec = websocket.JSON
	}

	return nil
}

func (acsa *ActionCableServerAdapter) EnsureConnected(ctx context.Context) error {
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

		acsa.connected = true
		resChan <- nil
	}()

	select {
	case <-ctx.Done():
		return errors.New("Connection timeout exceeded")
	case err := <-resChan:
		if err != nil {
			acsa.connected = true
		}
		return err
	}
}

func (acsa *ActionCableServerAdapter) SendEcho(payload *Payload) error {
	if !acsa.connected {
		ctx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
		defer cancel()

		err := acsa.EnsureConnected(ctx)

		if err != nil {
			return err
		}
	}

	data, err := json.Marshal(map[string]interface{}{"action": "echo", "payload": payloadTojsonPayload(payload)})
	if err != nil {
		return err
	}

	return acsa.codec.Send(acsa.conn, &acsaMsg{
		Command:    "message",
		Identifier: CableConfig.Channel,
		Data:       string(data),
	})
}

func (acsa *ActionCableServerAdapter) SendBroadcast(payload *Payload) error {
	if !acsa.connected {
		ctx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
		defer cancel()

		err := acsa.EnsureConnected(ctx)

		if err != nil {
			return err
		}
	}

	data, err := json.Marshal(map[string]interface{}{"action": "broadcast", "payload": payloadTojsonPayload(payload)})
	if err != nil {
		return err
	}

	return acsa.codec.Send(acsa.conn, &acsaMsg{
		Command:    "message",
		Identifier: CableConfig.Channel,
		Data:       string(data),
	})
}

func (acsa *ActionCableServerAdapter) Receive() (*serverSentMsg, error) {
	if !acsa.connected {
		ctx, cancel := context.WithTimeout(context.Background(), ConnectionTimeout)
		defer cancel()

		err := acsa.EnsureConnected(ctx)

		if err != nil {
			return nil, err
		}
	}

	msg, err := acsa.receiveIgnoringPing()
	if err != nil {
		return nil, err
	}

	if msg.Message == nil {
		panic(fmt.Errorf("Message is nil for %v", msg))
	}

	message := msg.Message.(map[string]interface{})
	payloadMap := message["payload"].(map[string]interface{})

	payload := &Payload{}
	unixNanosecond, err := strconv.ParseInt(payloadMap["sendTime"].(string), 10, 64)
	if err != nil {
		return nil, err
	}
	payload.SendTime = time.Unix(0, unixNanosecond)

	if padding, ok := payloadMap["padding"]; ok {
		paddingJson, err := json.Marshal(padding)
		if err != nil {
			return nil, err
		}
		payload.Padding = paddingJson
	}

	msgType, err := ParseMessageType(message["action"].(string))
	if err != nil {
		return nil, err
	}

	return &serverSentMsg{Type: msgType, Payload: payload}, nil
}

func (acsa *ActionCableServerAdapter) receiveIgnoringPing() (*acsaMsg, error) {
	for {
		var msg acsaMsg
		err := acsa.codec.Receive(acsa.conn, &msg)
		if err != nil {
			return nil, err
		}

		if msg.Type == "ping" || msg.Type == "confirm_subscription" {
			continue
		}

		if msg.Type == "reject_subscription" {
			return nil, errors.New("Subscription rejected")
		}

		return &msg, nil
	}
}
