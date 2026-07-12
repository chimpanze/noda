package noda

import "testing"

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
