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
