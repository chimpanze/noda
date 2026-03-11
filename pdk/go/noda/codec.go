package noda

import (
	"encoding/json"

	"github.com/vmihailenco/msgpack/v5"
)

// Codec handles serialization for the Wasm boundary.
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
}

// activeCodec is the package-level codec, set during GetInitInput based on encoding config.
// Defaults to JSON until initialize runs.
var activeCodec Codec = &jsonCodec{}

type jsonCodec struct{}

func (c *jsonCodec) Marshal(v any) ([]byte, error)      { return json.Marshal(v) }
func (c *jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }

type msgpackCodec struct{}

func (c *msgpackCodec) Marshal(v any) ([]byte, error)      { return msgpack.Marshal(v) }
func (c *msgpackCodec) Unmarshal(data []byte, v any) error { return msgpack.Unmarshal(data, v) }

// detectCodec auto-detects the encoding from the first byte of a payload.
// JSON objects start with '{' (0x7B). MessagePack maps start with 0x80-0x8F, 0xDE, or 0xDF.
func detectCodec(data []byte) Codec {
	if len(data) == 0 {
		return &jsonCodec{}
	}
	b := data[0]
	if b == '{' {
		return &jsonCodec{}
	}
	// MessagePack fixmap (0x80-0x8F), map16 (0xDE), map32 (0xDF)
	if (b >= 0x80 && b <= 0x8F) || b == 0xDE || b == 0xDF {
		return &msgpackCodec{}
	}
	// Default to JSON for unknown formats
	return &jsonCodec{}
}
