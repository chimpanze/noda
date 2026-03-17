package transform

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_ValidData(t *testing.T) {
	config := map[string]any{
		"data": "{{ input }}",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			"required": []any{"name"},
		},
	}

	executor := newValidateExecutor(config)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"name": "Alice",
		"age":  30,
	}))

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.NotNil(t, data)
}

func TestValidate_MissingRequired(t *testing.T) {
	config := map[string]any{
		"data": "{{ input }}",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		},
	}

	executor := newValidateExecutor(config)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"age": 30,
	}))

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)

	var valErr *validationResultError
	require.ErrorAs(t, err, &valErr)
	assert.NotEmpty(t, valErr.Errors)
}

func TestValidate_WrongType(t *testing.T) {
	config := map[string]any{
		"data": "{{ input }}",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"age": map[string]any{"type": "integer"},
			},
		},
	}

	executor := newValidateExecutor(config)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"age": "not a number",
	}))

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)

	var valErr *validationResultError
	require.ErrorAs(t, err, &valErr)
	assert.NotEmpty(t, valErr.Errors)
}

func TestValidate_PatternMismatch(t *testing.T) {
	config := map[string]any{
		"data": "{{ input }}",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"email": map[string]any{
					"type":    "string",
					"pattern": "^[a-z]+@[a-z]+\\.[a-z]+$",
				},
			},
		},
	}

	executor := newValidateExecutor(config)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"email": "not-an-email",
	}))

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
}

func TestValidate_MultipleErrors(t *testing.T) {
	config := map[string]any{
		"data": "{{ input }}",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
				"age":  map[string]any{"type": "integer"},
			},
			"required": []any{"name", "age"},
		},
	}

	executor := newValidateExecutor(config)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)

	var valErr *validationResultError
	require.ErrorAs(t, err, &valErr)
	assert.NotEmpty(t, valErr.Errors)
}

func TestValidate_MissingSchema(t *testing.T) {
	config := map[string]any{
		"data": "{{ input }}",
	}

	executor := newValidateExecutor(config)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"name": "Alice"}))

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestValidate_DefaultDataExpression(t *testing.T) {
	config := map[string]any{
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
		},
	}

	executor := newValidateExecutor(config)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"name": "Alice",
	}))

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.NotNil(t, data)
}

func TestValidate_DataResolveError(t *testing.T) {
	config := map[string]any{
		"data": "{{ nonexistent.field }}",
		"schema": map[string]any{
			"type": "object",
		},
	}

	executor := newValidateExecutor(config)
	execCtx := engine.NewExecutionContext()

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transform.validate: data")
}

func TestValidate_SingleErrorMessage(t *testing.T) {
	config := map[string]any{
		"data": "{{ input }}",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string"},
			},
			"required": []any{"name"},
		},
	}

	executor := newValidateExecutor(config)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)

	var valErr *validationResultError
	require.ErrorAs(t, err, &valErr)
	assert.Len(t, valErr.Errors, 1)
	// Single error should format with field:message
	assert.Contains(t, valErr.Error(), "validation failed:")
	assert.Contains(t, valErr.Error(), valErr.Errors[0].Field)
}

func TestValidate_Descriptor(t *testing.T) {
	d := &validateDescriptor{}
	assert.Equal(t, "validate", d.Name())
	assert.NotEmpty(t, d.Description())
	assert.Nil(t, d.ServiceDeps())
	schema := d.ConfigSchema()
	assert.NotNil(t, schema)
	outputs := d.OutputDescriptions()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

func TestValidate_NestedSchema(t *testing.T) {
	config := map[string]any{
		"data": "{{ input }}",
		"schema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"address": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
						"zip":  map[string]any{"type": "string"},
					},
					"required": []any{"city"},
				},
			},
		},
	}

	executor := newValidateExecutor(config)

	// Valid nested data
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"address": map[string]any{"city": "NYC", "zip": "10001"},
	}))
	output, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	// Invalid nested data
	execCtx2 := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"address": map[string]any{"zip": "10001"},
	}))
	_, _, err = executor.Execute(context.Background(), execCtx2, config, nil)
	require.Error(t, err)
}
