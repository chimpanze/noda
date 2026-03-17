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

func TestMap_CollectionResolveError(t *testing.T) {
	executor := newMapExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"collection": "{{ nonexistent.field }}",
		"expression": "{{ $item }}",
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transform.map: collection")
}

func TestMap_CollectionNotArray(t *testing.T) {
	executor := newMapExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"data": "not an array",
	}))

	config := map[string]any{
		"collection": "{{ input.data }}",
		"expression": "{{ $item }}",
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collection must be an array")
}

func TestMap_NilCollection(t *testing.T) {
	executor := newMapExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"data": nil,
	}))

	config := map[string]any{
		"collection": "{{ input.data }}",
		"expression": "{{ $item }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{}, data)
}

func TestMap_TypedSlice(t *testing.T) {
	executor := newMapExecutor(nil)
	// Use a typed slice ([]string) instead of []any to exercise the reflect path in toSlice
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"names": []string{"Alice", "Bob"},
	}))

	config := map[string]any{
		"collection": "{{ input.names }}",
		"expression": "{{ $item }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{"Alice", "Bob"}, data)
}

func TestMap_Descriptor(t *testing.T) {
	d := &mapDescriptor{}
	assert.Equal(t, "map", d.Name())
	assert.NotEmpty(t, d.Description())
	assert.Nil(t, d.ServiceDeps())
	schema := d.ConfigSchema()
	assert.NotNil(t, schema)
	outputs := d.OutputDescriptions()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}
