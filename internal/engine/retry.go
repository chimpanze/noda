package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/trace"
)

// retryNode re-executes a node according to the retry config.
// Returns the output name from a successful retry, or "error" if all retries exhausted.
func retryNode(
	ctx context.Context,
	node *CompiledNode,
	execCtx *ExecutionContextImpl,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
	retry *RetryConfig,
) (outputName string, err error) {
	delay, err := time.ParseDuration(retry.Delay)
	if err != nil {
		delay = time.Second
	}

	for attempt := 1; attempt <= retry.Attempts; attempt++ {
		// Wait before retry
		currentDelay := delay
		if retry.Backoff == "exponential" {
			currentDelay = delay * time.Duration(1<<(attempt-1))
		}

		execCtx.Log("info", fmt.Sprintf("retry attempt %d/%d", attempt, retry.Attempts), map[string]any{
			"node_id": node.ID,
			"delay":   currentDelay.String(),
		})

		execCtx.EmitTrace(string(trace.EventRetryAttempted), node.ID, node.Type, "", "", map[string]any{
			"attempt": attempt,
			"max":     retry.Attempts,
			"delay":   currentDelay.String(),
		})

		select {
		case <-ctx.Done():
			return "error", nil
		case <-time.After(currentDelay):
		}

		// Re-execute the node
		output, execErr := dispatchNode(ctx, node, execCtx, services, nodes)
		if execErr != nil {
			// dispatchNode only returns error if there's no error output edge,
			// but during retry the node does have error handling
			continue
		}
		if output != "error" {
			// Node succeeded on retry
			return output, nil
		}
		// Node failed again, continue retrying
	}

	// All retries exhausted
	return "error", nil
}
