package control

import (
	"context"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type ifDescriptor struct{}

func (d *ifDescriptor) Name() string                           { return "if" }
func (d *ifDescriptor) Description() string                    { return "Conditional branching based on an expression" }
func (d *ifDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *ifDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"condition": map[string]any{"type": "string", "description": "Expression to evaluate — must resolve to a truthy/falsy value"},
		},
		"required": []any{"condition"},
	}
}

type ifExecutor struct{}

func newIfExecutor(config map[string]any) api.NodeExecutor {
	return &ifExecutor{}
}

func (e *ifExecutor) Outputs() []string { return []string{"then", "else", api.OutputError} }

func (e *ifExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	condExpr, _ := config["condition"].(string)

	result, err := nCtx.Resolve(condExpr)
	if err != nil {
		return "", nil, err
	}

	if plugin.IsTruthy(result) {
		return "then", result, nil
	}
	return "else", result, nil
}
