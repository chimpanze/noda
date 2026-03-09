package wasm

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

var queryServiceDeps = map[string]api.ServiceDep{
	"runtime": {Prefix: "wasm", Required: true},
}

type queryDescriptor struct{}

func (d *queryDescriptor) Name() string                          { return "query" }
func (d *queryDescriptor) ServiceDeps() map[string]api.ServiceDep { return queryServiceDeps }
func (d *queryDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data":    map[string]any{},
			"timeout": map[string]any{"type": "string"},
		},
		"required": []any{"data"},
	}
}

type queryExecutor struct{}

func newQueryExecutor(_ map[string]any) api.NodeExecutor { return &queryExecutor{} }

func (e *queryExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *queryExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := getWasmService(services)
	if err != nil {
		return "", nil, err
	}

	data, err := resolveData(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("wasm.query: %w", err)
	}

	timeout := "5s"
	if t, ok := config["timeout"].(string); ok {
		resolved, err := nCtx.Resolve(t)
		if err == nil {
			if s, ok := resolved.(string); ok {
				timeout = s
			}
		}
	}

	result, err := svc.Query(ctx, data, timeout)
	if err != nil {
		return "error", map[string]any{"error": err.Error()}, nil
	}

	return "success", result, nil
}
