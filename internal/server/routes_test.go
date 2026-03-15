package server

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/trace"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/event"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/util"
	"github.com/chimpanze/noda/plugins/core/workflow"
	dbplugin "github.com/chimpanze/noda/plugins/db"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildTestNodeRegistry() *registry.NodeRegistry {
	nodeReg := registry.NewNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&control.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&transform.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&util.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&workflow.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&response.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&dbplugin.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&cacheplugin.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&event.Plugin{})
	return nodeReg
}

func newTestServer(t *testing.T, routes map[string]map[string]any, workflows map[string]map[string]any, root map[string]any, opts ...ServerOption) *Server {
	t.Helper()
	if root == nil {
		root = map[string]any{}
	}
	rc := &config.ResolvedConfig{
		Root:      root,
		Routes:    routes,
		Workflows: workflows,
		Schemas:   map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(), opts...)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	return srv
}

func TestRoute_GET_SimpleResponse(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"get-hello": {
				"method": "GET",
				"path":   "/hello",
				"trigger": map[string]any{
					"workflow": "hello",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"hello": {
				"nodes": map[string]any{
					"set_data": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"message": "Hello, World!",
							},
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ nodes.set_data }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "set_data", "to": "respond"},
				},
			},
		},
		nil,
	)

	req := httptest.NewRequest("GET", "/hello", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "Hello, World!", result["message"])
}

func TestRoute_POST_BodyMapping(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"create-task": {
				"method": "POST",
				"path":   "/tasks",
				"trigger": map[string]any{
					"workflow": "create-task",
					"input": map[string]any{
						"title": "{{ body.title }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"create-task": {
				"nodes": map[string]any{
					"build": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"id":    "task-1",
								"title": "{{ input.title }}",
							},
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "201",
							"body":   "{{ nodes.build }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "build", "to": "respond"},
				},
			},
		},
		nil,
	)

	reqBody := `{"title": "My Task"}`
	req := httptest.NewRequest("POST", "/tasks", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "My Task", result["title"])
	assert.Equal(t, "task-1", result["id"])
}

func TestRoute_PathParams(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"get-task": {
				"method": "GET",
				"path":   "/tasks/:id",
				"trigger": map[string]any{
					"workflow": "get-task",
					"input": map[string]any{
						"task_id": "{{ params.id }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"get-task": {
				"nodes": map[string]any{
					"build": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"id": "{{ input.task_id }}",
							},
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ nodes.build }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "build", "to": "respond"},
				},
			},
		},
		nil,
	)

	req := httptest.NewRequest("GET", "/tasks/42", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, float64(42), result["id"])
}

