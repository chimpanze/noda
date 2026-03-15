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
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Full e2e: GET → transform.set → response.json with 200
func TestE2E_GET_TransformSetToResponseJSON(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"get-greeting": {
				"method":  "GET",
				"path":    "/greet/:name",
				"summary": "Greet user",
				"trigger": map[string]any{
					"workflow": "greet",
					"input": map[string]any{
						"name": "{{ params.name }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"greet": {
				"nodes": map[string]any{
					"build_greeting": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"greeting": "Hello",
								"name":     "{{ input.name }}",
							},
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ nodes.build_greeting }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "build_greeting", "to": "respond"},
				},
			},
		},
		nil,
	)

	req := httptest.NewRequest("GET", "/greet/Alice", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "Hello", result["greeting"])
	assert.Equal(t, "Alice", result["name"])
}

// Full e2e: POST → trigger mapping → transform.set → response.json with 201
func TestE2E_POST_TriggerMapping_ResponseJSON(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"create-item": {
				"method": "POST",
				"path":   "/items",
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
					"build": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"id":    "item-1",
								"name":  "{{ input.name }}",
								"price": "{{ input.price }}",
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

	reqBody := `{"name":"Widget","price":9.99}`
	req := httptest.NewRequest("POST", "/items", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "Widget", result["name"])
	assert.Equal(t, 9.99, result["price"])
}

// JWT middleware → $.auth available in workflow expressions
func TestE2E_JWT_AuthInWorkflow(t *testing.T) {
	secret := "e2e-secret-at-least-32-bytes-long-here!"
	srv := newTestServer(t,
		map[string]map[string]any{
			"me": {
				"method":     "GET",
				"path":       "/api/me",
				"middleware": []any{"auth.jwt"},
				"trigger": map[string]any{
					"workflow": "whoami",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"whoami": {
				"nodes": map[string]any{
					"build": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"user_id": "{{ auth.sub }}",
								"email":   "{{ auth.claims.email }}",
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

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "user-99",
		"email": "alice@test.com",
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte(secret))

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "user-99", result["user_id"])
	assert.Equal(t, "alice@test.com", result["email"])
}

// No response node → 202 Accepted
func TestE2E_NoResponseNode_202(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"webhook": {
				"method": "POST",
				"path":   "/webhook",
				"trigger": map[string]any{
					"workflow": "handle-webhook",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"handle-webhook": {
				"nodes": map[string]any{
					"log_event": map[string]any{
						"type": "util.log",
						"config": map[string]any{
							"level":   "info",
							"message": "webhook received",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("POST", "/webhook", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 202, resp.StatusCode)
}

// Workflow error → standardized error response with correct status
func TestE2E_WorkflowError_StandardizedResponse(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"broken": {
				"method": "GET",
				"path":   "/broken",
				"trigger": map[string]any{
					"workflow": "broken-wf",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"broken-wf": {
				"nodes": map[string]any{
					"bad": map[string]any{
						"type":   "nonexistent.type",
						"config": map[string]any{},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("GET", "/broken", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	errData := result["error"].(map[string]any)
	assert.Equal(t, "INTERNAL_ERROR", errData["code"])
	assert.NotEmpty(t, errData["trace_id"])
}

// Middleware chain order: auth before handler
func TestE2E_MiddlewareChainOrder(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"protected": {
				"method":     "GET",
				"path":       "/secret",
				"middleware": []any{"auth.jwt"},
				"trigger": map[string]any{
					"workflow": "secret-wf",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"secret-wf": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "\"secret data\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		map[string]any{
			"security": map[string]any{
				"jwt": map[string]any{
					"secret": "auth-test-secret-at-least-32-bytes!",
				},
			},
		},
	)

	// Without token → 401
	req := httptest.NewRequest("GET", "/secret", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	// With valid token → 200
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte("auth-test-secret-at-least-32-bytes!"))
	req = httptest.NewRequest("GET", "/secret", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// Route groups with preset middleware
func TestE2E_RouteGroupsWithPresets(t *testing.T) {
	secret := "group-secret-at-least-32-bytes-long-here!"
	srv := newTestServer(t,
		map[string]map[string]any{
			"admin-users": {
				"method": "GET",
				"path":   "/api/admin/users",
				"trigger": map[string]any{
					"workflow": "admin-users",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"admin-users": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "\"admin data\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		map[string]any{
			"security": map[string]any{
				"jwt": map[string]any{
					"secret": secret,
				},
			},
			"middleware_presets": map[string]any{
				"admin_only": []any{"auth.jwt"},
			},
			"route_groups": map[string]any{
				"/api/admin": map[string]any{
					"middleware_preset": "admin_only",
				},
			},
		},
	)

	// Without token → 401 (group middleware applied)
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	// With token → 200
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "admin-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte(secret))
	req = httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// OpenAPI spec endpoint returns valid spec
func TestE2E_OpenAPIEndpoint(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"name":    "E2E API",
			"version": "1.0.0",
		},
		Routes: map[string]map[string]any{
			"hello": {
				"id":      "hello",
				"method":  "GET",
				"path":    "/hello",
				"summary": "Say hello",
				"tags":    []any{"greeting"},
				"trigger": map[string]any{
					"workflow": "hello",
					"input":    map[string]any{},
				},
				"response": map[string]any{
					"200": map[string]any{
						"description": "Success",
					},
				},
			},
		},
		Workflows: map[string]map[string]any{
			"hello": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "\"hello\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		Schemas: map[string]map[string]any{},
	}

	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	require.NoError(t, srv.RegisterOpenAPIRoutes())

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(body, &doc))
	assert.Equal(t, "3.1.0", doc["openapi"])

	info := doc["info"].(map[string]any)
	assert.Equal(t, "E2E API", info["title"])
}

// Concurrent requests handled correctly
func TestE2E_ConcurrentRequests(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"echo": {
				"method": "POST",
				"path":   "/echo",
				"trigger": map[string]any{
					"workflow": "echo",
					"input": map[string]any{
						"data": "{{ body }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"echo": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ input.data }}",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	// Send 10 concurrent requests
	results := make(chan int, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			body := strings.NewReader(`{"n":` + strings.Repeat("1", n+1) + `}`)
			req := httptest.NewRequest("POST", "/echo", body)
			req.Header.Set("Content-Type", "application/json")
			resp, err := srv.App().Test(req)
			if err != nil {
				results <- -1
				return
			}
			results <- resp.StatusCode
		}(i)
	}

	for i := 0; i < 10; i++ {
		status := <-results
		assert.Equal(t, 200, status, "concurrent request %d", i)
	}
}

// DELETE method works
func TestE2E_DELETE_Method(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"delete-item": {
				"method": "DELETE",
				"path":   "/items/:id",
				"trigger": map[string]any{
					"workflow": "delete-item",
					"input": map[string]any{
						"id": "{{ params.id }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"delete-item": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ input }}",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("DELETE", "/items/42", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// PUT method works
func TestE2E_PUT_Method(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"update-item": {
				"method": "PUT",
				"path":   "/items/:id",
				"trigger": map[string]any{
					"workflow": "update-item",
					"input": map[string]any{
						"id":   "{{ params.id }}",
						"name": "{{ body.name }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"update-item": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ input }}",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("PUT", "/items/1", strings.NewReader(`{"name":"Updated"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, float64(1), result["id"])
	assert.Equal(t, "Updated", result["name"])
}
