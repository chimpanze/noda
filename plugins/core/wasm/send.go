package wasm

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	wasmrt "github.com/chimpanze/noda/internal/wasm"
	"github.com/chimpanze/noda/pkg/api"
)

var sendServiceDeps = map[string]api.ServiceDep{
	"runtime": {Prefix: "wasm", Required: true},
}

type sendDescriptor struct{}

func (d *sendDescriptor) Name() string { return "send" }
func (d *sendDescriptor) Description() string {
	return "Sends a fire-and-forget command to a Wasm module"
}
func (d *sendDescriptor) ServiceDeps() map[string]api.ServiceDep { return sendServiceDeps }
func (d *sendDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data": map[string]any{"description": "Command data to send to the Wasm module"},
		},
		"required": []any{"data"},
	}
}
func (d *sendDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Response from the Wasm module",
		"error":   "Wasm execution error",
	}
}

func (d *sendDescriptor) OutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"sent": map[string]any{"type": "boolean"},
		},
		"required": []any{"sent"},
	}
}

type sendExecutor struct{}

func newSendExecutor(_ map[string]any) api.NodeExecutor { return &sendExecutor{} }

func (e *sendExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *sendExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*wasmrt.WasmService](services, "runtime")
	if err != nil {
		return "", nil, err
	}

	data, err := plugin.ResolveDeepAny(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("wasm.send: %w", err)
	}

	svc.SendCommand(data)

	return api.OutputSuccess, map[string]any{"sent": true}, nil
}
