package wasm

import (
	"encoding/json"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// Codec handles serialization/deserialization for the Wasm boundary.
type Codec interface {
	Marshal(v any) ([]byte, error)
	Unmarshal(data []byte, v any) error
	Name() string
}

// NewCodec creates a Codec for the given encoding name.
func NewCodec(encoding string) (Codec, error) {
	switch encoding {
	case "", "json":
		return &jsonCodec{}, nil
	case "msgpack":
		return &msgpackCodec{}, nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %q", encoding)
	}
}

type jsonCodec struct{}

func (c *jsonCodec) Marshal(v any) ([]byte, error)        { return json.Marshal(v) }
func (c *jsonCodec) Unmarshal(data []byte, v any) error    { return json.Unmarshal(data, v) }
func (c *jsonCodec) Name() string                          { return "json" }

type msgpackCodec struct{}

func (c *msgpackCodec) Marshal(v any) ([]byte, error)     { return msgpack.Marshal(v) }
func (c *msgpackCodec) Unmarshal(data []byte, v any) error { return msgpack.Unmarshal(data, v) }
func (c *msgpackCodec) Name() string                       { return "msgpack" }
