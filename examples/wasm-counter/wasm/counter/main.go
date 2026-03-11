package main

import (
	"encoding/json"

	"github.com/nodafw/noda-pdk-go/noda"
)

// Global counter state — persists across ticks in Wasm linear memory.
var counter int64

//go:wasmexport initialize
func initialize() int32 {
	input, err := noda.GetInitInput()
	if err != nil {
		return noda.Fail(err)
	}

	// Set initial counter value from config if provided
	if v, ok := input.Config["initial_value"].(float64); ok {
		counter = int64(v)
	}

	noda.LogInfo("counter module initialized", map[string]any{"initial_value": counter})
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
		if err := json.Unmarshal(cmd.Data, &op); err != nil {
			continue
		}

		switch op.Op {
		case "add":
			counter += op.Value
		case "subtract":
			counter -= op.Value
		case "multiply":
			counter *= op.Value
		case "reset":
			counter = 0
		}

		noda.LogInfo("counter operation", map[string]any{"op": op.Op, "value": op.Value, "result": counter})
	}

	return 0
}

//go:wasmexport query
func query() int32 {
	return noda.Output(map[string]any{
		"value": counter,
	})
}

//go:wasmexport shutdown
func shutdown() int32 {
	noda.LogInfo("counter module shutting down", map[string]any{"final_value": counter})
	return 0
}

func main() {}
