package transform

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMap_ExtractField(t *testing.T) {
	executor := newMapExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"users": []any{
			map[string]any{"name": "Alice", "age": 30},
			map[string]any{"name": "Bob", "age": 25},
		},
	}))

	config := map[string]any{
		"collection": "{{ input.users }}",
		"expression": `{{ $item.name }}`,
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{"Alice", "Bob"}, data)
}

func TestMap_ComputeValues(t *testing.T) {
	executor := newMapExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{
			map[string]any{"price": 10, "quantity": 2},
			map[string]any{"price": 5, "quantity": 3},
		},
	}))

	config := map[string]any{
		"collection": "{{ input.items }}",
		"expression": "{{ $item.price * $item.quantity }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{20, 15}, data)
}

func TestMap_IndexAvailable(t *testing.T) {
	executor := newMapExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{"a", "b", "c"},
	}))

	config := map[string]any{
		"collection": "{{ input.items }}",
		"expression": "{{ $index }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{0, 1, 2}, data)
}

func TestMap_EmptyCollection(t *testing.T) {
	executor := newMapExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{},
	}))

	config := map[string]any{
		"collection": "{{ input.items }}",
		"expression": "{{ $item }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{}, data)
}

func TestMap_ExpressionFailure(t *testing.T) {
	executor := newMapExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{map[string]any{"x": 1}},
	}))

	config := map[string]any{
		"collection": "{{ input.items }}",
		"expression": "{{ $item.nonexistent.deep }}",
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transform.map")
}
