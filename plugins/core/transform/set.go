package transform

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type setDescriptor struct{}

func (d *setDescriptor) Name() string                           { return "set" }
func (d *setDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *setDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"fields": map[string]any{"type": "object"},
		},
		"required": []any{"fields"},
	}
}

type setExecutor struct{}

func newSetExecutor(config map[string]any) api.NodeExecutor {
	return &setExecutor{}
}

func (e *setExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *setExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	fields, _ := config["fields"].(map[string]any)
	if fields == nil {
		return "", nil, fmt.Errorf("transform.set: fields is required")
	}

	result := make(map[string]any, len(fields))
	for key, expr := range fields {
		exprStr, ok := expr.(string)
		if !ok {
			result[key] = expr
			continue
		}
		resolved, err := nCtx.Resolve(exprStr)
		if err != nil {
			return "", nil, fmt.Errorf("transform.set: field %q: %w", key, err)
		}
		result[key] = resolved
	}

	return api.OutputSuccess, result, nil
}
