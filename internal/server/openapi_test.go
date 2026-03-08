package server

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		Schemas: map[string]map[string]any{
			"Task": {
				"type": "object",
				"properties": map[string]any{
					"title": map[string]any{"type": "string"},
				},
			},
		},
	}

	doc, err := GenerateOpenAPI(rc)
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
		Schemas: map[string]map[string]any{},
	}

	doc, err := GenerateOpenAPI(rc)
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
		Schemas: map[string]map[string]any{},
	}

	doc, err := GenerateOpenAPI(rc)
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

func TestOpenAPIEndpoint(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"hello": {
				"id":     "hello",
				"method": "GET",
				"path":   "/hello",
				"trigger": map[string]any{
					"workflow": "hello",
					"input":   map[string]any{},
				},
			},
		},
		Workflows: map[string]map[string]any{
			"hello": {
				"nodes": map[string]any{},
				"edges": []any{},
			},
		},
		Schemas: map[string]map[string]any{},
	}

	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	require.NoError(t, srv.RegisterOpenAPIRoutes())

	// Test /openapi.json endpoint
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "application/json")

	body, _ := io.ReadAll(resp.Body)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(body, &doc))
	assert.Equal(t, "3.1.0", doc["openapi"])

	// Test /docs endpoint
	req = httptest.NewRequest("GET", "/docs", nil)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/html")
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
