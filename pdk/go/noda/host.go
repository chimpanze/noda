package noda

import "github.com/extism/go-pdk"

// Raw host function imports — these are the only two functions the Noda host exposes.

//go:wasmimport extism:host/user noda_call
func hostCall(uint64) uint64

//go:wasmimport extism:host/user noda_call_async
func hostCallAsync(uint64)

// envelopeAny mirrors the host-call response envelope produced by the host:
// {"ok":true,"data":<v>} on success or {"ok":false,"error":{"code","message"}} on failure.
// Data is decoded into `any` (rather than json.RawMessage) so the same struct works for
// both the JSON and msgpack codecs; on success it is re-marshalled via activeCodec so
// callers (e.g. CallInto) can unmarshal it into their target type as before.
type envelopeAny struct {
	OK    bool `json:"ok" msgpack:"ok"`
	Data  any  `json:"data,omitempty" msgpack:"data,omitempty"`
	Error *struct {
		Code    string `json:"code" msgpack:"code"`
		Message string `json:"message" msgpack:"message"`
	} `json:"error,omitempty" msgpack:"error,omitempty"`
}

// decodeEnvelope parses a host response envelope; returns the re-marshalled data bytes
// on success, or a *HostError describing the failure.
func decodeEnvelope(raw []byte) ([]byte, error) {
	var env envelopeAny
	if err := activeCodec.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if !env.OK {
		if env.Error != nil {
			return nil, &HostError{Code: env.Error.Code, Message: env.Error.Message}
		}
		return nil, &HostError{Code: "INTERNAL_ERROR", Message: "host call failed"}
	}
	if env.Data == nil {
		return nil, nil
	}
	return activeCodec.Marshal(env.Data)
}

// call makes a synchronous host call with raw bytes, returning raw response bytes.
func call(data []byte) ([]byte, error) {
	mem := pdk.AllocateBytes(data)
	defer mem.Free()

	resultOffset := hostCall(mem.Offset())
	if resultOffset == 0 {
		return nil, nil // void success
	}
	rmem := pdk.FindMemory(resultOffset)
	return decodeEnvelope(rmem.ReadBytes())
}

// callAsync makes an asynchronous host call. Result arrives in the next tick's Responses.
func callAsync(data []byte) {
	mem := pdk.AllocateBytes(data)
	defer mem.Free()
	hostCallAsync(mem.Offset())
}
