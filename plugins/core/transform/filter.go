package transform

import (
	"context"
	"fmt"
	"reflect"

	"github.com/chimpanze/noda/pkg/api"
)

type filterDescriptor struct{}

func (d *filterDescriptor) Name() string                           { return "filter" }
func (d *filterDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *filterDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"collection": map[string]any{"type": "string"},
			"expression": map[string]any{"type": "string"},
		},
		"required": []any{"collection", "expression"},
	}
}

type filterExecutor struct{}

func newFilterExecutor(config map[string]any) api.NodeExecutor {
	return &filterExecutor{}
}

func (e *filterExecutor) Outputs() []string { return []string{"success", "error"} }

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
		if isTruthy(resolved) {
			result = append(result, item)
		}
	}

	if result == nil {
		result = []any{}
	}

	return "success", result, nil
}

// isTruthy determines if a value is truthy.
func isTruthy(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val != ""
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			return rv.Len() > 0
		}
		return true
	}
}
