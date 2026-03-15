package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidate_ValidRoute(t *testing.T) {
	rc := &RawConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"routes/tasks.json": {
				"id":     "list-tasks",
				"method": "GET",
				"path":   "/api/tasks",
				"trigger": map[string]any{
					"workflow": "list-tasks",
				},
			},
		},
		Schemas:     map[string]map[string]any{},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := Validate(rc)
	assert.Empty(t, errs)
}

func TestValidate_MissingRequiredField(t *testing.T) {
	rc := &RawConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"routes/tasks.json": {
				"id":   "list-tasks",
				"path": "/api/tasks",
				// missing method and trigger
			},
		},
		Schemas:     map[string]map[string]any{},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := Validate(rc)
	assert.NotEmpty(t, errs)

	// Should report missing fields
	hasMethodErr := false
	hasTriggerErr := false
	for _, e := range errs {
		assert.Equal(t, "routes/tasks.json", e.FilePath)
		if contains(e.Message, "method") {
			hasMethodErr = true
		}
		if contains(e.Message, "trigger") {
			hasTriggerErr = true
		}
	}
	assert.True(t, hasMethodErr || hasTriggerErr, "should report missing required fields")
}

func TestValidate_WrongType(t *testing.T) {
	rc := &RawConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"routes/tasks.json": {
				"id":     123, // should be string
				"method": "GET",
				"path":   "/api/tasks",
				"trigger": map[string]any{
					"workflow": "list-tasks",
				},
			},
		},
		Schemas:     map[string]map[string]any{},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := Validate(rc)
	assert.NotEmpty(t, errs)
}

func TestValidate_MultipleErrorsAcrossFiles(t *testing.T) {
	rc := &RawConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"routes/a.json": {"id": "a"},       // missing method, path, trigger
			"routes/b.json": {"method": "GET"}, // missing id, path, trigger
		},
		Schemas:     map[string]map[string]any{},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := Validate(rc)
	assert.True(t, len(errs) >= 2, "should have errors from multiple files")

	fileA := false
	fileB := false
	for _, e := range errs {
		if e.FilePath == "routes/a.json" {
			fileA = true
		}
		if e.FilePath == "routes/b.json" {
			fileB = true
		}
	}
	assert.True(t, fileA, "should have errors from file a")
	assert.True(t, fileB, "should have errors from file b")
}

func TestValidate_ValidWorkflow(t *testing.T) {
	rc := &RawConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"workflows/create.json": {
				"id": "create-task",
				"nodes": map[string]any{
					"validate": map[string]any{
						"type": "transform.validate",
					},
				},
				"edges": []any{},
			},
		},
		Schemas:     map[string]map[string]any{},
		Routes:      map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := Validate(rc)
	assert.Empty(t, errs)
}

func TestValidate_ExtraFieldsAllowed(t *testing.T) {
	rc := &RawConfig{
		Root: map[string]any{
			"services":     map[string]any{},
			"custom_field": "should not cause error",
		},
		Schemas:     map[string]map[string]any{},
		Routes:      map[string]map[string]any{},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := Validate(rc)
	assert.Empty(t, errs)
}

func TestValidate_ValidModel(t *testing.T) {
	rc := &RawConfig{
		Root: map[string]any{},
		Models: map[string]map[string]any{
			"models/user.json": {
				"table": "users",
				"columns": map[string]any{
					"id":   map[string]any{"type": "uuid", "primary_key": true},
					"name": map[string]any{"type": "text", "not_null": true},
				},
			},
		},
		Schemas:     map[string]map[string]any{},
		Routes:      map[string]map[string]any{},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := Validate(rc)
	assert.Empty(t, errs)
}

func TestValidate_InvalidModel_MissingTable(t *testing.T) {
	rc := &RawConfig{
		Root: map[string]any{},
		Models: map[string]map[string]any{
			"models/bad.json": {
				"columns": map[string]any{
					"id": map[string]any{"type": "uuid"},
				},
			},
		},
		Schemas:     map[string]map[string]any{},
		Routes:      map[string]map[string]any{},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := Validate(rc)
	assert.NotEmpty(t, errs)
	assert.Equal(t, "models/bad.json", errs[0].FilePath)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
