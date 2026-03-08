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
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/util"
	"github.com/chimpanze/noda/plugins/core/workflow"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
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
	return nodeReg
}

func newTestServer(t *testing.T, routes map[string]map[string]any, workflows map[string]map[string]any, root map[string]any) *Server {
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
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
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
					"input":   map[string]any{},
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
							"body":   "{{ set_data }}",
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
						"title": "{{ request.body.title }}",
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
							"body":   "{{ build }}",
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
						"task_id": "{{ request.params.id }}",
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
							"body":   "{{ build }}",
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
	assert.Equal(t, "42", result["id"])
}

func TestRoute_NotFound(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"hello": {
				"method": "GET",
				"path":   "/hello",
				"trigger": map[string]any{
					"workflow": "hello",
					"input":   map[string]any{},
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
					"input":   map[string]any{},
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
					"input":   map[string]any{},
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
	secret := "test-jwt-secret"
	srv := newTestServer(t,
		map[string]map[string]any{
			"protected": {
				"method":     "GET",
				"path":       "/me",
				"middleware": []any{"auth.jwt"},
				"trigger": map[string]any{
					"workflow": "get-me",
					"input":   map[string]any{},
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
								"user_id": "{{ auth.userId }}",
							},
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ build }}",
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
					"input":   map[string]any{},
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

func TestRoute_ResponseError(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"not-found": {
				"method": "GET",
				"path":   "/missing",
				"trigger": map[string]any{
					"workflow": "not-found",
					"input":   map[string]any{},
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
