package workflow

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type outputDescriptor struct{}

func (d *outputDescriptor) Name() string                           { return "output" }
func (d *outputDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *outputDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"data": map[string]any{},
		},
		"required": []any{"name"},
	}
}

type outputExecutor struct{}

func newOutputExecutor(config map[string]any) api.NodeExecutor {
	return &outputExecutor{}
}

// Outputs returns empty — workflow.output is a terminal node.
func (e *outputExecutor) Outputs() []string { return []string{} }

func (e *outputExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	name := OutputName(config)
	if name == "" {
		return "", nil, fmt.Errorf("workflow.output: missing required field \"name\"")
	}

	// Resolve data expression if present
	if dataExpr, ok := config["data"].(string); ok {
		data, err := nCtx.Resolve(dataExpr)
		if err != nil {
			return "", nil, err
		}
		return name, data, nil
	}

	// No data field — return nil
	return name, nil, nil
}

// OutputName returns the static name from the config.
func OutputName(config map[string]any) string {
	name, _ := config["name"].(string)
	return name
}
