package transform

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type deleteDescriptor struct{}

func (d *deleteDescriptor) Name() string                           { return "delete" }
func (d *deleteDescriptor) Description() string                    { return "Removes fields from an object" }
func (d *deleteDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *deleteDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"data":   map[string]any{"type": "string", "description": "Expression resolving to an object"},
			"fields": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Field names to remove"},
		},
		"required": []any{"data", "fields"},
	}
}

type deleteExecutor struct{}

func newDeleteExecutor(config map[string]any) api.NodeExecutor {
	return &deleteExecutor{}
}

func (e *deleteExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *deleteExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	dataExpr, _ := config["data"].(string)
	fieldsRaw, _ := config["fields"].([]any)

	resolved, err := nCtx.Resolve(dataExpr)
	if err != nil {
		return "", nil, fmt.Errorf("transform.delete: data: %w", err)
	}

	obj, ok := resolved.(map[string]any)
	if !ok {
		return "", nil, fmt.Errorf("transform.delete: data must be an object, got %T", resolved)
	}

	// Build set of fields to delete
	deleteSet := make(map[string]bool, len(fieldsRaw))
	for _, f := range fieldsRaw {
		if s, ok := f.(string); ok {
			deleteSet[s] = true
		}
	}

	// Copy without deleted fields
	result := make(map[string]any, len(obj))
	for k, v := range obj {
		if !deleteSet[k] {
			result[k] = v
		}
	}

	return api.OutputSuccess, result, nil
}
