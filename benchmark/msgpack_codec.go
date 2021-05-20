package benchmark

import (
	"bytes"

	"golang.org/x/net/websocket"

	"github.com/vmihailenco/msgpack/v5"
)

func msgPackMarshal(v interface{}) (msg []byte, payloadType byte, err error) {
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	enc.SetCustomStructTag("json")

	err = enc.Encode(v)

	return buf.Bytes(), websocket.BinaryFrame, err
}

func msgPackUnmarshal(msg []byte, payloadType byte, v interface{}) (err error) {
	dec := msgpack.NewDecoder(bytes.NewReader(msg))
	dec.SetCustomStructTag("json")

	err = dec.Decode(v)

	return err
}

var MsgPackCodec = websocket.Codec{Marshal: msgPackMarshal, Unmarshal: msgPackUnmarshal}
