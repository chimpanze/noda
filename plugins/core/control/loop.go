package control

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

// depthTracker is implemented by execution contexts that support recursion depth tracking.
type depthTracker interface {
	CheckAndIncrementDepth() error
	DecrementDepth()
}

type loopDescriptor struct{}

func (d *loopDescriptor) Name() string { return "loop" }
func (d *loopDescriptor) Description() string {
	return "Iterates a sub-workflow over each item in a collection"
}
func (d *loopDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *loopDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"collection": map[string]any{"type": "string", "description": "Expression resolving to an array"},
			"workflow":   map[string]any{"type": "string", "description": "Sub-workflow ID to execute per item"},
			"input":      map[string]any{"type": "object", "description": "Input template — $item and $index available"},
		},
		"required": []any{"collection", "workflow"},
	}
}
func (d *loopDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"done":  "Array of results from each iteration",
		"error": "Error from a failed iteration",
	}
}

// LoopExecutor handles iteration over collections.
// The actual sub-workflow execution is injected via api.SubWorkflowRunner
// to avoid circular imports with the engine package.
type LoopExecutor struct {
	Runner api.SubWorkflowRunner
}

// InjectSubWorkflowRunner implements api.SubWorkflowInjectable.
func (e *LoopExecutor) InjectSubWorkflowRunner(runner api.SubWorkflowRunner) {
	e.Runner = runner
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

	// Enforce collection size limit
	maxItems := 100_000
	if mi, ok := config["max_items"].(float64); ok && int(mi) > 0 {
		maxItems = int(mi)
	}
	if mi, ok := config["max_items"].(int); ok && mi > 0 {
		maxItems = mi
	}
	if len(items) > maxItems {
		return "", nil, fmt.Errorf("loop: collection size %d exceeds maximum %d", len(items), maxItems)
	}

	if e.Runner == nil {
		return "", nil, fmt.Errorf("loop: sub-workflow runner not configured")
	}

	var results []any

	// Check recursion depth
	if dt, ok := nCtx.(depthTracker); ok {
		if err := dt.CheckAndIncrementDepth(); err != nil {
			return "", nil, fmt.Errorf("loop: %w", err)
		}
		defer dt.DecrementDepth()
	}

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
