// Package noda provides the Noda Plugin Development Kit (PDK) for Go-based Wasm modules.
// It wraps the raw Extism host functions with typed helpers so module authors can focus
// on logic rather than serialization boilerplate.
package noda

import "github.com/extism/go-pdk"

// Call makes a synchronous host call and returns the raw response bytes.
func Call(service, operation string, payload any) ([]byte, error) {
	req := hostCallRequest{
		Service:   service,
		Operation: operation,
		Payload:   payload,
	}
	data, err := activeCodec.Marshal(req)
	if err != nil {
		return nil, err
	}
	return call(data)
}

// CallInto makes a synchronous host call and unmarshals the response into dst.
func CallInto(service, operation string, payload any, dst any) error {
	raw, err := Call(service, operation, payload)
	if err != nil {
		return err
	}
	if raw == nil {
		return nil
	}
	return activeCodec.Unmarshal(raw, dst)
}

// CallAsync fires an asynchronous host call. The result arrives in the next tick's
// Responses map, keyed by the given label.
func CallAsync(service, operation, label string, payload any) {
	req := hostCallRequest{
		Service:   service,
		Operation: operation,
		Payload:   payload,
		Label:     label,
	}
	data, err := activeCodec.Marshal(req)
	if err != nil {
		return // cannot report errors in async context; call is silently dropped
	}
	callAsync(data)
}

// GetInitInput reads and parses the initialize export's input.
// It also auto-detects and sets the active codec based on the encoding field.
func GetInitInput() (*InitInput, error) {
	raw := pdk.Input()

	// Auto-detect codec from first byte (bootstrap: we don't know encoding yet)
	codec := detectCodec(raw)

	var input InitInput
	if err := codec.Unmarshal(raw, &input); err != nil {
		return nil, err
	}

	// Set the active codec based on the declared encoding
	switch input.Encoding {
	case "msgpack":
		activeCodec = &msgpackCodec{}
	default:
		activeCodec = &jsonCodec{}
	}

	return &input, nil
}

// GetTickInput reads and parses the tick export's input.
func GetTickInput() (*TickInput, error) {
	raw := pdk.Input()
	var input TickInput
	if err := activeCodec.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	return &input, nil
}

// Output writes a value as the export's output using the active codec.
// Use this in query exports to return data.
func Output(v any) int32 {
	data, err := activeCodec.Marshal(v)
	if err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.Output(data)
	return 0
}

// Fail sets the error from an error value and returns 1.
func Fail(err error) int32 {
	pdk.SetError(err)
	return 1
}

// FailMsg sets a string error message and returns 1.
func FailMsg(msg string) int32 {
	pdk.SetErrorString(msg)
	return 1
}
