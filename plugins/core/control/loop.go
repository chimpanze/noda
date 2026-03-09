package control

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type loopDescriptor struct{}

func (d *loopDescriptor) Name() string                           { return "loop" }
func (d *loopDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *loopDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"collection": map[string]any{"type": "string"},
			"workflow":   map[string]any{"type": "string"},
			"input":      map[string]any{"type": "object"},
		},
		"required": []any{"collection", "workflow"},
	}
}

// LoopExecutor handles iteration over collections.
// The actual sub-workflow execution is injected via SubWorkflowRunner
// to avoid circular imports with the engine package.
type LoopExecutor struct {
	Runner SubWorkflowRunner
}

// SubWorkflowRunner executes a sub-workflow. Injected by the engine.
type SubWorkflowRunner interface {
	RunSubWorkflow(ctx context.Context, workflowID string, input any, parentCtx api.ExecutionContext) (outputName string, data any, err error)
}

func newLoopExecutor(config map[string]any) api.NodeExecutor {
	return &LoopExecutor{}
}

func (e *LoopExecutor) Outputs() []string { return []string{"done", api.OutputError} }

func (e *LoopExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	collectionExpr, _ := config["collection"].(string)
	workflowID, _ := config["workflow"].(string)
	inputTemplate, _ := config["input"].(map[string]any)

	// Resolve collection expression
	collectionVal, err := nCtx.Resolve(collectionExpr)
	if err != nil {
		return "", nil, fmt.Errorf("loop: resolve collection: %w", err)
	}

	items, ok := collectionVal.([]any)
	if !ok {
		return "", nil, fmt.Errorf("loop: collection must be an array, got %T", collectionVal)
	}

	if len(items) == 0 {
		return "done", []any{}, nil
	}

	if e.Runner == nil {
		return "", nil, fmt.Errorf("loop: sub-workflow runner not configured")
	}

	var results []any

	for i, item := range items {
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		default:
		}

		// Build input for this iteration by resolving input template
		// with $item and $index available
		iterInput, err := buildIterInput(inputTemplate, item, i, nCtx)
		if err != nil {
			return "", nil, fmt.Errorf("loop: iteration %d input: %w", i, err)
		}

		_, data, err := e.Runner.RunSubWorkflow(ctx, workflowID, iterInput, nCtx)
		if err != nil {
			return "", nil, fmt.Errorf("loop: iteration %d: %w", i, err)
		}
		results = append(results, data)
	}

	return "done", results, nil
}

// buildIterInput resolves the input template with $item and $index available
// in the expression context via ResolveWithVars.
func buildIterInput(template map[string]any, item any, index int, nCtx api.ExecutionContext) (map[string]any, error) {
	vars := map[string]any{
		"$item":  item,
		"$index": index,
	}

	if template == nil {
		return map[string]any{"item": item, "index": index}, nil
	}

	result := make(map[string]any, len(template))
	for k, v := range template {
		if s, ok := v.(string); ok {
			resolved, err := nCtx.ResolveWithVars(s, vars)
			if err != nil {
				return nil, fmt.Errorf("field %q: %w", k, err)
			}
			result[k] = resolved
		} else {
			result[k] = v
		}
	}

	return result, nil
}
