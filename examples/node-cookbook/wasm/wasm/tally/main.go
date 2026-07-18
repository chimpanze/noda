package main

import (
	"github.com/nodafw/noda-pdk-go/noda"
)

// Global running total — persists across ticks in Wasm linear memory.
var total int64

//go:wasmexport initialize
func initialize() int32 {
	input, err := noda.GetInitInput()
	if err != nil {
		return noda.Fail(err)
	}

	// Set the starting total from config if provided.
	if v, ok := input.Config["initial"].(float64); ok {
		total = int64(v)
	}

	noda.LogInfo("tally module initialized", map[string]any{"initial": total})
	return 0
}

//go:wasmexport tick
func tick() int32 {
	input, err := noda.GetTickInput()
	if err != nil {
		return 0
	}

	for _, cmd := range input.Commands {
		var op struct {
			Op    string `json:"op"`
			Value int64  `json:"value"`
		}
		if err := noda.DecodeInto(cmd.Data, &op); err != nil {
			continue
		}

		if op.Op == "add" {
			total += op.Value
		}

		noda.LogInfo("tally operation", map[string]any{"op": op.Op, "value": op.Value, "result": total})
	}

	return 0
}

//go:wasmexport query
func query() int32 {
	return noda.Output(map[string]any{
		"total": total,
	})
}

//go:wasmexport shutdown
func shutdown() int32 {
	noda.LogInfo("tally module shutting down", map[string]any{"final_total": total})
	return 0
}

func main() {}
