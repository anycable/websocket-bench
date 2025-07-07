package benchmark

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/net/websocket"
	"regexp"
	"strconv"
	"time"
)

import (
	"encoding/json"
)

const (
	ClientBroadcastEvent              = "client-broadcast"
	PingType                          = "pusher:ping"
	PongType                          = "pusher:pong"
	SubscribeType                     = "pusher:subscribe"
	ConnectionEstablishedType         = "pusher:connection_established"
	SubscriptionSucceededType         = "pusher_internal:subscription_succeeded"
	InternalSubscriptionSucceededType = "pusher_internal:subscription_succeeded"
	UnsubscribeType                   = "pusher:unsubscribe"
)

var channelNameRegexp = regexp.MustCompile(`^[A-Za-z0-9_=\-@,.;]+$`)

type PusherCommonConfig struct {
	Channel   string
	AppID     string
	AppKey    string
	AppSecret string
	Host      string
	Port      string
}

type pusherMsg struct {
	Event   string          `json:"event"`
	Channel string          `json:"channel,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type broadcastPayload struct {
	SenderSocketID string      `json:"sender_socket_id"`
	Action         string      `json:"action"`
	Payload        interface{} `json:"payload"`
}

func receiveIgnoringPing(conn *websocket.Conn) (*pusherMsg, error) {
	for {
		var msg pusherMsg
		if err := websocket.JSON.Receive(conn, &msg); err != nil {
			return nil, err
		}
		if msg.Event == PingType {
			_ = websocket.JSON.Send(conn, map[string]interface{}{"event": PongType})
			continue
		}
		return &msg, nil
	}
}

func waitConnectionEstablished(conn *websocket.Conn) (string, error) {
	msg, err := receiveIgnoringPing(conn)
	if err != nil {
		return "", err
	}
	if msg.Event != ConnectionEstablishedType {
		return "", fmt.Errorf("expected %s, got %s", ConnectionEstablishedType, msg.Event)
	}

	// data can come either as a string or an object
	type socketPayload struct {
		SocketID string `json:"socket_id"`
	}
	var inner string
	var sp socketPayload

	if err := json.Unmarshal(msg.Data, &inner); err == nil {
		if err := json.Unmarshal([]byte(inner), &sp); err == nil && sp.SocketID != "" {
			return sp.SocketID, nil
		}
	}

	if err := json.Unmarshal(msg.Data, &sp); err != nil {
		return "", err
	}
	if sp.SocketID == "" {
		return "", errors.New("socket_id is empty")
	}
	return sp.SocketID, nil
}

func waitSubscriptionSucceeded(conn *websocket.Conn, channel string) error {
	for {
		msg, err := receiveIgnoringPing(conn)
		if err != nil {
			return err
		}
		if (msg.Event == SubscriptionSucceededType ||
			msg.Event == InternalSubscriptionSucceededType) &&
			msg.Channel == channel {
			return nil
		}
	}
}

func parseBroadcast(raw *pusherMsg, selfSocketID string) (*serverSentMsg, error) {
	if raw.Event != ClientBroadcastEvent {
		return nil, nil
	}

	var dataStr string
	if err := json.Unmarshal(raw.Data, &dataStr); err != nil {
		return nil, fmt.Errorf("unwrap data: %w", err)
	}

	var message broadcastPayload
	if err := json.Unmarshal([]byte(dataStr), &message); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	if message.Action != "broadcast" {
		return nil, nil
	}

	pm, ok := message.Payload.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected payload type: %T", message.Payload)
	}

	nsec, err := strconv.ParseInt(pm["sendTime"].(string), 10, 64)
	if err != nil {
		return nil, err
	}
	pl := &Payload{SendTime: time.Unix(0, nsec)}
	if pad, ok := pm["padding"]; ok {
		if b, err := json.Marshal(pad); err == nil {
			pl.Padding = b
		}
	}

	if message.SenderSocketID == selfSocketID {
		return &serverSentMsg{Type: MsgServerBroadcastResult, Payload: pl}, nil
	}
	return &serverSentMsg{Type: MsgServerBroadcast, Payload: pl}, nil
}

func signChannel(socketID, channel, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(socketID + ":" + channel))
	return hex.EncodeToString(mac.Sum(nil))
}
