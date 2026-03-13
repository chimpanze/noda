package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRefs_Simple(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/User.json": {
				"User": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
					},
				},
			},
		},
		Routes: map[string]map[string]any{
			"routes/users.json": {
				"response": map[string]any{
					"$ref": "schemas/User",
				},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	assert.Empty(t, errs)

	response := rc.Routes["routes/users.json"]["response"].(map[string]any)
	assert.Equal(t, "object", response["type"])
	assert.NotNil(t, response["properties"])
}

func TestResolveRefs_InWorkflow(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/Task.json": {
				"Task": map[string]any{
					"type": "object",
				},
			},
		},
		Routes: map[string]map[string]any{},
		Workflows: map[string]map[string]any{
			"workflows/create.json": {
				"nodes": map[string]any{
					"validate": map[string]any{
						"type":   "transform.validate",
						"config": map[string]any{"schema": map[string]any{"$ref": "schemas/Task"}},
					},
				},
			},
		},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	assert.Empty(t, errs)

	schema := rc.Workflows["workflows/create.json"]["nodes"].(map[string]any)["validate"].(map[string]any)["config"].(map[string]any)["schema"].(map[string]any)
	assert.Equal(t, "object", schema["type"])
}

func TestResolveRefs_NestedRef(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/common.json": {
				"Pagination": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"page": map[string]any{"type": "integer"},
					},
				},
				"PaginatedList": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pagination": map[string]any{"$ref": "schemas/Pagination"},
						"items":      map[string]any{"type": "array"},
					},
				},
			},
		},
		Routes: map[string]map[string]any{
			"routes/list.json": {
				"response": map[string]any{"$ref": "schemas/PaginatedList"},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	assert.Empty(t, errs)

	response := rc.Routes["routes/list.json"]["response"].(map[string]any)
	props := response["properties"].(map[string]any)
	pagination := props["pagination"].(map[string]any)
	assert.Equal(t, "object", pagination["type"])
}

func TestResolveRefs_CircularRef(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/a.json": {
				"A": map[string]any{"child": map[string]any{"$ref": "schemas/B"}},
				"B": map[string]any{"child": map[string]any{"$ref": "schemas/A"}},
			},
		},
		Routes: map[string]map[string]any{
			"routes/test.json": {
				"schema": map[string]any{"$ref": "schemas/A"},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "circular")
}

func TestResolveRefs_MissingRef(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{},
		Routes: map[string]map[string]any{
			"routes/test.json": {
				"schema": map[string]any{"$ref": "schemas/NonExistent"},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "schemas/NonExistent")
	assert.Contains(t, errs[0].Error(), "routes/test.json")
}

func TestResolveRefs_MultipleDefinitionsFromOneFile(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/models.json": {
				"User": map[string]any{"type": "user"},
				"Task": map[string]any{"type": "task"},
			},
		},
		Routes: map[string]map[string]any{
			"routes/a.json": {"schema": map[string]any{"$ref": "schemas/User"}},
			"routes/b.json": {"schema": map[string]any{"$ref": "schemas/Task"}},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	assert.Empty(t, errs)

	assert.Equal(t, "user", rc.Routes["routes/a.json"]["schema"].(map[string]any)["type"])
	assert.Equal(t, "task", rc.Routes["routes/b.json"]["schema"].(map[string]any)["type"])
}

func TestResolveRefs_SubfolderSchema(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/validation/CreateTask.json": {
				"CreateTask": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{"type": "string"},
					},
				},
			},
			"project/schemas/models/Task.json": {
				"Task": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":    map[string]any{"type": "string"},
						"title": map[string]any{"type": "string"},
					},
				},
			},
			// Flat schema still works
			"project/schemas/Common.json": {
				"Pagination": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"page": map[string]any{"type": "integer"},
					},
				},
			},
		},
		Routes: map[string]map[string]any{
			"routes/create.json": {
				"body":     map[string]any{"$ref": "schemas/validation/CreateTask"},
				"response": map[string]any{"$ref": "schemas/models/Task"},
			},
			"routes/list.json": {
				"pagination": map[string]any{"$ref": "schemas/Pagination"},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	assert.Empty(t, errs)

	// Subfolder ref: schemas/validation/CreateTask
	body := rc.Routes["routes/create.json"]["body"].(map[string]any)
	assert.Equal(t, "object", body["type"])
	assert.NotNil(t, body["properties"])

	// Subfolder ref: schemas/models/Task
	response := rc.Routes["routes/create.json"]["response"].(map[string]any)
	assert.Equal(t, "object", response["type"])

	// Flat ref still works: schemas/Pagination
	pagination := rc.Routes["routes/list.json"]["pagination"].(map[string]any)
	assert.Equal(t, "object", pagination["type"])
}

func TestExtractSchemasRelPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/project/schemas/Task.json", "schemas"},
		{"/project/schemas/validation/CreateTask.json", "schemas/validation"},
		{"/project/schemas/models/db/User.json", "schemas/models/db"},
		{"schemas/Task.json", "schemas"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractSchemasRelPath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
