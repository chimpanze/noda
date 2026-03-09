package engine

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/trace"
)

// dispatchNode executes a single node: resolves services, calls Execute, stores output.
func dispatchNode(
	ctx context.Context,
	node *CompiledNode,
	execCtx *ExecutionContextImpl,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
) (outputName string, err error) {
	// Look up executor factory
	factory, ok := nodes.GetFactory(node.Type)
	if !ok {
		return "", fmt.Errorf("node %q: unknown type %q", node.ID, node.Type)
	}

	// Create executor instance
	executor := factory(node.Config)

	// Resolve service slots
	resolvedServices := make(map[string]any)
	for slot, svcName := range node.Services {
		svc, found := services.Get(svcName)
		if !found {
			return "", fmt.Errorf("node %q: service %q not found for slot %q", node.ID, svcName, slot)
		}
		resolvedServices[slot] = svc
	}

	// Start OTel node span
	ctx, nodeSpan := trace.StartNodeSpan(ctx, execCtx.Tracer(), node.ID, node.Type)
	execCtx.EmitTrace("node:entered", node.ID, node.Type, "", "", nil)

	// Set current node for logging context
	execCtx.SetCurrentNode(node.ID)
	defer execCtx.SetCurrentNode("")

	// Register alias if configured
	if node.As != "" {
		execCtx.RegisterAlias(node.ID, node.As)
	}

	// Execute the node
	output, data, execErr := executor.Execute(ctx, execCtx, node.Config, resolvedServices)

	if execErr != nil {
		// Check if node has error output edges
		if containsString(node.Outputs, "error") {
			// Store error data and return "error" output
			errorData := map[string]any{
				"error":   execErr.Error(),
				"node_id": node.ID,
			}
			execCtx.SetOutput(node.ID, errorData)
			trace.EndNodeSpan(nodeSpan, "error", nil)
			execCtx.EmitTrace("node:completed", node.ID, node.Type, "error", execErr.Error(), errorData)
			return "error", nil
		}
		trace.EndNodeSpan(nodeSpan, "", execErr)
		execCtx.EmitTrace("node:failed", node.ID, node.Type, "", execErr.Error(), nil)
		return "", fmt.Errorf("node %q: %w", node.ID, execErr)
	}

	// Intercept HTTPResponse if present
	execCtx.InterceptResponse(data)

	// Store output
	execCtx.SetOutput(node.ID, data)
	if output == "" {
		output = "success"
	}

	// Validate that the executor returned a declared output name
	if !containsString(node.Outputs, output) {
		execErr := fmt.Errorf("node %q returned undeclared output %q", node.ID, output)
		trace.EndNodeSpan(nodeSpan, "", execErr)
		execCtx.EmitTrace("node:failed", node.ID, node.Type, "", execErr.Error(), nil)
		return "", execErr
	}

	trace.EndNodeSpan(nodeSpan, output, nil)
	execCtx.EmitTrace("node:completed", node.ID, node.Type, output, "", data)
	return output, nil
}
