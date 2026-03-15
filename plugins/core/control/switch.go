package control

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type switchDescriptor struct{}

func (d *switchDescriptor) Name() string                           { return "switch" }
func (d *switchDescriptor) Description() string                    { return "Multi-way branching with case matching" }
func (d *switchDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *switchDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"expression": map[string]any{"type": "string", "description": "Expression to evaluate for matching"},
			"cases":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Case values to match against the expression"},
		},
		"required": []any{"expression", "cases"},
	}
}
func (d *switchDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"default": "Input data passed through to the matching case output",
	}
}

type switchExecutor struct {
	cases []string
}

func newSwitchExecutor(config map[string]any) api.NodeExecutor {
	var cases []string
	if rawCases, ok := config["cases"].([]any); ok {
		for _, c := range rawCases {
			if s, ok := c.(string); ok {
				cases = append(cases, s)
			}
		}
	}
	return &switchExecutor{cases: cases}
}

func (e *switchExecutor) Outputs() []string {
	outputs := make([]string, 0, len(e.cases)+2)
	outputs = append(outputs, e.cases...)
	outputs = append(outputs, "default", api.OutputError)
	return outputs
}

func (e *switchExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	exprStr, _ := config["expression"].(string)

	result, err := nCtx.Resolve(exprStr)
	if err != nil {
		return "", nil, err
	}

	// Convert result to string for matching
	resultStr := fmt.Sprintf("%v", result)

	for _, c := range e.cases {
		if resultStr == c {
			return c, result, nil
		}
	}

	return "default", result, nil
}
