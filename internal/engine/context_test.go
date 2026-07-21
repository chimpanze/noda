package engine

import (
	"fmt"
	"sync"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionContext_InputAuthTrigger(t *testing.T) {
	auth := &api.AuthData{UserID: "user-1", Roles: []string{"admin"}}
	trigger := api.TriggerData{Type: "http", TraceID: "trace-123"}

	ctx := NewExecutionContext(
		WithInput(map[string]any{"name": "Alice"}),
		WithAuth(auth),
		WithTrigger(trigger),
	)

	input := ctx.Input().(map[string]any)
	assert.Equal(t, "Alice", input["name"])
	assert.Equal(t, "user-1", ctx.Auth().UserID)
	assert.Equal(t, "trace-123", ctx.Trigger().TraceID)
	assert.Equal(t, "http", ctx.Trigger().Type)
}

func TestExecutionContext_SetGetOutput(t *testing.T) {
	ctx := NewExecutionContext()
	ctx.SetOutput("fetch-user", map[string]any{"id": 1, "name": "Bob"})

	data, ok := ctx.GetOutput("fetch-user")
	assert.True(t, ok)
	assert.Equal(t, "Bob", data.(map[string]any)["name"])
}

func TestExecutionContext_AsAlias(t *testing.T) {
	ctx := NewExecutionContext()
	ctx.RegisterAlias("fetch-user-node-123", "user")
	ctx.SetOutput("fetch-user-node-123", map[string]any{"name": "Alice"})

	// Retrieve by original node ID should resolve through alias
	data, ok := ctx.GetOutput("fetch-user-node-123")
	assert.True(t, ok)
	assert.Equal(t, "Alice", data.(map[string]any)["name"])
}

func TestExecutionContext_Resolve(t *testing.T) {
	ctx := NewExecutionContext(
		WithInput(map[string]any{"name": "Alice"}),
	)
	ctx.SetOutput("query", map[string]any{"count": 42})

	result, err := ctx.Resolve("{{ input.name }}")
	require.NoError(t, err)
	assert.Equal(t, "Alice", result)

	result, err = ctx.Resolve("{{ nodes.query.count }}")
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}

func TestExecutionContext_ResolveSecrets(t *testing.T) {
	secretsCtx := map[string]any{"NODA_TEST_SECRET": "super-secret"}
	ctx := NewExecutionContext(WithSecrets(secretsCtx))

	result, err := ctx.Resolve("{{ secrets.NODA_TEST_SECRET }}")
	require.NoError(t, err)
	assert.Equal(t, "super-secret", result)
}

func TestExecutionContext_ResolveSecretsInExpression(t *testing.T) {
	secretsCtx := map[string]any{"NODA_TEST_PREFIX": "prod"}
	ctx := NewExecutionContext(WithSecrets(secretsCtx))

	result, err := ctx.Resolve("{{ secrets.NODA_TEST_PREFIX + \"-db\" }}")
	require.NoError(t, err)
	assert.Equal(t, "prod-db", result)
}

func TestExecutionContext_ConcurrentOutputWrites(t *testing.T) {
	ctx := NewExecutionContext()

	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ctx.SetOutput(fmt.Sprintf("node-%d", n), n)
		}(i)
	}
	wg.Wait()

	for i := range 100 {
		data, ok := ctx.GetOutput(fmt.Sprintf("node-%d", i))
		assert.True(t, ok)
		assert.Equal(t, i, data)
	}
}

func TestExecutionContext_TraceIDGenerated(t *testing.T) {
	ctx1 := NewExecutionContext()
	ctx2 := NewExecutionContext()

	assert.NotEmpty(t, ctx1.Trigger().TraceID)
	assert.NotEmpty(t, ctx2.Trigger().TraceID)
	assert.NotEqual(t, ctx1.Trigger().TraceID, ctx2.Trigger().TraceID)
}

func TestExecutionContext_DepthTracking(t *testing.T) {
	ctx := NewExecutionContext()

	// Should allow incrementing up to max depth
	for i := range 64 {
		require.NoError(t, ctx.CheckAndIncrementDepth(), "depth %d should succeed", i)
	}

	// Should reject at max depth
	err := ctx.CheckAndIncrementDepth()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum recursion depth")

	// Decrement and try again
	ctx.DecrementDepth()
	require.NoError(t, ctx.CheckAndIncrementDepth())

	// Clean up
	for range 64 {
		ctx.DecrementDepth()
	}
}

func TestOutputKeys(t *testing.T) {
	ctx := NewExecutionContext()
	ctx.SetOutput("zebra", "z")
	ctx.SetOutput("alpha", "a")
	ctx.SetOutput("mike", "m")

	keys := ctx.OutputKeys()
	assert.Equal(t, []string{"alpha", "mike", "zebra"}, keys)
}

func TestOutputKeys_Empty(t *testing.T) {
	ctx := NewExecutionContext()
	keys := ctx.OutputKeys()
	assert.Empty(t, keys)
}

func TestExecutionContext_EvictOutput(t *testing.T) {
	ctx := NewExecutionContext()
	ctx.SetOutput("temp", "data")

	_, ok := ctx.GetOutput("temp")
	assert.True(t, ok)

	ctx.EvictOutput("temp")

	_, ok = ctx.GetOutput("temp")
	assert.False(t, ok)
}

func TestExecutionContext_EvictOutputNoopWithRetainOutputs(t *testing.T) {
	ctx := NewExecutionContext(WithRetainOutputs())
	ctx.SetOutput("temp", "data")

	ctx.EvictOutput("temp")

	data, ok := ctx.GetOutput("temp")
	assert.True(t, ok, "output must survive eviction under WithRetainOutputs")
	assert.Equal(t, "data", data)
}

func TestResolve_EnvCaptureNotCorruptedByConcurrentResolves(t *testing.T) {
	c := NewExecutionContext(WithInput(map[string]any{"marker": "orig"}))
	// Capture the whole environment map via $env (expr-lang exposes it by reference).
	captured, err := c.Resolve("{{ $env }}")
	require.NoError(t, err)
	capturedMap, ok := captured.(map[string]any)
	require.True(t, ok)

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Resolve("{{ input.marker }}")
		}()
	}
	// Read the captured map concurrently with the Resolves above.
	for range 50 {
		_ = capturedMap["input"]
	}
	wg.Wait()

	// Under the pool, capturedMap was recycled/cleared by a concurrent Resolve.
	require.Contains(t, capturedMap, "input", "captured $env map must remain intact")
}
