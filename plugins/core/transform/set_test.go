package transform

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSet_SimpleFieldMapping(t *testing.T) {
	executor := newSetExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"name": "Alice", "age": 30}))

	config := map[string]any{
		"fields": map[string]any{
			"greeting": "{{ \"Hello, \" + input.name }}",
			"doubled":  "{{ input.age * 2 }}",
		},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, "Hello, Alice", result["greeting"])
	assert.Equal(t, 60, result["doubled"])
}

func TestSet_MultipleFields(t *testing.T) {
	executor := newSetExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"x": 1, "y": 2, "z": 3}))

	config := map[string]any{
		"fields": map[string]any{
			"a": "{{ input.x }}",
			"b": "{{ input.y }}",
			"c": "{{ input.z }}",
		},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, 1, result["a"])
	assert.Equal(t, 2, result["b"])
	assert.Equal(t, 3, result["c"])
}

func TestSet_UpstreamNodeOutput(t *testing.T) {
	executor := newSetExecutor(nil)
	execCtx := engine.NewExecutionContext()
	execCtx.SetOutput("prev", map[string]any{"id": 42})

	config := map[string]any{
		"fields": map[string]any{
			"result_id": "{{ prev.id }}",
		},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, 42, data.(map[string]any)["result_id"])
}

func TestSet_StringInterpolation(t *testing.T) {
	executor := newSetExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"id": 5}))

	config := map[string]any{
		"fields": map[string]any{
			"url": "{{ \"/users/\" + string(input.id) }}",
		},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "/users/5", data.(map[string]any)["url"])
}

func TestSet_ExpressionFailure(t *testing.T) {
	executor := newSetExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"fields": map[string]any{
			"bad": "{{ nonexistent.field }}",
		},
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transform.set")
}

func TestSet_MissingFields(t *testing.T) {
	executor := newSetExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fields is required")
}
