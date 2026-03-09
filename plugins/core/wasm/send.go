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

func (d *sendDescriptor) Name() string                           { return "send" }
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

func (e *sendExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *sendExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*wasmrt.WasmService](services, "runtime")
	if err != nil {
		return "", nil, err
	}

	data, err := plugin.ResolveAny(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("wasm.send: %w", err)
	}

	svc.SendCommand(data)

	return api.OutputSuccess, map[string]any{"sent": true}, nil
}
