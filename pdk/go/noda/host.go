package noda

import "github.com/extism/go-pdk"

// Raw host function imports — these are the only two functions the Noda host exposes.

//go:wasmimport extism:host/user noda_call
func hostCall(uint64) uint64

//go:wasmimport extism:host/user noda_call_async
func hostCallAsync(uint64)

// call makes a synchronous host call with raw bytes, returning raw response bytes.
func call(data []byte) ([]byte, error) {
	mem := pdk.AllocateBytes(data)
	defer mem.Free()

	resultOffset := hostCall(mem.Offset())
	if resultOffset == 0 {
		return nil, nil
	}
	rmem := pdk.FindMemory(resultOffset)
	return rmem.ReadBytes(), nil
}

// callAsync makes an asynchronous host call. Result arrives in the next tick's Responses.
func callAsync(data []byte) {
	mem := pdk.AllocateBytes(data)
	defer mem.Free()
	hostCallAsync(mem.Offset())
}
