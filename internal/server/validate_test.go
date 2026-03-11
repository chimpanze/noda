package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBodyValidator_ValidBody(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  map[string]any{"type": "string"},
			"email": map[string]any{"type": "string"},
		},
		"required": []any{"name"},
	}

	v := newBodyValidator(schema)
	err := v.Validate(map[string]any{"name": "Alice", "email": "alice@test.com"})
	assert.NoError(t, err)
}

func TestBodyValidator_InvalidBody_MissingRequired(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []any{"name"},
	}

	v := newBodyValidator(schema)
	err := v.Validate(map[string]any{"email": "alice@test.com"})
	require.Error(t, err)

	bve, ok := err.(*bodyValidationError)
	require.True(t, ok, "expected bodyValidationError")
	assert.NotEmpty(t, bve.Errors)
	assert.Contains(t, bve.Errors[0].Message, "name")
}

func TestBodyValidator_InvalidBody_WrongType(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"age": map[string]any{"type": "integer"},
		},
	}

	v := newBodyValidator(schema)
	err := v.Validate(map[string]any{"age": "not-a-number"})
	require.Error(t, err)

	bve, ok := err.(*bodyValidationError)
	require.True(t, ok, "expected bodyValidationError")
	assert.NotEmpty(t, bve.Errors)
	assert.Equal(t, "/age", bve.Errors[0].Field)
}

func TestBodyValidator_MultipleErrors(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
			"age":  map[string]any{"type": "integer"},
		},
		"required": []any{"name"},
	}

	v := newBodyValidator(schema)
	// Missing required "name" AND wrong type for "age"
	err := v.Validate(map[string]any{"age": "not-a-number"})
	require.Error(t, err)

	bve, ok := err.(*bodyValidationError)
	require.True(t, ok)
	assert.GreaterOrEqual(t, len(bve.Errors), 2)
}

func TestBodyValidator_NilSchema(t *testing.T) {
	v := newBodyValidator(nil)
	err := v.Validate(map[string]any{"foo": "bar"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil schema")
}

func TestBodyValidator_NilBody(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []any{"name"},
	}

	v := newBodyValidator(schema)
	err := v.Validate(nil)
	require.Error(t, err)
}

func TestBodyValidator_ErrorMessage_Single(t *testing.T) {
	bve := &bodyValidationError{
		Errors: []bodyValidationDetail{
			{Field: "/name", Message: "missing property"},
		},
	}
	assert.Equal(t, "body validation failed: /name: missing property", bve.Error())
}

func TestBodyValidator_ErrorMessage_Multiple(t *testing.T) {
	bve := &bodyValidationError{
		Errors: []bodyValidationDetail{
			{Field: "/name", Message: "missing"},
			{Field: "/age", Message: "missing"},
		},
	}
	assert.Equal(t, "body validation failed: 2 errors", bve.Error())
}

func TestResponseValidator_ValidBody(t *testing.T) {
	cfg := map[string]any{
		"200": map[string]any{
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":   map[string]any{"type": "integer"},
					"name": map[string]any{"type": "string"},
				},
				"required": []any{"id", "name"},
			},
		},
	}
	rv := newResponseValidator(cfg, "strict")
	err := rv.ValidateResponse(200, map[string]any{"id": 1, "name": "Alice"})
	assert.NoError(t, err)
}

func TestResponseValidator_InvalidBody(t *testing.T) {
	cfg := map[string]any{
		"200": map[string]any{
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":   map[string]any{"type": "integer"},
					"name": map[string]any{"type": "string"},
				},
				"required": []any{"id", "name"},
			},
		},
	}
	rv := newResponseValidator(cfg, "strict")
	err := rv.ValidateResponse(200, map[string]any{"id": 1})
	require.Error(t, err)
}

func TestResponseValidator_NoSchemaForStatus(t *testing.T) {
	cfg := map[string]any{
		"200": map[string]any{
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer"},
				},
				"required": []any{"id"},
			},
		},
	}
	rv := newResponseValidator(cfg, "strict")
	// 201 has no schema — should pass
	err := rv.ValidateResponse(201, map[string]any{"anything": "goes"})
	assert.NoError(t, err)
}

func TestResponseValidator_MultipleStatuses(t *testing.T) {
	cfg := map[string]any{
		"200": map[string]any{
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"items": map[string]any{"type": "array"},
				},
				"required": []any{"items"},
			},
		},
		"201": map[string]any{
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "integer"},
				},
				"required": []any{"id"},
			},
		},
	}
	rv := newResponseValidator(cfg, "warn")

	// 200 valid
	assert.NoError(t, rv.ValidateResponse(200, map[string]any{"items": []any{}}))
	// 201 valid
	assert.NoError(t, rv.ValidateResponse(201, map[string]any{"id": 42}))
	// 200 invalid
	assert.Error(t, rv.ValidateResponse(200, map[string]any{"wrong": "field"}))
	// 201 invalid
	assert.Error(t, rv.ValidateResponse(201, map[string]any{"wrong": "field"}))
}

func TestResponseValidator_EmptyConfig(t *testing.T) {
	cfg := map[string]any{
		"validate": "warn",
	}
	rv := newResponseValidator(cfg, "warn")
	assert.Empty(t, rv.schemas)
	// No schemas → always nil
	assert.NoError(t, rv.ValidateResponse(200, map[string]any{"anything": true}))
}
