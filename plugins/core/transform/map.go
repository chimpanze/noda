package transform

import (
	"context"
	"fmt"
	"reflect"

	"github.com/chimpanze/noda/pkg/api"
)

type mapDescriptor struct{}

func (d *mapDescriptor) Name() string { return "map" }
func (d *mapDescriptor) Description() string {
	return "Transforms each item in an array using an expression"
}
func (d *mapDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *mapDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"collection": map[string]any{"type": "string", "description": "Expression resolving to an array"},
			"expression": map[string]any{"type": "string", "description": "Expression applied to each item ($item, $index available)"},
		},
		"required": []any{"collection", "expression"},
	}
}

type mapExecutor struct{}

func newMapExecutor(config map[string]any) api.NodeExecutor {
	return &mapExecutor{}
}

func (e *mapExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *mapExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	collectionExpr, _ := config["collection"].(string)
	expression, _ := config["expression"].(string)

	collectionRaw, err := nCtx.Resolve(collectionExpr)
	if err != nil {
		return "", nil, fmt.Errorf("transform.map: collection: %w", err)
	}

	items, err := toSlice(collectionRaw)
	if err != nil {
		return "", nil, fmt.Errorf("transform.map: collection must be an array: %w", err)
	}

	result := make([]any, len(items))
	for i, item := range items {
		vars := map[string]any{
			"$item":  item,
			"$index": i,
		}
		resolved, err := nCtx.ResolveWithVars(expression, vars)
		if err != nil {
			return "", nil, fmt.Errorf("transform.map: item %d: %w", i, err)
		}
		result[i] = resolved
	}

	return api.OutputSuccess, result, nil
}

// toSlice converts an any value to []any.
func toSlice(v any) ([]any, error) {
	if v == nil {
		return []any{}, nil
	}
	if s, ok := v.([]any); ok {
		return s, nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice {
		result := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			result[i] = rv.Index(i).Interface()
		}
		return result, nil
	}
	return nil, fmt.Errorf("expected array, got %T", v)
}
