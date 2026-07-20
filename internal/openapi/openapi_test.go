package openapi

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Filename ("User.schema") differs from the ref name ("schemas/User"); the
// emitted component MUST be keyed "User" so a $ref: schemas/User resolves.
func TestGenerate_SchemaRegistryComponentNaming(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		SchemaRegistry: map[string]map[string]any{
			"schemas/User": {"type": "object", "properties": map[string]any{
				"id": map[string]any{"type": "string"},
			}},
		},
		Routes: map[string]map[string]any{
			"create": {
				"id": "create-user", "method": "POST", "path": "/users",
				"body": map[string]any{"schema": map[string]any{"$ref": "schemas/User"}},
			},
		},
	}
	doc, err := Generate(rc)
	require.NoError(t, err)
	require.NotNil(t, doc.Components)
	_, ok := doc.Components.Schemas["User"]
	assert.True(t, ok, "component should be keyed by ref name 'User', not file path")

	body := doc.Paths.Find("/users").Post.RequestBody.Value.Content["application/json"]
	assert.Equal(t, "#/components/schemas/User", body.Schema.Ref)
}

func TestScalarHTML_UsesSpecPath(t *testing.T) {
	html := ScalarHTML("/custom/spec.json")
	assert.Contains(t, html, `data-url="/custom/spec.json"`)
}

func TestGenerateOpenAPI_BasicRoutes(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"name":    "Test API",
			"version": "2.0.0",
		},
		Routes: map[string]map[string]any{
			"get-tasks": {
				"id":      "get-tasks",
				"method":  "GET",
				"path":    "/api/tasks",
				"summary": "List all tasks",
				"tags":    []any{"tasks"},
				"trigger": map[string]any{
					"workflow": "list-tasks",
				},
				"response": map[string]any{
					"200": map[string]any{
						"description": "Success",
					},
				},
			},
			"create-task": {
				"id":      "create-task",
				"method":  "POST",
				"path":    "/api/tasks",
				"summary": "Create a task",
				"tags":    []any{"tasks"},
				"body": map[string]any{
					"schema": map[string]any{
						"$ref": "schemas/Task",
					},
				},
				"trigger": map[string]any{
					"workflow": "create-task",
				},
			},
		},
		// NOTE: Generate reads rc.SchemaRegistry (ref-name keyed), not
		// rc.Schemas (file-path keyed) — see the SchemaRegistry fix this
		// package exists to carry. Ported from
		// internal/server/openapi_test.go with this field adjusted to match.
		SchemaRegistry: map[string]map[string]any{
			"schemas/Task": {
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{"type": "string"},
				},
			},
		},
	}

	doc, err := Generate(rc)
	require.NoError(t, err)

	assert.Equal(t, "3.1.0", doc.OpenAPI)
	assert.Equal(t, "Test API", doc.Info.Title)
	assert.Equal(t, "2.0.0", doc.Info.Version)

	// Check paths
	tasksPath := doc.Paths.Find("/api/tasks")
	require.NotNil(t, tasksPath)
	require.NotNil(t, tasksPath.Get)
	assert.Equal(t, "List all tasks", tasksPath.Get.Summary)
	assert.Contains(t, tasksPath.Get.Tags, "tasks")

	require.NotNil(t, tasksPath.Post)
	assert.Equal(t, "Create a task", tasksPath.Post.Summary)
	require.NotNil(t, tasksPath.Post.RequestBody)

	// Check component schemas
	require.NotNil(t, doc.Components)
	assert.Contains(t, doc.Components.Schemas, "Task")
}

func TestGenerateOpenAPI_PathParams(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"get-task": {
				"id":     "get-task",
				"method": "GET",
				"path":   "/api/tasks/:id",
				"trigger": map[string]any{
					"workflow": "get-task",
				},
			},
		},
	}

	doc, err := Generate(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/tasks/{id}")
	require.NotNil(t, pathItem)
	require.NotNil(t, pathItem.Get)
	require.Len(t, pathItem.Get.Parameters, 1)
	assert.Equal(t, "id", pathItem.Get.Parameters[0].Value.Name)
	assert.Equal(t, "path", pathItem.Get.Parameters[0].Value.In)
	assert.True(t, pathItem.Get.Parameters[0].Value.Required)
}

func TestGenerateOpenAPI_JWTSecurity(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"security": map[string]any{
				"jwt": map[string]any{
					"secret": "test",
				},
			},
		},
		Routes: map[string]map[string]any{
			"protected": {
				"id":         "protected",
				"method":     "GET",
				"path":       "/api/me",
				"middleware": []any{"auth.jwt"},
				"trigger": map[string]any{
					"workflow": "get-me",
				},
			},
		},
	}

	doc, err := Generate(rc)
	require.NoError(t, err)

	// Check security scheme
	require.NotNil(t, doc.Components)
	require.Contains(t, doc.Components.SecuritySchemes, "bearerAuth")
	scheme := doc.Components.SecuritySchemes["bearerAuth"].Value
	assert.Equal(t, "http", scheme.Type)
	assert.Equal(t, "bearer", scheme.Scheme)

	// Check route security
	pathItem := doc.Paths.Find("/api/me")
	require.NotNil(t, pathItem)
	require.NotNil(t, pathItem.Get.Security)
}

func TestFiberToOpenAPIPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/api/tasks/:id", "/api/tasks/{id}"},
		{"/api/users/:userId/posts/:postId", "/api/users/{userId}/posts/{postId}"},
		{"/api/tasks", "/api/tasks"},
		{"/api/tasks/:id?", "/api/tasks/{id}"},
	}

	for _, tt := range tests {
		result := fiberToOpenAPIPath(tt.input)
		assert.Equal(t, tt.expected, result, "for input %s", tt.input)
	}
}