func TestRoute_NotFound(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"hello": {
				"method": "GET",
				"path":   "/hello",
				"trigger": map[string]any{
					"workflow": "hello",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"hello": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "\"ok\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("GET", "/unknown", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestRoute_NoResponseNode_202Accepted(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"fire-forget": {
				"method": "POST",
				"path":   "/fire",
				"trigger": map[string]any{
					"workflow": "fire-forget",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"fire-forget": {
				"nodes": map[string]any{
					"log-it": map[string]any{
						"type": "util.log",
						"config": map[string]any{
							"level":   "info",
							"message": "fire and forget",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("POST", "/fire", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 202, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "accepted", result["status"])
}

func TestRoute_WorkflowError_MappedResponse(t *testing.T) {
	// Use a workflow that references an unknown node type to trigger an error
	srv := newTestServer(t,
		map[string]map[string]any{
			"fail-route": {
				"method": "GET",
				"path":   "/fail",
				"trigger": map[string]any{
					"workflow": "failing",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"failing": {
				"nodes": map[string]any{
					"bad-node": map[string]any{
						"type":   "nonexistent.node",
						"config": map[string]any{},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("GET", "/fail", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	// Should get 500 since workflow compilation fails
	assert.Equal(t, 500, resp.StatusCode)
}

func TestRoute_JWTAuth_ClaimsAvailable(t *testing.T) {
	secret := "test-jwt-secret-at-least-32-bytes-long!"
	srv := newTestServer(t,
		map[string]map[string]any{
			"protected": {
				"method":     "GET",
				"path":       "/me",
				"middleware": []any{"auth.jwt"},
				"trigger": map[string]any{
					"workflow": "get-me",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"get-me": {
				"nodes": map[string]any{
					"build": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"user_id": "{{ auth.sub }}",
							},
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ nodes.build }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "build", "to": "respond"},
				},
			},
		},
		map[string]any{
			"security": map[string]any{
				"jwt": map[string]any{
					"secret": secret,
				},
			},
		},
	)

	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-42",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte(secret))

	req := httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "user-42", result["user_id"])
}

func TestRoute_ResponseRedirect(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"redirect": {
				"method": "GET",
				"path":   "/old",
				"trigger": map[string]any{
					"workflow": "redirect",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"redirect": {
				"nodes": map[string]any{
					"redir": map[string]any{
						"type": "response.redirect",
						"config": map[string]any{
							"url":    "/new",
							"status": float64(301),
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("GET", "/old", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 301, resp.StatusCode)
	assert.Equal(t, "/new", resp.Header.Get("Location"))
}

func TestRoute_BodySchemaValidation_RejectsInvalid(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"create-item": {
				"method": "POST",
				"path":   "/items",
				"body": map[string]any{
					"schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":  map[string]any{"type": "string"},
							"price": map[string]any{"type": "number"},
						},
						"required": []any{"name", "price"},
					},
				},
				"trigger": map[string]any{
					"workflow": "create-item",
					"input": map[string]any{
						"name":  "{{ body.name }}",
						"price": "{{ body.price }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"create-item": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "201",
							"body":   "\"created\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	// Missing required field "price"
	reqBody := `{"name": "Widget"}`
	req := httptest.NewRequest("POST", "/items", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	errData := result["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errData["code"])
	assert.NotEmpty(t, errData["details"].(map[string]any)["errors"])
}

func TestRoute_BodySchemaValidation_AcceptsValid(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"create-item": {
				"method": "POST",
				"path":   "/items",
				"body": map[string]any{
					"schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{"type": "string"},
						},
						"required": []any{"name"},
					},
				},
				"trigger": map[string]any{
					"workflow": "create-item",
					"input": map[string]any{
						"name": "{{ body.name }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"create-item": {
				"nodes": map[string]any{
					"build": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"name": "{{ input.name }}",
							},
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "201",
							"body":   "{{ nodes.build }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "build", "to": "respond"},
				},
			},
		},
		nil,
	)

	reqBody := `{"name": "Widget"}`
	req := httptest.NewRequest("POST", "/items", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "Widget", result["name"])
}

func TestRoute_BodySchemaValidation_DisabledWithValidateFalse(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"create-item": {
				"method": "POST",
				"path":   "/items",
				"body": map[string]any{
					"schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name": map[string]any{"type": "string"},
						},
						"required": []any{"name"},
					},
					"validate": false,
				},
				"trigger": map[string]any{
					"workflow": "create-item",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"create-item": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "\"ok\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	// Body missing required "name" but validation is disabled
	reqBody := `{"other": "value"}`
	req := httptest.NewRequest("POST", "/items", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRoute_NoBodySection_WorksAsUsual(t *testing.T) {
	// This is essentially the same as existing tests, but explicit about no body section
	srv := newTestServer(t,
		map[string]map[string]any{
			"simple": {
				"method": "POST",
				"path":   "/simple",
				"trigger": map[string]any{
					"workflow": "simple",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"simple": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "\"ok\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	reqBody := `{"anything": "goes"}`
	req := httptest.NewRequest("POST", "/simple", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRoute_ResponseError(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"not-found": {
				"method": "GET",
				"path":   "/missing",
				"trigger": map[string]any{
					"workflow": "not-found",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"not-found": {
				"nodes": map[string]any{
					"err": map[string]any{
						"type": "response.error",
						"config": map[string]any{
							"status":  "404",
							"code":    "NOT_FOUND",
							"message": "Resource not found",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("GET", "/missing", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	errData := result["error"].(map[string]any)
	assert.Equal(t, "NOT_FOUND", errData["code"])
	assert.Equal(t, "Resource not found", errData["message"])
	assert.NotEmpty(t, errData["trace_id"])
}

// responseValidationRoute returns a route config with a response schema requiring {"id": integer, "name": string}.
func responseValidationRoute(validate any) map[string]any {
	route := map[string]any{
		"method": "GET",
		"path":   "/item",
		"trigger": map[string]any{
			"workflow": "get-item",
			"input":    map[string]any{},
		},
		"response": map[string]any{
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
		},
	}
	if validate != nil {
		route["response"].(map[string]any)["validate"] = validate
	}
	return route
}

// mismatchedWorkflow returns a workflow that responds with {"wrong": "data"} — missing required id/name.
func mismatchedWorkflow() map[string]any {
	return map[string]any{
		"nodes": map[string]any{
			"build": map[string]any{
				"type": "transform.set",
				"config": map[string]any{
					"fields": map[string]any{"wrong": "data"},
				},
			},
			"respond": map[string]any{
				"type": "response.json",
				"config": map[string]any{
					"status": "200",
					"body":   "{{ nodes.build }}",
				},
			},
		},
		"edges": []any{
			map[string]any{"from": "build", "to": "respond"},
		},
	}
}

// validWorkflow returns a workflow that responds with {"id": 1, "name": "Widget"}.
func validWorkflow() map[string]any {
	return map[string]any{
		"nodes": map[string]any{
			"build": map[string]any{
				"type": "transform.set",
				"config": map[string]any{
					"fields": map[string]any{"id": 1, "name": "Widget"},
				},
			},
			"respond": map[string]any{
				"type": "response.json",
				"config": map[string]any{
					"status": "200",
					"body":   "{{ nodes.build }}",
				},
			},
		},
		"edges": []any{
			map[string]any{"from": "build", "to": "respond"},
		},
	}
}

func TestResponseValidation_DevMode_WarnsButSendsResponse(t *testing.T) {
	hub := trace.NewEventHub()
	srv := newTestServer(t,
		map[string]map[string]any{"get-item": responseValidationRoute(nil)},
		map[string]map[string]any{"get-item": mismatchedWorkflow()},
		nil,
		WithTraceHub(hub),
	)

	req := httptest.NewRequest("GET", "/item", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	// Default mode in dev: warn but still send original response
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "data", result["wrong"])
}

func TestResponseValidation_Strict_Returns500(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{"get-item": responseValidationRoute("strict")},
		map[string]map[string]any{"get-item": mismatchedWorkflow()},
		nil,
	)

	req := httptest.NewRequest("GET", "/item", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	errData := result["error"].(map[string]any)
	assert.Equal(t, "RESPONSE_VALIDATION_ERROR", errData["code"])
}

func TestResponseValidation_WarnMode_Production(t *testing.T) {
	// No traceHub (production), but validate: "warn" — still sends original response
	srv := newTestServer(t,
		map[string]map[string]any{"get-item": responseValidationRoute("warn")},
		map[string]map[string]any{"get-item": mismatchedWorkflow()},
		nil,
	)

	req := httptest.NewRequest("GET", "/item", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "data", result["wrong"])
}

func TestResponseValidation_Disabled(t *testing.T) {
	hub := trace.NewEventHub()
	srv := newTestServer(t,
		map[string]map[string]any{"get-item": responseValidationRoute(false)},
		map[string]map[string]any{"get-item": mismatchedWorkflow()},
		nil,
		WithTraceHub(hub),
	)

	req := httptest.NewRequest("GET", "/item", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	// Disabled — no validation even in dev mode
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "data", result["wrong"])
}

func TestResponseValidation_ValidResponse_PassesThrough(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{"get-item": responseValidationRoute("strict")},
		map[string]map[string]any{"get-item": validWorkflow()},
		nil,
	)

	req := httptest.NewRequest("GET", "/item", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "Widget", result["name"])
}
