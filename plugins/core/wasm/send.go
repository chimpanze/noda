package wasm

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"

	wasmrt "github.com/chimpanze/noda/internal/wasm"
)

var sendServiceDeps = map[string]api.ServiceDep{
	"runtime": {Prefix: "wasm", Required: true},
}

type sendDescriptor struct{}

func (d *sendDescriptor) Name() string                          { return "send" }
func (d *sendDescriptor) ServiceDeps() map[string]api.ServiceDep { return sendServiceDeps }
func (d *sendDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data": map[string]any{},
		},
		"required": []any{"data"},
	}
}

type sendExecutor struct{}

func newSendExecutor(_ map[string]any) api.NodeExecutor { return &sendExecutor{} }

func (e *sendExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *sendExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := getWasmService(services)
	if err != nil {
		return "", nil, err
	}

	data, err := resolveData(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("wasm.send: %w", err)
	}

	svc.SendCommand(data)

	return "success", map[string]any{"sent": true}, nil
}

func getWasmService(services map[string]any) (*wasmrt.WasmService, error) {
	svc, ok := services["runtime"]
	if !ok {
		return nil, fmt.Errorf("wasm runtime service not configured")
	}
	ws, ok := svc.(*wasmrt.WasmService)
	if !ok {
		return nil, fmt.Errorf("service does not implement WasmService")
	}
	return ws, nil
}

func resolveData(nCtx api.ExecutionContext, config map[string]any) (any, error) {
	raw, ok := config["data"]
	if !ok {
		return nil, fmt.Errorf("missing required field \"data\"")
	}
	if expr, ok := raw.(string); ok {
		val, err := nCtx.Resolve(expr)
		if err != nil {
			return nil, fmt.Errorf("resolve data: %w", err)
		}
		return val, nil
	}
	return raw, nil
}
