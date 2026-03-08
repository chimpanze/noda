package control

import (
	"context"
	"reflect"

	"github.com/chimpanze/noda/pkg/api"
)

type ifDescriptor struct{}

func (d *ifDescriptor) Name() string                         { return "if" }
func (d *ifDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *ifDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"condition": map[string]any{"type": "string"},
		},
		"required": []any{"condition"},
	}
}

type ifExecutor struct{}

func newIfExecutor(config map[string]any) api.NodeExecutor {
	return &ifExecutor{}
}

func (e *ifExecutor) Outputs() []string { return []string{"then", "else", "error"} }

func (e *ifExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	condExpr, _ := config["condition"].(string)

	result, err := nCtx.Resolve(condExpr)
	if err != nil {
		return "", nil, err
	}

	if isTruthy(result) {
		return "then", result, nil
	}
	return "else", result, nil
}

// isTruthy determines if a value is truthy using Go/Noda rules.
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
		// Check for empty array/slice
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			return rv.Len() > 0
		}
		return true
	}
}
