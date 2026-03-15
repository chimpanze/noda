package wasm

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	wasmrt "github.com/chimpanze/noda/internal/wasm"
	"github.com/chimpanze/noda/pkg/api"
)

var queryServiceDeps = map[string]api.ServiceDep{
	"runtime": {Prefix: "wasm", Required: true},
}

type queryDescriptor struct{}

func (d *queryDescriptor) Name() string                           { return "query" }
func (d *queryDescriptor) Description() string                    { return "Sends a synchronous query to a Wasm module" }
func (d *queryDescriptor) ServiceDeps() map[string]api.ServiceDep { return queryServiceDeps }
func (d *queryDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data":    map[string]any{"description": "Query data to send to the Wasm module"},
			"timeout": map[string]any{"type": "string", "description": "Query timeout duration (default: 5s)"},
		},
		"required": []any{"data"},
	}
}
func (d *queryDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Query result from the Wasm module",
		"error":   "Wasm execution error",
	}
}

type queryExecutor struct{}

func newQueryExecutor(_ map[string]any) api.NodeExecutor { return &queryExecutor{} }

func (e *queryExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *queryExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*wasmrt.WasmService](services, "runtime")
	if err != nil {
		return "", nil, err
	}

	data, err := plugin.ResolveDeepAny(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("wasm.query: %w", err)
	}

	timeout := "5s"
	if t, found, err := plugin.ResolveOptionalString(nCtx, config, "timeout"); err == nil && found {
		timeout = t
	}

	result, err := svc.Query(ctx, data, timeout)
	if err != nil {
		return "", nil, err
	}

	return api.OutputSuccess, result, nil
}
