package noda

import "testing"

// TestDecodeInto_BothCodecs covers data fidelity under both codecs. Note: a
// codec-decoded any is a codec-agnostic Go value (maps, strings, numbers), so
// this test alone cannot detect a hardcoded-JSON DecodeInto — the
// codec-routing discrimination lives in TestDecodeInto_UsesActiveCodec.
func TestDecodeInto_BothCodecs(t *testing.T) {
	type payload struct {
		Op    string `json:"op" msgpack:"op"`
		Count int    `json:"count" msgpack:"count"`
	}
	orig := activeCodec
	t.Cleanup(func() { activeCodec = orig })

	for name, c := range map[string]Codec{"json": &jsonCodec{}, "msgpack": &msgpackCodec{}} {
		t.Run(name, func(t *testing.T) {
			activeCodec = c
			// simulate what the host delivers: a codec-decoded any
			raw, err := c.Marshal(payload{Op: "incr", Count: 7})
			if err != nil {
				t.Fatal(err)
			}
			var decoded any
			if err := c.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			var dst payload
			if err := DecodeInto(decoded, &dst); err != nil {
				t.Fatalf("DecodeInto: %v", err)
			}
			if dst.Op != "incr" || dst.Count != 7 {
				t.Fatalf("round-trip lost data: %+v", dst)
			}
		})
	}
}

// spyCodec counts calls through the Codec interface, delegating to inner.
type spyCodec struct {
	inner      Codec
	marshals   int
	unmarshals int
}

func (s *spyCodec) Marshal(v any) ([]byte, error) {
	s.marshals++
	return s.inner.Marshal(v)
}

func (s *spyCodec) Unmarshal(data []byte, v any) error {
	s.unmarshals++
	return s.inner.Unmarshal(data, v)
}

// TestDecodeInto_UsesActiveCodec proves DecodeInto routes through activeCodec
// rather than hardcoding a JSON round-trip.
func TestDecodeInto_UsesActiveCodec(t *testing.T) {
	orig := activeCodec
	t.Cleanup(func() { activeCodec = orig })
	spy := &spyCodec{inner: &msgpackCodec{}}
	activeCodec = spy

	var dst struct {
		Op string `json:"op" msgpack:"op"`
	}
	if err := DecodeInto(map[string]any{"op": "incr"}, &dst); err != nil {
		t.Fatalf("DecodeInto: %v", err)
	}
	if dst.Op != "incr" {
		t.Fatalf("round-trip lost data: %+v", dst)
	}
	// The discriminator: a hardcoded-JSON DecodeInto never touches activeCodec.
	if spy.marshals != 1 || spy.unmarshals != 1 {
		t.Fatalf("DecodeInto bypassed activeCodec: marshals=%d unmarshals=%d", spy.marshals, spy.unmarshals)
	}
}
