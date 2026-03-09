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

func TestExecutionContext_EvictOutput(t *testing.T) {
	ctx := NewExecutionContext()
	ctx.SetOutput("temp", "data")

	_, ok := ctx.GetOutput("temp")
	assert.True(t, ok)

	ctx.EvictOutput("temp")

	_, ok = ctx.GetOutput("temp")
	assert.False(t, ok)
}
