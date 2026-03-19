package engine

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
)

// ResolveInput evaluates trigger input mapping expressions against a context map.
func ResolveInput(compiler *expr.Compiler, inputMap map[string]any, ctx map[string]any) (map[string]any, error) {
	if inputMap == nil {
		return map[string]any{}, nil
	}

	resolver := expr.NewResolver(compiler, ctx)
	result := make(map[string]any)

	for key, exprVal := range inputMap {
		exprStr, ok := exprVal.(string)
		if !ok {
			result[key] = exprVal
			continue
		}

		resolved, err := resolver.Resolve(exprStr)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
		result[key] = resolved
	}

	return result, nil
}

// RunWorkflow executes a workflow, using the cache if available or compiling on the fly.
func RunWorkflow(
	ctx context.Context,
	workflowID string,
	execCtx *ExecutionContextImpl,
	cache *WorkflowCache,
	workflows map[string]map[string]any,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
) error {
	if cache != nil {
		graph, ok := cache.Get(workflowID)
		if !ok {
			return fmt.Errorf("workflow %q not found", workflowID)
		}
		return ExecuteGraph(ctx, graph, execCtx, services, nodes)
	}

	// Fallback: compile on the fly (used in tests without a cache)
	wfData, ok := workflows[workflowID]
	if !ok {
		return fmt.Errorf("workflow %q not found", workflowID)
	}
	wfConfig, err := ParseWorkflowFromMap(workflowID, wfData)
	if err != nil {
		return fmt.Errorf("parse workflow %q: %w", workflowID, err)
	}
	graph, err := Compile(wfConfig, nodes)
	if err != nil {
		return fmt.Errorf("compile workflow %q: %w", workflowID, err)
	}
	return ExecuteGraph(ctx, graph, execCtx, services, nodes)
}
