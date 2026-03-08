package engine

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/registry"
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
			return "error", nil
		}
		return "", fmt.Errorf("node %q: %w", node.ID, execErr)
	}

	// Store output
	execCtx.SetOutput(node.ID, data)
	if output == "" {
		output = "success"
	}
	return output, nil
}
