package transform

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilter_ByCondition(t *testing.T) {
	executor := newFilterExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"people": []any{
			map[string]any{"name": "Alice", "age": 25},
			map[string]any{"name": "Bob", "age": 15},
			map[string]any{"name": "Carol", "age": 30},
		},
	}))

	config := map[string]any{
		"collection": "{{ input.people }}",
		"expression": "{{ $item.age >= 18 }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.([]any)
	assert.Len(t, result, 2)
	assert.Equal(t, "Alice", result[0].(map[string]any)["name"])
	assert.Equal(t, "Carol", result[1].(map[string]any)["name"])
}

func TestFilter_AllPass(t *testing.T) {
	executor := newFilterExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"nums": []any{1, 2, 3},
	}))

	config := map[string]any{
		"collection": "{{ input.nums }}",
		"expression": "{{ $item > 0 }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{1, 2, 3}, data)
}

func TestFilter_NonePass(t *testing.T) {
	executor := newFilterExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"nums": []any{1, 2, 3},
	}))

	config := map[string]any{
		"collection": "{{ input.nums }}",
		"expression": "{{ $item > 100 }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{}, data)
}

func TestFilter_EmptyCollection(t *testing.T) {
	executor := newFilterExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{},
	}))

	config := map[string]any{
		"collection": "{{ input.items }}",
		"expression": "{{ true }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{}, data)
}

func TestFilter_IndexAvailable(t *testing.T) {
	executor := newFilterExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{"a", "b", "c", "d"},
	}))

	config := map[string]any{
		"collection": "{{ input.items }}",
		"expression": "{{ $index % 2 == 0 }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{"a", "c"}, data)
}

func TestFilter_ExpressionFailure(t *testing.T) {
	executor := newFilterExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{map[string]any{"x": 1}},
	}))

	config := map[string]any{
		"collection": "{{ input.items }}",
		"expression": "{{ $item.nonexistent.deep }}",
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transform.filter")
}
