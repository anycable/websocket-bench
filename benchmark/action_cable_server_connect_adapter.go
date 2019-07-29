package benchmark

import (
	"fmt"
	"time"

	"golang.org/x/net/websocket"
)

type ActionCableServerConnectAdapter struct {
	conn     *websocket.Conn
	initTime time.Time
}

func (acsa *ActionCableServerConnectAdapter) Startup() error {
	return nil
}

func (acsa *ActionCableServerConnectAdapter) Connected(ts time.Time) error {
	acsa.initTime = ts
	return nil
}

func (acsa *ActionCableServerConnectAdapter) Receive() (*serverSentMsg, error) {
	welcomeMsg, err := acsa.receiveIgnoringPing()
	if err != nil {
		return nil, err
	}
	if welcomeMsg.Type != "welcome" {
		return nil, fmt.Errorf("expected welcome msg, got %v", welcomeMsg)
	}

	err = websocket.JSON.Send(acsa.conn, &acsaMsg{
		Command:    "subscribe",
		Identifier: CableConfig.Channel,
	})
	if err != nil {
		return nil, err
	}

	confirmMsg, err := acsa.receiveIgnoringPing()
	if err != nil {
		return nil, err
	}

	if confirmMsg.Type != "confirm_subscription" {
		return nil, fmt.Errorf("expected confirm msg, got %v", confirmMsg)
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
		err := websocket.JSON.Receive(acsa.conn, &msg)
		if err != nil {
			return nil, err
		}

		if msg.Type == "ping" {
			continue
		}

		return &msg, nil
	}
}
