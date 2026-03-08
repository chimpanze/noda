package engine

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failNTimesExecutor fails the first N times, then succeeds.
type failNTimesExecutor struct {
	failCount int
	calls     atomic.Int32
}

func (e *failNTimesExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *failNTimesExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	n := int(e.calls.Add(1))
	if n <= e.failCount {
		return "", nil, fmt.Errorf("attempt %d failed", n)
	}
	return "success", map[string]any{"recovered": true}, nil
}

func setupRetryTest(t *testing.T, executor api.NodeExecutor) (*registry.NodeRegistry, *registry.ServiceRegistry) {
	t.Helper()
	plugins := registry.NewPluginRegistry()
	nodes := registry.NewNodeRegistry()

	p := &testPlugin{
		name:   "retry-test",
		prefix: "rt",
		nodes: []api.NodeRegistration{
			{
				Descriptor: &testDescriptor{name: "flaky"},
				Factory:    func(map[string]any) api.NodeExecutor { return executor },
			},
		},
	}
	require.NoError(t, plugins.Register(p))
	require.NoError(t, nodes.RegisterFromPlugin(p))

	return nodes, registry.NewServiceRegistry()
}

func TestRetry_SucceedsOnRetry(t *testing.T) {
	executor := &failNTimesExecutor{failCount: 1}
	nodes, services := setupRetryTest(t, executor)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "flaky-node",
		Type:    "rt.flaky",
		Outputs: []string{"success", "error"},
	}

	// First dispatch fails
	output, err := dispatchNode(context.Background(), node, execCtx, services, nodes)
	require.NoError(t, err)
	assert.Equal(t, "error", output)

	// Retry should succeed on second attempt
	output, err = retryNode(context.Background(), node, execCtx, services, nodes, &RetryConfig{
		Attempts: 3,
		Backoff:  "fixed",
		Delay:    "1ms",
	})
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestRetry_AllRetriesExhausted(t *testing.T) {
	executor := &failNTimesExecutor{failCount: 100} // always fails
	nodes, services := setupRetryTest(t, executor)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "always-fail",
		Type:    "rt.flaky",
		Outputs: []string{"success", "error"},
	}

	output, err := retryNode(context.Background(), node, execCtx, services, nodes, &RetryConfig{
		Attempts: 2,
		Backoff:  "fixed",
		Delay:    "1ms",
	})
	require.NoError(t, err)
	assert.Equal(t, "error", output)
}

func TestRetry_FixedBackoff(t *testing.T) {
	executor := &failNTimesExecutor{failCount: 100}
	nodes, services := setupRetryTest(t, executor)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "node",
		Type:    "rt.flaky",
		Outputs: []string{"success", "error"},
	}

	start := time.Now()
	retryNode(context.Background(), node, execCtx, services, nodes, &RetryConfig{
		Attempts: 3,
		Backoff:  "fixed",
		Delay:    "10ms",
	})
	elapsed := time.Since(start)

	// 3 retries × 10ms = ~30ms minimum
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(25))
}

func TestRetry_ExponentialBackoff(t *testing.T) {
	executor := &failNTimesExecutor{failCount: 100}
	nodes, services := setupRetryTest(t, executor)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "node",
		Type:    "rt.flaky",
		Outputs: []string{"success", "error"},
	}

	start := time.Now()
	retryNode(context.Background(), node, execCtx, services, nodes, &RetryConfig{
		Attempts: 3,
		Backoff:  "exponential",
		Delay:    "10ms",
	})
	elapsed := time.Since(start)

	// exponential: 10ms + 20ms + 40ms = 70ms minimum
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(60))
}

func TestRetry_ContextCancellation(t *testing.T) {
	executor := &failNTimesExecutor{failCount: 100}
	nodes, services := setupRetryTest(t, executor)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "node",
		Type:    "rt.flaky",
		Outputs: []string{"success", "error"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()

	output, err := retryNode(ctx, node, execCtx, services, nodes, &RetryConfig{
		Attempts: 100,
		Backoff:  "fixed",
		Delay:    "100ms",
	})
	require.NoError(t, err)
	assert.Equal(t, "error", output)
}
