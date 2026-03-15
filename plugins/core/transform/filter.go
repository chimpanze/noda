package transform

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type filterDescriptor struct{}

func (d *filterDescriptor) Name() string                           { return "filter" }
func (d *filterDescriptor) Description() string                    { return "Filters an array by a predicate expression" }
func (d *filterDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *filterDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"collection": map[string]any{"type": "string", "description": "Expression resolving to an array"},
			"expression": map[string]any{"type": "string", "description": "Predicate expression — keeps items where truthy"},
		},
		"required": []any{"collection", "expression"},
	}
}
func (d *filterDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Array of items matching the condition",
		"error":   "Expression evaluation error",
	}
}

type filterExecutor struct{}

func newFilterExecutor(config map[string]any) api.NodeExecutor {
	return &filterExecutor{}
}

func (e *filterExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *filterExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	collectionExpr, _ := config["collection"].(string)
	expression, _ := config["expression"].(string)

	collectionRaw, err := nCtx.Resolve(collectionExpr)
	if err != nil {
		return "", nil, fmt.Errorf("transform.filter: collection: %w", err)
	}

	items, err := toSlice(collectionRaw)
	if err != nil {
		return "", nil, fmt.Errorf("transform.filter: collection must be an array: %w", err)
	}

	var result []any
	for i, item := range items {
		vars := map[string]any{
			"$item":  item,
			"$index": i,
		}
		resolved, err := nCtx.ResolveWithVars(expression, vars)
		if err != nil {
			return "", nil, fmt.Errorf("transform.filter: item %d: %w", i, err)
		}
		if plugin.IsTruthy(resolved) {
			result = append(result, item)
		}
	}

	if result == nil {
		result = []any{}
	}

	return api.OutputSuccess, result, nil
}
