package transform

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelete_OneField(t *testing.T) {
	executor := newDeleteExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"name": "Alice", "password": "secret", "age": 30,
	}))

	config := map[string]any{
		"data":   "{{ input }}",
		"fields": []any{"password"},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, "Alice", result["name"])
	assert.Equal(t, 30, result["age"])
	_, hasPassword := result["password"]
	assert.False(t, hasPassword)
}

func TestDelete_MultipleFields(t *testing.T) {
	executor := newDeleteExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"a": 1, "b": 2, "c": 3, "d": 4,
	}))

	config := map[string]any{
		"data":   "{{ input }}",
		"fields": []any{"a", "c"},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Len(t, result, 2)
	assert.Equal(t, 2, result["b"])
	assert.Equal(t, 4, result["d"])
}

func TestDelete_NonexistentField(t *testing.T) {
	executor := newDeleteExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"name": "Alice",
	}))

	config := map[string]any{
		"data":   "{{ input }}",
		"fields": []any{"nonexistent"},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, "Alice", result["name"])
}

func TestDelete_OriginalUnchanged(t *testing.T) {
	original := map[string]any{"name": "Alice", "age": 30}
	executor := newDeleteExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(original))

	config := map[string]any{
		"data":   "{{ input }}",
		"fields": []any{"age"},
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)

	// Original should still have both fields
	assert.Equal(t, 30, original["age"])
	assert.Equal(t, "Alice", original["name"])
}

func TestDelete_NestedObject(t *testing.T) {
	executor := newDeleteExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"name":    "Alice",
		"address": map[string]any{"city": "NYC", "zip": "10001"},
	}))

	config := map[string]any{
		"data":   "{{ input }}",
		"fields": []any{"address"},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Len(t, result, 1)
	assert.Equal(t, "Alice", result["name"])
}

func TestDelete_DataResolveError(t *testing.T) {
	executor := newDeleteExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{
		"data":   "{{ nonexistent.field }}",
		"fields": []any{"a"},
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transform.delete: data")
}

func TestDelete_DataNotObject(t *testing.T) {
	executor := newDeleteExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput("not an object"))

	config := map[string]any{
		"data":   "{{ input }}",
		"fields": []any{"a"},
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data must be an object")
}

func TestDelete_Descriptor(t *testing.T) {
	d := &deleteDescriptor{}
	assert.Equal(t, "delete", d.Name())
	assert.NotEmpty(t, d.Description())
	assert.Nil(t, d.ServiceDeps())
	schema := d.ConfigSchema()
	assert.NotNil(t, schema)
	outputs := d.OutputDescriptions()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}
