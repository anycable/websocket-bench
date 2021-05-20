package benchmark

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/websocket"

	"github.com/anycable/websocket-bench/action_cable"
	"github.com/vmihailenco/msgpack/v5"
)

func protobufMarshal(v interface{}) (msg []byte, payloadType byte, err error) {
	data, ok := v.(*acsaMsg)
	if !ok {
		return nil, 0, errors.New("Unsupported message struct")
	}

	buf := action_cable.Message{}
	buf.Identifier = data.Identifier
	buf.Data = data.Data
	buf.Command = action_cable.Command(action_cable.Command_value[data.Command])

	b, err := proto.Marshal(&buf)

	if err != nil {
		return nil, 0, fmt.Errorf("Failed to marshal protobuf: %v. Error: %v", buf, err)
	}

	return b, websocket.BinaryFrame, err
}

func protobufUnmarshal(msg []byte, payloadType byte, v interface{}) (err error) {
	data, ok := v.(*acsaMsg)
	if !ok {
		return errors.New("Unsupported message struct")
	}

	buf := action_cable.Message{}

	err = proto.Unmarshal(msg, &buf)

	if err != nil {
		return fmt.Errorf("Failed to unmarshal protobuf: %v. Error: %v", buf, err)
	}

	data.Identifier = buf.Identifier
	data.Type = buf.Type.String()

	if buf.Message != nil {
		var payload interface{}
		err = msgpack.Unmarshal(buf.Message, &payload)

		if err != nil {
			return
		}

		data.Message = payload
	}

	return err
}

var ProtoBufCodec = websocket.Codec{Marshal: protobufMarshal, Unmarshal: protobufUnmarshal}
