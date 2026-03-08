package workflow

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type runDescriptor struct{}

func (d *runDescriptor) Name() string { return "run" }
func (d *runDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return nil
}
func (d *runDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow":    map[string]any{"type": "string"},
			"input":       map[string]any{"type": "object"},
			"transaction": map[string]any{"type": "boolean"},
		},
		"required": []any{"workflow"},
	}
}

// RunExecutor executes a sub-workflow.
// SubWorkflowRunner is injected by the engine to avoid circular imports.
type RunExecutor struct {
	Runner  SubWorkflowRunner
	outputs []string
}

// SubWorkflowRunner executes a sub-workflow and returns the output name and data.
type SubWorkflowRunner interface {
	RunSubWorkflow(ctx context.Context, workflowID string, input any, parentCtx api.ExecutionContext) (outputName string, data any, err error)
}

func newRunExecutor(config map[string]any) api.NodeExecutor {
	// Collect outputs from sub-workflow's workflow.output nodes
	// For now, use a default set. The engine will inject proper outputs.
	return &RunExecutor{
		outputs: []string{"success", "error"},
	}
}

func (e *RunExecutor) Outputs() []string { return e.outputs }

// SetOutputs allows the engine to set the dynamic outputs after discovering
// the sub-workflow's workflow.output nodes.
func (e *RunExecutor) SetOutputs(outputs []string) {
	e.outputs = outputs
}

func (e *RunExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	workflowID, _ := config["workflow"].(string)

	if e.Runner == nil {
		return "", nil, fmt.Errorf("workflow.run: sub-workflow runner not configured")
	}

	// Resolve input map
	var input any
	if inputMap, ok := config["input"].(map[string]any); ok {
		resolved := make(map[string]any, len(inputMap))
		for k, v := range inputMap {
			if expr, ok := v.(string); ok {
				val, err := nCtx.Resolve(expr)
				if err != nil {
					return "", nil, fmt.Errorf("workflow.run: resolve input %q: %w", k, err)
				}
				resolved[k] = val
			} else {
				resolved[k] = v
			}
		}
		input = resolved
	}

	outputName, data, err := e.Runner.RunSubWorkflow(ctx, workflowID, input, nCtx)
	if err != nil {
		return "", nil, err
	}

	return outputName, data, nil
}
