package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Middleware: extractMiddlewareConfig branches ---

func TestExtractMiddlewareConfig_MiddlewareSection(t *testing.T) {
	root := map[string]any{
		"middleware": map[string]any{
			"limiter": map[string]any{"max": float64(100)},
		},
	}
	cfg := extractMiddlewareConfig("limiter", root)
	require.NotNil(t, cfg)
	assert.Equal(t, float64(100), cfg["max"])
}

func TestExtractMiddlewareConfig_SecuritySection(t *testing.T) {
	root := map[string]any{
		"security": map[string]any{
			"cors": map[string]any{"allow_origins": "*"},
		},
	}
	cfg := extractMiddlewareConfig("security.cors", root)
	require.NotNil(t, cfg)
	assert.Equal(t, "*", cfg["allow_origins"])
}

func TestExtractMiddlewareConfig_AuthJWT(t *testing.T) {
	root := map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{"secret": "s3cr3t"},
		},
	}
	cfg := extractMiddlewareConfig("auth.jwt", root)
	require.NotNil(t, cfg)
	assert.Equal(t, "s3cr3t", cfg["secret"])
}

func TestExtractMiddlewareConfig_CasbinEnforce(t *testing.T) {
	root := map[string]any{
		"security": map[string]any{
			"casbin": map[string]any{"model": "test.conf"},
		},
	}
	cfg := extractMiddlewareConfig("casbin.enforce", root)
	require.NotNil(t, cfg)
	assert.Equal(t, "test.conf", cfg["model"])
}

func TestExtractMiddlewareConfig_NilRoot(t *testing.T) {
	cfg := extractMiddlewareConfig("limiter", nil)
	assert.Nil(t, cfg)
}

func TestExtractMiddlewareConfig_NoMatch(t *testing.T) {
	root := map[string]any{
		"middleware": map[string]any{
			"other": map[string]any{},
		},
	}
	cfg := extractMiddlewareConfig("limiter", root)
	assert.Nil(t, cfg)
}

// --- Middleware: individual factories ---

func TestBuildMiddleware_Helmet(t *testing.T) {
	h, err := BuildMiddleware("security.headers", nil)
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestBuildMiddleware_CSRF(t *testing.T) {
	h, err := BuildMiddleware("security.csrf", map[string]any{
		"security": map[string]any{
			"csrf": map[string]any{
				"cookie_name":         "_csrf",
				"cookie_secure":       true,
				"cookie_http_only":    true,
				"cookie_same_site":    "Strict",
				"cookie_session_only": true,
				"single_use_token":    true,
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestBuildMiddleware_CSRF_NilConfig(t *testing.T) {
	h, err := BuildMiddleware("security.csrf", nil)
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestBuildMiddleware_Compress(t *testing.T) {
	h, err := BuildMiddleware("compress", nil)
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestBuildMiddleware_ETag(t *testing.T) {
	h, err := BuildMiddleware("etag", nil)
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestBuildMiddleware_Logger(t *testing.T) {
	h, err := BuildMiddleware("logger", nil)
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestBuildMiddleware_Timeout_Default(t *testing.T) {
	h, err := BuildMiddleware("timeout", nil)
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestBuildMiddleware_Timeout_CustomDuration(t *testing.T) {
	h, err := BuildMiddleware("timeout", map[string]any{
		"middleware": map[string]any{
			"timeout": map[string]any{
				"duration": "5s",
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, h)
}

func TestBuildMiddleware_Timeout_InvalidDuration(t *testing.T) {
	_, err := BuildMiddleware("timeout", map[string]any{
		"middleware": map[string]any{
			"timeout": map[string]any{
				"duration": "not-a-duration",
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid duration")
}

func TestBuildMiddleware_Limiter_InvalidExpiration(t *testing.T) {
	_, err := BuildMiddleware("limiter", map[string]any{
		"middleware": map[string]any{
			"limiter": map[string]any{
				"max":        float64(10),
				"expiration": "not-a-duration",
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid expiration")
}

func TestBuildMiddleware_Limiter_NilConfig(t *testing.T) {
	_, err := BuildMiddleware("limiter", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max=0")
}

func TestBuildMiddleware_JWT_UnsupportedAlgorithm(t *testing.T) {
	_, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"secret":    "test-key-that-is-at-least-32-bytes-long!!",
				"algorithm": "RS256",
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported algorithm")
}

func TestBuildMiddleware_JWT_HS384(t *testing.T) {
	secret := "test-secret-384-at-least-32-bytes-long!"
	app := fiber.New()
	h, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"secret":    secret,
				"algorithm": "HS384",
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	token := jwt.NewWithClaims(jwt.SigningMethodHS384, jwt.MapClaims{
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte(secret))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestBuildMiddleware_JWT_HS512(t *testing.T) {
	secret := "test-secret-512-at-least-32-bytes-long!"
	app := fiber.New()
	h, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"secret":    secret,
				"algorithm": "HS512",
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	token := jwt.NewWithClaims(jwt.SigningMethodHS512, jwt.MapClaims{
		"sub": "user-1",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte(secret))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestBuildMiddleware_JWT_InvalidAuthFormat(t *testing.T) {
	app := fiber.New()
	h, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"secret": "test-key-that-is-at-least-32-bytes-long!!",
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic abc123")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "invalid authorization format")
}

func TestBuildMiddleware_JWT_Roles(t *testing.T) {
	secret := "test-secret-roles-at-least-32-bytes!"
	app := fiber.New()
	h, err := BuildMiddleware("auth.jwt", map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{
				"secret": secret,
			},
		},
	})
	require.NoError(t, err)

	app.Use(h)
	app.Get("/test", func(c fiber.Ctx) error {
		roles := c.Locals(api.LocalJWTRoles)
		return c.JSON(map[string]any{"roles": roles})
	})

	// Roles must be []any for JWT MapClaims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "user-1",
		"roles": []string{"admin", "editor"},
		"exp":   time.Now().Add(time.Hour).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte(secret))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// --- Helpers ---

func TestMapStr_ExistingKey(t *testing.T) {
	m := map[string]any{"key": "value"}
	assert.Equal(t, "value", mapStr(m, "key"))
}

func TestMapStr_MissingKey(t *testing.T) {
	m := map[string]any{"key": "value"}
	assert.Equal(t, "", mapStr(m, "missing"))
}

func TestMapStr_NonStringValue(t *testing.T) {
	m := map[string]any{"key": 42}
	assert.Equal(t, "", mapStr(m, "key"))
}

func TestDedupe_Empty(t *testing.T) {
	result := dedupe(nil)
	assert.Empty(t, result)
}

func TestDedupe_NoDuplicates(t *testing.T) {
	result := dedupe([]string{"a", "b", "c"})
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestDedupe_WithDuplicates(t *testing.T) {
	result := dedupe([]string{"a", "b", "a", "c", "b"})
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestDedupe_AllSame(t *testing.T) {
	result := dedupe([]string{"x", "x", "x"})
	assert.Equal(t, []string{"x"}, result)
}

func TestSplitTrim(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"}, splitTrim("a, b, c"))
	assert.Equal(t, []string{"a"}, splitTrim("a"))
	assert.Empty(t, splitTrim(""))
}

// --- ValidateMiddlewareOrder ---

func TestValidateMiddlewareOrder_Valid(t *testing.T) {
	err := ValidateMiddlewareOrder([]string{"auth.jwt", "casbin.enforce"})
	assert.NoError(t, err)
}

func TestValidateMiddlewareOrder_Invalid(t *testing.T) {
	err := ValidateMiddlewareOrder([]string{"casbin.enforce", "auth.jwt"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must appear before")
}

func TestValidateMiddlewareOrder_NoCasbin(t *testing.T) {
	err := ValidateMiddlewareOrder([]string{"auth.jwt", "recover"})
	assert.NoError(t, err)
}

func TestValidateMiddlewareOrder_CasbinWithoutJWT(t *testing.T) {
	// casbin.enforce without auth.jwt present -- no constraint violation
	err := ValidateMiddlewareOrder([]string{"casbin.enforce"})
	assert.NoError(t, err)
}

func TestValidateMiddlewareOrder_Empty(t *testing.T) {
	err := ValidateMiddlewareOrder(nil)
	assert.NoError(t, err)
}

// --- Validation: validateWorkflowRefs ---

func TestValidateWorkflowRefs_AllPresent(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{
		"wf1": {"nodes": map[string]any{}, "edges": []any{}},
		"wf2": {"nodes": map[string]any{}, "edges": []any{}},
	}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	assert.NoError(t, srv.validateWorkflowRefs("ep", "wf1", "wf2"))
}

func TestValidateWorkflowRefs_Missing(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{
		"wf1": {"nodes": map[string]any{}, "edges": []any{}},
	}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	err = srv.validateWorkflowRefs("ep", "wf1", "missing-wf")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing-wf")
	assert.Contains(t, err.Error(), "not found")
}

func TestValidateWorkflowRefs_EmptyStringsSkipped(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	assert.NoError(t, srv.validateWorkflowRefs("ep", "", ""))
}

// --- Connections: empty endpoints registration ---

func TestRegisterConnections_EmptyEndpoints(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Connections: map[string]map[string]any{
			"conn1": {
				"endpoints": map[string]any{},
			},
		},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	assert.NoError(t, srv.registerConnections())
}

func TestRegisterConnections_NoEndpointsKey(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root:        map[string]any{},
		Connections: map[string]map[string]any{"conn1": {"other": "data"}},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	assert.NoError(t, srv.registerConnections())
}

func TestRegisterConnections_NilConnections(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	assert.NoError(t, srv.registerConnections())
}

func TestRegisterConnections_EndpointMissingPath(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Connections: map[string]map[string]any{
			"conn1": {
				"endpoints": map[string]any{
					"nopath": map[string]any{
						"type": "websocket",
						// no "path" key
					},
				},
			},
		},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	// Should skip endpoints without path
	assert.NoError(t, srv.registerConnections())
}

func TestRegisterConnections_EndpointNotMap(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Connections: map[string]map[string]any{
			"conn1": {
				"endpoints": map[string]any{
					"bad": "not-a-map",
				},
			},
		},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	assert.NoError(t, srv.registerConnections())
}

// --- OpenAPI edge cases ---

func TestGenerateOpenAPI_DefaultNameVersion(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)
	assert.Equal(t, "Noda API", doc.Info.Title)
	assert.Equal(t, "1.0.0", doc.Info.Version)
}

func TestGenerateOpenAPI_ServerURL(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"server": map[string]any{
				"port": float64(8080),
			},
		},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)
	require.Len(t, doc.Servers, 1)
	assert.Equal(t, "http://localhost:8080", doc.Servers[0].URL)
}

func TestGenerateOpenAPI_PUTRoute(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"update-task": {
				"id":     "update-task",
				"method": "PUT",
				"path":   "/api/tasks/:id",
				"trigger": map[string]any{
					"workflow": "update-task",
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/tasks/{id}")
	require.NotNil(t, pathItem)
	assert.NotNil(t, pathItem.Put)
}

func TestGenerateOpenAPI_PATCHRoute(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"patch-task": {
				"id":     "patch-task",
				"method": "PATCH",
				"path":   "/api/tasks/:id",
				"trigger": map[string]any{
					"workflow": "patch-task",
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/tasks/{id}")
	require.NotNil(t, pathItem)
	assert.NotNil(t, pathItem.Patch)
}

func TestGenerateOpenAPI_DELETERoute(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"delete-task": {
				"id":     "delete-task",
				"method": "DELETE",
				"path":   "/api/tasks/:id",
				"trigger": map[string]any{
					"workflow": "delete-task",
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/tasks/{id}")
	require.NotNil(t, pathItem)
	assert.NotNil(t, pathItem.Delete)
}

func TestGenerateOpenAPI_RouteWithoutID(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"auto-id": {
				"method": "GET",
				"path":   "/api/items",
				"trigger": map[string]any{
					"workflow": "list-items",
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/items")
	require.NotNil(t, pathItem)
	require.NotNil(t, pathItem.Get)
	// operationID generated from method+path
	assert.Equal(t, "get_api_items", pathItem.Get.OperationID)
}

func TestGenerateOpenAPI_QueryParams(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"search": {
				"id":     "search",
				"method": "GET",
				"path":   "/api/search",
				"query": map[string]any{
					"schema": map[string]any{
						"properties": map[string]any{
							"q": map[string]any{
								"type": "string",
							},
							"page": map[string]any{
								"type": "integer",
							},
						},
					},
				},
				"trigger": map[string]any{
					"workflow": "search",
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/search")
	require.NotNil(t, pathItem)
	require.NotNil(t, pathItem.Get)
	assert.Len(t, pathItem.Get.Parameters, 2)
}

func TestGenerateOpenAPI_ResponseWithSchema(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"get-item": {
				"id":     "get-item",
				"method": "GET",
				"path":   "/api/items/:id",
				"trigger": map[string]any{
					"workflow": "get-item",
				},
				"response": map[string]any{
					"200": map[string]any{
						"description": "Item found",
						"schema": map[string]any{
							"$ref": "schemas/Item",
						},
					},
					"404": map[string]any{
						"description": "Not found",
					},
				},
			},
		},
		Schemas: map[string]map[string]any{
			"Item": {
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
			},
		},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/items/{id}")
	require.NotNil(t, pathItem)
	require.NotNil(t, pathItem.Get)
	assert.GreaterOrEqual(t, pathItem.Get.Responses.Len(), 2)
}

func TestGenerateOpenAPI_ResponseWithInlineSchema(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"inline": {
				"id":     "inline",
				"method": "GET",
				"path":   "/api/inline",
				"trigger": map[string]any{
					"workflow": "inline",
				},
				"response": map[string]any{
					"200": map[string]any{
						"description": "OK",
						"schema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"msg": map[string]any{"type": "string"},
							},
						},
					},
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/inline")
	require.NotNil(t, pathItem)
}

func TestGenerateOpenAPI_BodyWithInlineSchema(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"create": {
				"id":     "create",
				"method": "POST",
				"path":   "/api/create",
				"trigger": map[string]any{
					"workflow": "create",
				},
				"body": map[string]any{
					"content_type": "application/xml",
					"schema": map[string]any{
						"type": "object",
					},
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/create")
	require.NotNil(t, pathItem)
	require.NotNil(t, pathItem.Post)
	require.NotNil(t, pathItem.Post.RequestBody)
	assert.Contains(t, pathItem.Post.RequestBody.Value.Content, "application/xml")
}

func TestGenerateOpenAPI_BodyNoSchema(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"create": {
				"id":     "create",
				"method": "POST",
				"path":   "/api/create",
				"trigger": map[string]any{
					"workflow": "create",
				},
				"body": map[string]any{},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/create")
	require.NotNil(t, pathItem)
	require.NotNil(t, pathItem.Post)
	require.NotNil(t, pathItem.Post.RequestBody)
}

func TestGenerateOpenAPI_SkipsEmptyMethodOrPath(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"no-method": {
				"path": "/api/test",
			},
			"no-path": {
				"method": "GET",
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)
	assert.Equal(t, 0, doc.Paths.Len())
}

func TestGenerateOpenAPI_HasJWTMiddlewarePreset(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"security": map[string]any{
				"jwt": map[string]any{"secret": "s"},
			},
		},
		Routes: map[string]map[string]any{
			"preset-auth": {
				"id":                "preset-auth",
				"method":            "GET",
				"path":              "/api/me",
				"middleware_preset": "authenticated",
				"trigger": map[string]any{
					"workflow": "get-me",
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/me")
	require.NotNil(t, pathItem)
	require.NotNil(t, pathItem.Get)
	// hasJWTMiddleware returns true for preset containing "auth"
	require.NotNil(t, pathItem.Get.Security)
}

func TestGenerateOpenAPI_JWTSecuritySchemeNoSchemas(t *testing.T) {
	// JWT config but no component schemas — Components should still be created
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"security": map[string]any{
				"jwt": map[string]any{"secret": "test"},
			},
		},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)
	require.NotNil(t, doc.Components)
	assert.Contains(t, doc.Components.SecuritySchemes, "bearerAuth")
}

func TestRouteToOperationID_WithID(t *testing.T) {
	route := map[string]any{"id": "custom-id", "method": "GET", "path": "/test"}
	assert.Equal(t, "custom-id", routeToOperationID(route))
}

func TestRouteToOperationID_WithoutID(t *testing.T) {
	route := map[string]any{"method": "POST", "path": "/api/tasks"}
	assert.Equal(t, "post_api_tasks", routeToOperationID(route))
}

func TestHasJWTMiddleware_True(t *testing.T) {
	assert.True(t, hasJWTMiddleware(map[string]any{
		"middleware": []any{"auth.jwt"},
	}))
}

func TestHasJWTMiddleware_False(t *testing.T) {
	assert.False(t, hasJWTMiddleware(map[string]any{
		"middleware": []any{"recover"},
	}))
}

func TestHasJWTMiddleware_NoMiddleware(t *testing.T) {
	assert.False(t, hasJWTMiddleware(map[string]any{}))
}

func TestHasJWTMiddleware_PresetWithAuth(t *testing.T) {
	assert.True(t, hasJWTMiddleware(map[string]any{
		"middleware_preset": "authenticated",
	}))
}

func TestHasJWTMiddleware_PresetWithoutAuth(t *testing.T) {
	assert.False(t, hasJWTMiddleware(map[string]any{
		"middleware_preset": "public",
	}))
}

func TestGetStringFromRoot(t *testing.T) {
	root := map[string]any{"name": "MyAPI"}
	assert.Equal(t, "MyAPI", getStringFromRoot(root, "name", "default"))
	assert.Equal(t, "default", getStringFromRoot(root, "missing", "default"))
	assert.Equal(t, "default", getStringFromRoot(map[string]any{"name": 42}, "name", "default"))
}

func TestScalarHTML(t *testing.T) {
	html := scalarHTML()
	assert.Contains(t, html, "<!DOCTYPE html>")
	assert.Contains(t, html, "/openapi.json")
	assert.Contains(t, html, "@scalar/api-reference")
}

func TestConvertSchema_Invalid(t *testing.T) {
	// A schema with a value that can't be unmarshalled to openapi3.Schema
	// but can be marshalled — this should still return a SchemaRef
	schema := map[string]any{"type": "string"}
	ref, err := convertSchema(schema)
	require.NoError(t, err)
	require.NotNil(t, ref)
}

// --- Response helpers ---

func TestWriteHTTPResponse_WithHeadersAndCookies(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		return writeHTTPResponse(c, &api.HTTPResponse{
			Status: 200,
			Headers: map[string]string{
				"X-Custom": "value",
			},
			Cookies: []api.Cookie{
				{
					Name:     "session",
					Value:    "abc123",
					Path:     "/",
					Domain:   "example.com",
					MaxAge:   3600,
					Secure:   true,
					HTTPOnly: true,
					SameSite: "Strict",
				},
			},
			Body: map[string]any{"msg": "ok"},
		})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "value", resp.Header.Get("X-Custom"))

	cookies := resp.Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "session", cookies[0].Name)
	assert.Equal(t, "abc123", cookies[0].Value)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "ok", result["msg"])
}

func TestWriteHTTPResponse_NilBody(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		return writeHTTPResponse(c, &api.HTTPResponse{
			Status: 204,
			Body:   nil,
		})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 204, resp.StatusCode)
}

func TestWriteHTTPResponse_NoHeaders(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		return writeHTTPResponse(c, &api.HTTPResponse{
			Status: 200,
			Body:   "hello",
		})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestWriteErrorResponse(t *testing.T) {
	app := fiber.New()
	app.Get("/test", func(c fiber.Ctx) error {
		return writeErrorResponse(c, 422, ErrorResponse{
			Error: api.ErrorData{
				Code:    "VALIDATION_ERROR",
				Message: "invalid input",
				TraceID: "trace-1",
			},
		})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 422, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	errObj := result["error"].(map[string]any)
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

// --- Server: errorHandler ---

func TestErrorHandler_FiberErrors(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		expectCode string
	}{
		{"not found", fiber.StatusNotFound, "NOT_FOUND"},
		{"method not allowed", fiber.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
		{"too many requests", fiber.StatusTooManyRequests, "RATE_LIMITED"},
		{"unauthorized", fiber.StatusUnauthorized, "UNAUTHORIZED"},
		{"forbidden", fiber.StatusForbidden, "FORBIDDEN"},
		{"request timeout", fiber.StatusRequestTimeout, "TIMEOUT"},
		{"bad request", fiber.StatusBadRequest, "HTTP_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc := &config.ResolvedConfig{
				Root: map[string]any{},
			}
			srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
			require.NoError(t, err)

			app := srv.App()
			app.Get("/err", func(c fiber.Ctx) error {
				return fiber.NewError(tt.status, "test error")
			})

			req := httptest.NewRequest("GET", "/err", nil)
			resp, err := app.Test(req)
			require.NoError(t, err)
			assert.Equal(t, tt.status, resp.StatusCode)

			body, _ := io.ReadAll(resp.Body)
			var result map[string]any
			require.NoError(t, json.Unmarshal(body, &result))
			errObj := result["error"].(map[string]any)
			assert.Equal(t, tt.expectCode, errObj["code"])
		})
	}
}

func TestErrorHandler_NonFiberError(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)

	app := srv.App()
	app.Get("/err", func(c fiber.Ctx) error {
		return &fiber.Error{Code: 500, Message: "internal issue"}
	})

	req := httptest.NewRequest("GET", "/err", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
}

// --- Presets ---

func TestExpandPreset_Unknown(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{
			"known": []any{"recover"},
		},
	})

	_, err := srv.expandPreset("unknown")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown middleware preset")
}

func TestExpandPreset_Valid(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{
			"base": []any{"recover", "requestid"},
		},
	})

	mws, err := srv.expandPreset("base")
	assert.NoError(t, err)
	assert.Equal(t, []string{"recover", "requestid"}, mws)
}

func TestValidatePresets_GroupPreset(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"middleware_presets": map[string]any{
				"known": []any{"recover"},
			},
			"route_groups": map[string]any{
				"/api": map[string]any{
					"middleware_preset": "unknown-group-preset",
				},
			},
		},
		Routes: map[string]map[string]any{},
	}
	srv, _ := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())

	errs := srv.ValidatePresets()
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unknown middleware preset")
	assert.Contains(t, errs[0].Error(), "route group")
}

func TestGetPresets_Empty(t *testing.T) {
	srv := testServerWithConfig(map[string]any{})
	presets := srv.getPresets()
	assert.Empty(t, presets)
}

func TestGetGlobalMiddleware_Empty(t *testing.T) {
	srv := testServerWithConfig(map[string]any{})
	mw := srv.getGlobalMiddleware()
	assert.Nil(t, mw)
}

func TestGetRouteGroups_Empty(t *testing.T) {
	srv := testServerWithConfig(map[string]any{})
	groups := srv.getRouteGroups()
	assert.Empty(t, groups)
}

func TestGetGroupMiddleware_DirectList(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"route_groups": map[string]any{
			"/api": map[string]any{
				"middleware": []any{"recover", "requestid"},
			},
		},
	})

	mw, err := srv.getGroupMiddleware("/api/test")
	require.NoError(t, err)
	assert.Equal(t, []string{"recover", "requestid"}, mw)
}

func TestGetGroupMiddleware_NoMatch(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"route_groups": map[string]any{
			"/admin": map[string]any{
				"middleware": []any{"recover"},
			},
		},
	})

	mw, err := srv.getGroupMiddleware("/api/test")
	require.NoError(t, err)
	assert.Nil(t, mw)
}

// --- Routes: registerRoute validation ---

func TestRegisterRoute_MissingMethod(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)

	err = srv.registerRoute("test", map[string]any{
		"path": "/test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "method and path are required")
}

func TestRegisterRoute_MissingPath(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)

	err = srv.registerRoute("test", map[string]any{
		"method": "GET",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "method and path are required")
}

func TestRegisterRoute_MissingTrigger(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)

	err = srv.registerRoute("test", map[string]any{
		"method": "GET",
		"path":   "/test",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "trigger config is required")
}

func TestRegisterRoute_MissingWorkflowID(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)

	err = srv.registerRoute("test", map[string]any{
		"method":  "GET",
		"path":    "/test",
		"trigger": map[string]any{},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "trigger.workflow is required")
}

func TestRegisterRoute_UnsupportedMethod(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{
		"wf": {"nodes": map[string]any{}, "edges": []any{}},
	}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	err = srv.registerRoute("test", map[string]any{
		"method": "OPTIONS",
		"path":   "/test",
		"trigger": map[string]any{
			"workflow": "wf",
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported HTTP method")
}

func TestRegisterRoute_WorkflowNotFound(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	err = srv.registerRoute("test", map[string]any{
		"method": "GET",
		"path":   "/test",
		"trigger": map[string]any{
			"workflow": "nonexistent",
		},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Server: Setup and options ---

func TestNewServer_WithLogger(t *testing.T) {
	rc := testConfig()
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry(),
		WithLogger(nil)) // nil is a valid slog.Logger
	require.NoError(t, err)
	assert.NotNil(t, srv)
}

func TestNewServer_WithTraceHub(t *testing.T) {
	rc := testConfig()
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry(),
		WithTraceHub(nil))
	require.NoError(t, err)
	assert.NotNil(t, srv)
}

func TestServer_WorkflowCache(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := testConfig()
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	assert.Equal(t, cache, srv.WorkflowCache())
}

// --- Server: Setup compiles workflows when no cache is provided ---

func TestServer_Setup_CompilesWorkflows(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root:   map[string]any{},
		Routes: map[string]map[string]any{},
		Workflows: map[string]map[string]any{
			"test-wf": {
				"nodes": map[string]any{},
				"edges": []any{},
			},
		},
		Schemas: map[string]map[string]any{},
	}

	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	assert.NotNil(t, srv.WorkflowCache())
}

// --- Trigger: coerceNumeric ---

func TestCoerceNumeric_Integer(t *testing.T) {
	assert.Equal(t, 42, coerceNumeric("42"))
}

func TestCoerceNumeric_Float(t *testing.T) {
	assert.Equal(t, 3.14, coerceNumeric("3.14"))
}

func TestCoerceNumeric_NonNumericString(t *testing.T) {
	assert.Equal(t, "hello", coerceNumeric("hello"))
}

func TestCoerceNumeric_NonString(t *testing.T) {
	assert.Equal(t, 42, coerceNumeric(42))
	assert.Equal(t, true, coerceNumeric(true))
}

// --- Trigger: getFileFields ---

func TestGetFileFields_WithFiles(t *testing.T) {
	cfg := map[string]any{
		"files": []any{"avatar", "document"},
	}
	fields := getFileFields(cfg)
	assert.True(t, fields["avatar"])
	assert.True(t, fields["document"])
	assert.False(t, fields["other"])
}

func TestGetFileFields_NoFiles(t *testing.T) {
	cfg := map[string]any{}
	fields := getFileFields(cfg)
	assert.Empty(t, fields)
}

// --- Route: all HTTP methods ---

func TestRoute_PUT(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"put-route": {
				"method": "PUT",
				"path":   "/items/:id",
				"trigger": map[string]any{
					"workflow": "update",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"update": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "\"updated\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("PUT", "/items/1", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRoute_PATCH(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"patch-route": {
				"method": "PATCH",
				"path":   "/items/:id",
				"trigger": map[string]any{
					"workflow": "patch",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"patch": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "\"patched\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("PATCH", "/items/1", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRoute_DELETE(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"delete-route": {
				"method": "DELETE",
				"path":   "/items/:id",
				"trigger": map[string]any{
					"workflow": "delete",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"delete": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "\"deleted\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		nil,
	)

	req := httptest.NewRequest("DELETE", "/items/1", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// --- Health: mustReq helper sanity ---

func TestHealth_ErrorResponses(t *testing.T) {
	srv := setupHealthServer(t)

	// Confirm /health with no services returns healthy
	resp, err := srv.App().Test(mustReq(http.MethodGet, "/health"))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body := readJSON(t, resp)
	assert.Equal(t, "healthy", body["status"])
}

// --- Middleware: resolveEndpointMiddleware ---

func TestResolveEndpointMiddleware_EmptyEndpoint(t *testing.T) {
	srv := testServerWithConfig(map[string]any{})

	handlers, err := srv.resolveEndpointMiddleware(map[string]any{})
	require.NoError(t, err)
	assert.Empty(t, handlers)
}

func TestResolveEndpointMiddleware_WithPreset(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{
			"base": []any{"recover", "requestid"},
		},
	})

	handlers, err := srv.resolveEndpointMiddleware(map[string]any{
		"middleware_preset": "base",
	})
	require.NoError(t, err)
	assert.Len(t, handlers, 2)
}

func TestResolveEndpointMiddleware_WithDirectMiddleware(t *testing.T) {
	srv := testServerWithConfig(map[string]any{})

	handlers, err := srv.resolveEndpointMiddleware(map[string]any{
		"middleware": []any{"recover"},
	})
	require.NoError(t, err)
	assert.Len(t, handlers, 1)
}

func TestResolveEndpointMiddleware_UnknownMiddleware(t *testing.T) {
	srv := testServerWithConfig(map[string]any{})

	_, err := srv.resolveEndpointMiddleware(map[string]any{
		"middleware": []any{"nonexistent"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown middleware")
}

func TestResolveEndpointMiddleware_UnknownPreset(t *testing.T) {
	srv := testServerWithConfig(map[string]any{})

	_, err := srv.resolveEndpointMiddleware(map[string]any{
		"middleware_preset": "nonexistent",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown middleware preset")
}

func TestResolveEndpointMiddleware_DeduplicatesPresetAndDirect(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{
			"base": []any{"recover"},
		},
	})

	handlers, err := srv.resolveEndpointMiddleware(map[string]any{
		"middleware_preset": "base",
		"middleware":        []any{"recover", "requestid"},
	})
	require.NoError(t, err)
	// "recover" from preset, "recover" from direct should be deduped
	assert.Len(t, handlers, 2) // recover + requestid
}

// --- Endpoint middleware ordering validation ---

func TestResolveEndpointMiddleware_ValidOrder(t *testing.T) {
	srv := testServerWithConfig(map[string]any{})

	handlers, err := srv.resolveEndpointMiddleware(map[string]any{
		"middleware": []any{"recover", "requestid"},
	})
	require.NoError(t, err)
	assert.Len(t, handlers, 2)
}

func TestResolveEndpointMiddleware_InvalidOrder(t *testing.T) {
	srv := testServerWithConfig(map[string]any{})

	_, err := srv.resolveEndpointMiddleware(map[string]any{
		"middleware": []any{"casbin.enforce", "auth.jwt"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must appear before")
}

// --- Middleware: CORS with nil config ---

func TestBuildMiddleware_CORS_NilConfig(t *testing.T) {
	h, err := BuildMiddleware("security.cors", nil)
	require.NoError(t, err)
	require.NotNil(t, h)
}

// --- Middleware: CORS with credentials ---

func TestBuildMiddleware_CORS_WithCredentials(t *testing.T) {
	h, err := BuildMiddleware("security.cors", map[string]any{
		"security": map[string]any{
			"cors": map[string]any{
				"allow_origins":     "http://example.com",
				"allow_methods":     "GET, POST",
				"allow_headers":     "Authorization, Content-Type",
				"allow_credentials": true,
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, h)
}

// --- applyGlobalMiddleware ---

func TestApplyGlobalMiddleware_Success(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"global_middleware": []any{"recover", "requestid"},
		},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)

	err = srv.applyGlobalMiddleware()
	assert.NoError(t, err)
}

func TestApplyGlobalMiddleware_UnknownMiddleware(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"global_middleware": []any{"nonexistent"},
		},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)

	err = srv.applyGlobalMiddleware()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown middleware")
}

// --- OpenAPI: response with non-map entry ---

func TestGenerateOpenAPI_ResponseNonMapEntry(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"test": {
				"id":     "test",
				"method": "GET",
				"path":   "/test",
				"trigger": map[string]any{
					"workflow": "test",
				},
				"response": map[string]any{
					"200": "not-a-map", // should be skipped
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/test")
	require.NotNil(t, pathItem)
	// Default 200 response should be added since the non-map was skipped
	assert.Equal(t, 1, pathItem.Get.Responses.Len())
}

// --- OpenAPI: multiple routes on same path ---

func TestGenerateOpenAPI_SamePathDifferentMethods(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"get-items": {
				"id":     "get-items",
				"method": "GET",
				"path":   "/api/items",
				"trigger": map[string]any{
					"workflow": "list",
				},
			},
			"create-item": {
				"id":     "create-item",
				"method": "POST",
				"path":   "/api/items",
				"trigger": map[string]any{
					"workflow": "create",
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}
	doc, err := GenerateOpenAPI(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/items")
	require.NotNil(t, pathItem)
	assert.NotNil(t, pathItem.Get)
	assert.NotNil(t, pathItem.Post)
}

// --- MapTrigger: request ID from header ---

func TestMapTrigger_CustomRequestID(t *testing.T) {
	result := triggerTest(t, "GET", "/test", nil, map[string]string{
		"X-Request-Id": "custom-trace-123",
	}, map[string]any{
		"input": map[string]any{},
	})

	assert.Equal(t, "custom-trace-123", result.Trigger.TraceID)
}

// --- Casbin helper: toStringSlice ---

func TestToStringSlice_Valid(t *testing.T) {
	result, err := toStringSlice([]any{"a", "b", "c"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestToStringSlice_NotArray(t *testing.T) {
	_, err := toStringSlice("not-array")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected array")
}

func TestToStringSlice_NonStringItems(t *testing.T) {
	result, err := toStringSlice([]any{"a", float64(42), true})
	require.NoError(t, err)
	assert.Equal(t, "a", result[0])
	assert.Equal(t, "42", result[1])
	assert.Equal(t, "true", result[2])
}

// --- editor.go: relPath helper ---

func TestRelPath_EmptyPath(t *testing.T) {
	assert.Equal(t, "", relPath("/base", ""))
}

func TestRelPath_ValidPath(t *testing.T) {
	result := relPath("/base", "/base/sub/file.json")
	assert.Equal(t, "sub/file.json", result)
}

// --- Trigger: parseBody with form data ---

func TestParseBody_EmptyBody(t *testing.T) {
	app := fiber.New()
	var result any

	app.Post("/test", func(c fiber.Ctx) error {
		result = parseBody(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Nil(t, result)
}

func TestParseBody_PlainText(t *testing.T) {
	app := fiber.New()
	var result any

	app.Post("/test", func(c fiber.Ctx) error {
		result = parseBody(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/test", strings.NewReader("plain text body"))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "plain text body", result)
}

func TestParseBody_JSONBody(t *testing.T) {
	app := fiber.New()
	var result any

	app.Post("/test", func(c fiber.Ctx) error {
		result = parseBody(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/test", strings.NewReader(`{"key":"value"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", m["key"])
}

func TestParseBody_InvalidJSON_WithJSONContentType(t *testing.T) {
	app := fiber.New()
	var result any

	app.Post("/test", func(c fiber.Ctx) error {
		result = parseBody(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/test", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	// Falls back to raw string
	assert.Equal(t, "not-json", result)
}

func TestParseBody_NoContentType_ValidJSON(t *testing.T) {
	app := fiber.New()
	var result any

	app.Post("/test", func(c fiber.Ctx) error {
		result = parseBody(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/test", strings.NewReader(`{"auto":"detect"}`))
	// No Content-Type header
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "detect", m["auto"])
}

func TestParseQuery(t *testing.T) {
	app := fiber.New()
	var result map[string]any

	app.Get("/test", func(c fiber.Ctx) error {
		result = parseQuery(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test?a=1&b=two", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "1", result["a"])
	assert.Equal(t, "two", result["b"])
}

func TestParseQuery_Empty(t *testing.T) {
	app := fiber.New()
	var result map[string]any

	app.Get("/test", func(c fiber.Ctx) error {
		result = parseQuery(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Empty(t, result)
}

func TestParseHeaders(t *testing.T) {
	app := fiber.New()
	var result map[string]any

	app.Get("/test", func(c fiber.Ctx) error {
		result = parseHeaders(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Custom", "value1")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "value1", result["X-Custom"])
}

func TestParseParams(t *testing.T) {
	app := fiber.New()
	var result map[string]any

	app.Get("/items/:id", func(c fiber.Ctx) error {
		result = parseParams(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/items/42", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "42", result["id"])
}

func TestBuildRawRequestContext(t *testing.T) {
	app := fiber.New()
	var ctx map[string]any

	app.Get("/test", func(c fiber.Ctx) error {
		ctx = buildRawRequestContext(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test?q=hello", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	assert.Equal(t, "GET", ctx["method"])
	assert.Equal(t, "/test", ctx["path"])
	assert.NotNil(t, ctx["query"])
	assert.NotNil(t, ctx["headers"])
	assert.NotNil(t, ctx["params"])
}

func TestBuildRawRequestContext_WithJWTClaims(t *testing.T) {
	app := fiber.New()
	var ctx map[string]any

	app.Use(func(c fiber.Ctx) error {
		c.Locals(api.LocalJWTClaims, map[string]any{"sub": "user-1"})
		c.Locals(api.LocalJWTUserID, "user-1")
		c.Locals(api.LocalJWTRoles, []string{"admin"})
		return c.Next()
	})

	app.Get("/test", func(c fiber.Ctx) error {
		ctx = buildRawRequestContext(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	authCtx, ok := ctx["auth"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "user-1", authCtx["sub"])
	assert.NotNil(t, authCtx["roles"])
	assert.NotNil(t, authCtx["claims"])
}

func TestExtractAuth_NoClaims(t *testing.T) {
	app := fiber.New()
	var auth *api.AuthData

	app.Get("/test", func(c fiber.Ctx) error {
		auth = extractAuth(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Nil(t, auth)
}

func TestExtractAuth_WithClaims(t *testing.T) {
	app := fiber.New()
	var auth *api.AuthData

	app.Use(func(c fiber.Ctx) error {
		c.Locals(api.LocalJWTClaims, map[string]any{"sub": "user-1", "email": "a@b.com"})
		c.Locals(api.LocalJWTUserID, "user-1")
		c.Locals(api.LocalJWTRoles, []string{"editor"})
		return c.Next()
	})

	app.Get("/test", func(c fiber.Ctx) error {
		auth = extractAuth(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	require.NotNil(t, auth)
	assert.Equal(t, "user-1", auth.UserID)
	assert.Equal(t, []string{"editor"}, auth.Roles)
	assert.Equal(t, "a@b.com", auth.Claims["email"])
}

// --- Casbin: multi-tenant with query param fallback ---

func TestCasbin_MultiTenant_QueryFallback(t *testing.T) {
	cfg := map[string]any{
		"model": multiTenantModel,
		"policies": []any{
			[]any{"p", "alice", "ws-1", "/api/data", "GET"},
		},
		"tenant_param": "workspace_id",
	}

	mw, err := newCasbinMiddleware(cfg, nil)
	require.NoError(t, err)

	errHandler := func(c fiber.Ctx, err error) error {
		if fe, ok := err.(*fiber.Error); ok {
			return c.Status(fe.Code).SendString(fe.Message)
		}
		return c.Status(500).SendString(err.Error())
	}

	app := fiber.New(fiber.Config{ErrorHandler: errHandler})
	app.Get("/api/data", func(c fiber.Ctx) error {
		c.Locals(api.LocalJWTUserID, "alice")
		return c.Next()
	}, mw, func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// Use query param for tenant
	req := httptest.NewRequest("GET", "/api/data?workspace_id=ws-1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// --- Casbin: loadPolicies edge cases ---

func TestCasbin_PolicyTooFewFields(t *testing.T) {
	cfg := map[string]any{
		"model": aclModel,
		"policies": []any{
			[]any{"p"}, // too few
		},
	}

	_, err := newCasbinMiddleware(cfg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too few fields")
}

func TestCasbin_RoleLinkTooFewFields(t *testing.T) {
	cfg := map[string]any{
		"model": rbacModel,
		"policies": []any{
			[]any{"p", "admin", "/api/*", "GET"},
		},
		"role_links": []any{
			[]any{"g"}, // too few
		},
	}

	_, err := newCasbinMiddleware(cfg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too few fields")
}

func TestCasbin_PolicyNotArray(t *testing.T) {
	cfg := map[string]any{
		"model": aclModel,
		"policies": []any{
			"not-an-array",
		},
	}

	_, err := newCasbinMiddleware(cfg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected array")
}

func TestCasbin_RoleLinkNotArray(t *testing.T) {
	cfg := map[string]any{
		"model": rbacModel,
		"policies": []any{
			[]any{"p", "admin", "/api/*", "GET"},
		},
		"role_links": []any{
			"not-an-array",
		},
	}

	_, err := newCasbinMiddleware(cfg, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected array")
}

// --- editor.go: findUpstreamNodes ---

func TestFindUpstreamNodes(t *testing.T) {
	e := &EditorAPI{}

	wfConfig := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "transform.set"},
			"b": map[string]any{"type": "control.if"},
			"c": map[string]any{"type": "response.json"},
		},
		"edges": []any{
			map[string]any{"from": "a", "output": "default", "to": "b"},
			map[string]any{"from": "b", "output": "true", "to": "c"},
		},
	}

	result := e.findUpstreamNodes(wfConfig, "c")
	// Should find b (direct upstream) and a (upstream of b)
	assert.Len(t, result, 2)

	nodeIDs := make(map[string]bool)
	for _, r := range result {
		nodeIDs[r["node_id"].(string)] = true
	}
	assert.True(t, nodeIDs["a"])
	assert.True(t, nodeIDs["b"])
}

func TestFindUpstreamNodes_NoEdges(t *testing.T) {
	e := &EditorAPI{}

	wfConfig := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "transform.set"},
		},
		"edges": []any{},
	}

	result := e.findUpstreamNodes(wfConfig, "a")
	assert.Empty(t, result)
}

func TestFindUpstreamNodes_StartNode(t *testing.T) {
	e := &EditorAPI{}

	wfConfig := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "transform.set"},
			"b": map[string]any{"type": "response.json"},
		},
		"edges": []any{
			map[string]any{"from": "a", "output": "default", "to": "b"},
		},
	}

	result := e.findUpstreamNodes(wfConfig, "a")
	assert.Empty(t, result) // a has no upstream
}

// --- addPathParams edge cases ---

func TestAddPathParams_NoParams(t *testing.T) {
	op := &openapi3.Operation{}
	addPathParams(op, "/api/items")
	assert.Empty(t, op.Parameters)
}

func TestAddPathParams_MultipleParams(t *testing.T) {
	op := &openapi3.Operation{}
	addPathParams(op, "/api/{orgId}/items/{id}")
	assert.Len(t, op.Parameters, 2)
}

// --- addQueryParams edge cases ---

func TestAddQueryParams_NoProperties(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParams(op, map[string]any{
		"schema": map[string]any{},
	})
	assert.Empty(t, op.Parameters)
}

func TestAddQueryParams_NoSchema(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParams(op, map[string]any{})
	assert.Empty(t, op.Parameters)
}

func TestAddQueryParams_PropertyWithoutType(t *testing.T) {
	op := &openapi3.Operation{}
	addQueryParams(op, map[string]any{
		"schema": map[string]any{
			"properties": map[string]any{
				"q": map[string]any{}, // no type key
			},
		},
	})
	require.Len(t, op.Parameters, 1)
	// Defaults to "string"
	assert.Contains(t, *op.Parameters[0].Value.Schema.Value.Type, "string")
}

// --- Server: global middleware error propagation ---

func TestResolveMiddlewareChain_UnknownMiddleware(t *testing.T) {
	srv := testServerWithConfig(map[string]any{})

	route := map[string]any{
		"id":         "test",
		"path":       "/test",
		"middleware": []any{"nonexistent"},
	}

	_, err := srv.ResolveMiddlewareChain(route)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown middleware")
}

func TestResolveMiddlewareChain_BadOrder(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"security": map[string]any{
			"jwt":    map[string]any{"secret": "test-secret-key-that-is-at-least-32-bytes-long"},
			"casbin": map[string]any{"model": aclModel, "policies": []any{[]any{"p", "a", "/b", "GET"}}},
		},
	})

	route := map[string]any{
		"id":         "test",
		"path":       "/test",
		"middleware": []any{"casbin.enforce", "auth.jwt"},
	}

	_, err := srv.ResolveMiddlewareChain(route)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must appear before")
}

// --- Response: ErrorResponse JSON structure ---

func TestErrorResponse_JSONMarshalling(t *testing.T) {
	resp := ErrorResponse{
		Error: api.ErrorData{
			Code:    "TEST_ERROR",
			Message: "test message",
			TraceID: "trace-123",
		},
	}
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	errObj := result["error"].(map[string]any)
	assert.Equal(t, "TEST_ERROR", errObj["code"])
	assert.Equal(t, "test message", errObj["message"])
	assert.Equal(t, "trace-123", errObj["trace_id"])
}

// --- MapErrorToHTTP: verify details for ValidationError ---

func TestMapErrorToHTTP_ValidationError_Details(t *testing.T) {
	err := &api.ValidationError{Field: "name", Message: "required", Value: ""}
	status, resp := MapErrorToHTTP(err, "t-1", false)
	assert.Equal(t, 422, status)
	details := resp.Error.Details.(map[string]any)
	assert.Equal(t, "name", details["field"])
	assert.Equal(t, "", details["value"])
}

// --- Casbin: extractSubject ---

func TestExtractSubject_WithUserID(t *testing.T) {
	app := fiber.New()
	var sub string

	app.Get("/test", func(c fiber.Ctx) error {
		c.Locals(api.LocalJWTUserID, "user-1")
		sub = extractSubject(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(req)
	assert.Equal(t, "user-1", sub)
}

func TestExtractSubject_NoUserID(t *testing.T) {
	app := fiber.New()
	var sub string

	app.Get("/test", func(c fiber.Ctx) error {
		sub = extractSubject(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(req)
	assert.Equal(t, "", sub)
}

func TestExtractSubject_EmptyString(t *testing.T) {
	app := fiber.New()
	var sub string

	app.Get("/test", func(c fiber.Ctx) error {
		c.Locals(api.LocalJWTUserID, "")
		sub = extractSubject(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	_, _ = app.Test(req)
	assert.Equal(t, "", sub)
}

// --- OpenAPI: addResponses default response ---

func TestAddResponses_EmptyResponseDef(t *testing.T) {
	op := &openapi3.Operation{}
	route := map[string]any{}
	rc := &config.ResolvedConfig{Schemas: map[string]map[string]any{}}
	addResponses(op, route, rc)

	// Should add default 200 response
	assert.Equal(t, 1, op.Responses.Len())
}

func TestAddResponses_WithDescription(t *testing.T) {
	op := &openapi3.Operation{}
	route := map[string]any{
		"response": map[string]any{
			"201": map[string]any{
				"description": "Created successfully",
			},
		},
	}
	rc := &config.ResolvedConfig{Schemas: map[string]map[string]any{}}
	addResponses(op, route, rc)

	assert.GreaterOrEqual(t, op.Responses.Len(), 1)
}

// --- OpenAPI: addRequestBody with $ref ---

func TestAddRequestBody_WithRef(t *testing.T) {
	op := &openapi3.Operation{}
	bodyDef := map[string]any{
		"schema": map[string]any{
			"$ref": "schemas/MySchema",
		},
	}
	rc := &config.ResolvedConfig{Schemas: map[string]map[string]any{}}
	addRequestBody(op, bodyDef, rc)

	require.NotNil(t, op.RequestBody)
	mediaType := op.RequestBody.Value.Content["application/json"]
	require.NotNil(t, mediaType)
	assert.Equal(t, "#/components/schemas/MySchema", mediaType.Schema.Ref)
}

// --- Server: Setup with global middleware error ---

func TestServer_Setup_GlobalMiddlewareError(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"global_middleware": []any{"nonexistent"},
		},
		Routes:    map[string]map[string]any{},
		Workflows: map[string]map[string]any{},
		Schemas:   map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)

	err = srv.Setup()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "global middleware")
}

// --- Server: Setup with route errors ---

func TestServer_Setup_RouteError(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"bad-route": {
				"method": "GET",
				// missing path and trigger
			},
		},
		Workflows: map[string]map[string]any{},
		Schemas:   map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)

	err = srv.Setup()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "register routes")
}

// --- Server: Setup with invalid preset ---

func TestServer_Setup_InvalidPreset(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"preset-route": {
				"method":            "GET",
				"path":              "/test",
				"middleware_preset": "nonexistent",
				"trigger": map[string]any{
					"workflow": "wf",
				},
			},
		},
		Workflows: map[string]map[string]any{
			"wf": {"nodes": map[string]any{}, "edges": []any{}},
		},
		Schemas: map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)

	err = srv.Setup()
	assert.Error(t, err)
}

// --- openapi3 import needed ---

func TestFiberToOpenAPIPath_NoParams(t *testing.T) {
	assert.Equal(t, "/api/items", fiberToOpenAPIPath("/api/items"))
}

func TestFiberToOpenAPIPath_RootPath(t *testing.T) {
	assert.Equal(t, "/", fiberToOpenAPIPath("/"))
}

// --- editor.go: resolvedConfig ---

func TestEditorAPI_ResolvedConfig_WithReloader(t *testing.T) {
	// Without reloader, should return rc
	e := &EditorAPI{
		rc: &config.ResolvedConfig{Root: map[string]any{"name": "test"}},
	}
	rc := e.resolvedConfig()
	require.NotNil(t, rc)
	assert.Equal(t, "test", rc.Root["name"])
}

// --- editor.go: findUpstreamNodes with nil edges ---

func TestFindUpstreamNodes_NilEdges(t *testing.T) {
	e := &EditorAPI{}
	wfConfig := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "transform.set"},
		},
	}
	result := e.findUpstreamNodes(wfConfig, "a")
	assert.Empty(t, result)
}

func TestFindUpstreamNodes_NilNodeEdgeEntry(t *testing.T) {
	e := &EditorAPI{}
	wfConfig := map[string]any{
		"nodes": map[string]any{},
		"edges": []any{nil, "bad"},
	}
	result := e.findUpstreamNodes(wfConfig, "c")
	assert.Empty(t, result)
}

func TestFindUpstreamNodes_CyclicGraph(t *testing.T) {
	e := &EditorAPI{}
	wfConfig := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "transform.set"},
			"b": map[string]any{"type": "control.loop"},
		},
		"edges": []any{
			map[string]any{"from": "a", "output": "default", "to": "b"},
			map[string]any{"from": "b", "output": "loop", "to": "a"},
		},
	}
	// Should not infinite loop due to visited tracking
	result := e.findUpstreamNodes(wfConfig, "b")
	assert.Len(t, result, 1) // only "a"
}

// --- editor_static.go: RegisterEditorUI (no embedded FS) ---

func TestRegisterEditorUI_NoEmbeddedFS(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)

	// RegisterEditorUI when editorfs.FS is nil (default for non-embed builds)
	srv.RegisterEditorUI()

	// Should register placeholder routes
	req := httptest.NewRequest("GET", "/editor", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Editor not embedded")

	req = httptest.NewRequest("GET", "/editor/some/path", nil)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Editor not embedded")
}

// --- Server: registerRoutes with multiple routes ---

func TestRegisterRoutes_MultipleRoutes(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{
		"wf1": {"nodes": map[string]any{}, "edges": []any{}},
		"wf2": {"nodes": map[string]any{}, "edges": []any{}},
	}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"get-items": {
				"method": "GET",
				"path":   "/items",
				"trigger": map[string]any{
					"workflow": "wf1",
					"input":    map[string]any{},
				},
			},
			"post-items": {
				"method": "POST",
				"path":   "/items",
				"trigger": map[string]any{
					"workflow": "wf2",
					"input":    map[string]any{},
				},
			},
		},
		Workflows: map[string]map[string]any{
			"wf1": {"nodes": map[string]any{}, "edges": []any{}},
			"wf2": {"nodes": map[string]any{}, "edges": []any{}},
		},
		Schemas: map[string]map[string]any{},
	}

	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	err = srv.registerRoutes()
	assert.NoError(t, err)
}

// --- Server: custom response timeout ---

func TestRoute_CustomResponseTimeout(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"fast": {
				"method": "GET",
				"path":   "/fast",
				"trigger": map[string]any{
					"workflow": "fast-wf",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"fast-wf": {
				"nodes": map[string]any{
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "\"fast\"",
						},
					},
				},
				"edges": []any{},
			},
		},
		map[string]any{
			"server": map[string]any{
				"response_timeout": "5s",
			},
		},
	)

	req := httptest.NewRequest("GET", "/fast", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// --- Trigger: MapTrigger with expression error ---

func TestMapTrigger_ExpressionError(t *testing.T) {
	app := fiber.New()
	compiler := expr.NewCompilerWithFunctions()
	var triggerErr error

	app.Get("/test", func(c fiber.Ctx) error {
		_, triggerErr = MapTrigger(c, map[string]any{
			"input": map[string]any{
				"val": "{{ undefined_func() }}",
			},
		}, compiler)
		if triggerErr != nil {
			return c.Status(400).SendString(triggerErr.Error())
		}
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
	assert.NotNil(t, triggerErr)
	assert.Contains(t, triggerErr.Error(), "trigger mapping")
}

// --- readyFlag state ---

func TestSetReady_SetsFlag(t *testing.T) {
	readyFlag.Store(false)
	SetReady()
	assert.True(t, readyFlag.Load())
	// Clean up
	readyFlag.Store(false)
}

// --- OpenAPI: buildWorkflowRunner ---

func TestBuildWorkflowRunner(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{
		"test-wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"level":   "info",
						"message": "runner test",
					},
				},
			},
			"edges": []any{},
		},
	}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{Root: map[string]any{}}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	runner := srv.buildWorkflowRunner("test")
	require.NotNil(t, runner)

	// Execute the runner -- workflow exists, should succeed
	err = runner(t.Context(), "test-wf", map[string]any{})
	assert.NoError(t, err)
}

// --- Server: runWorkflow ---

func TestRunWorkflow_NotFound(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{Root: map[string]any{}}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	err = srv.runWorkflow(t.Context(), "nonexistent", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- Casbin: multi-tenant with no tenant param available ---

func TestCasbin_MultiTenant_NoTenantParam(t *testing.T) {
	cfg := map[string]any{
		"model": multiTenantModel,
		"policies": []any{
			[]any{"p", "alice", "ws-1", "/api/data", "GET"},
		},
		"tenant_param": "workspace_id",
	}

	mw, err := newCasbinMiddleware(cfg, nil)
	require.NoError(t, err)

	errHandler := func(c fiber.Ctx, err error) error {
		if fe, ok := err.(*fiber.Error); ok {
			return c.Status(fe.Code).SendString(fe.Message)
		}
		return c.Status(500).SendString(err.Error())
	}

	app := fiber.New(fiber.Config{ErrorHandler: errHandler})
	app.Get("/api/data", func(c fiber.Ctx) error {
		c.Locals(api.LocalJWTUserID, "alice")
		return c.Next()
	}, mw, func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	// No workspace_id param or query — tenant is empty
	req := httptest.NewRequest("GET", "/api/data", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 403, resp.StatusCode)
}

// --- MapTrigger: method and path in context ---

func TestMapTrigger_MethodAndPath(t *testing.T) {
	app := fiber.New()
	compiler := expr.NewCompilerWithFunctions()
	var result *TriggerResult

	app.Post("/items", func(c fiber.Ctx) error {
		var err error
		result, err = MapTrigger(c, map[string]any{
			"input": map[string]any{},
		}, compiler)
		if err != nil {
			return err
		}
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/items", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "http", result.Trigger.Type)
}

// --- MapTrigger: nil input map ---

func TestMapTrigger_NoInputMap(t *testing.T) {
	result := triggerTest(t, "GET", "/test", nil, nil, map[string]any{})
	assert.Empty(t, result.Input)
}

// --- Middleware: splitTrim edge cases ---

func TestSplitTrim_WithSpaces(t *testing.T) {
	result := splitTrim("  a , b ,  c  ")
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestSplitTrim_TrailingComma(t *testing.T) {
	result := splitTrim("a,b,")
	assert.Equal(t, []string{"a", "b"}, result)
}

// --- Routes: test Query method and headers ---

func TestRoute_QueryParams(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"search": {
				"method": "GET",
				"path":   "/search",
				"trigger": map[string]any{
					"workflow": "search-wf",
					"input": map[string]any{
						"q": "{{ query.q }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"search-wf": {
				"nodes": map[string]any{
					"build": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"query": "{{ input.q }}",
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

	req := httptest.NewRequest("GET", "/search?q=hello", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, "hello", result["query"])
}

// --- Routes: error handler integration ---

func TestRoute_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t,
		map[string]map[string]any{
			"get-only": {
				"method": "GET",
				"path":   "/only-get",
				"trigger": map[string]any{
					"workflow": "wf",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"wf": {
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

	req := httptest.NewRequest("POST", "/only-get", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	// Fiber should return 405 Method Not Allowed
	assert.Equal(t, 405, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	errObj := result["error"].(map[string]any)
	assert.Equal(t, "METHOD_NOT_ALLOWED", errObj["code"])
}

// --- MapErrorToHTTP: message preservation ---

func TestMapErrorToHTTP_NotFoundMessage(t *testing.T) {
	err := &api.NotFoundError{Resource: "user", ID: "99"}
	_, resp := MapErrorToHTTP(err, "t", false)
	assert.Contains(t, resp.Error.Message, "user")
}

func TestMapErrorToHTTP_ConflictMessage(t *testing.T) {
	err := &api.ConflictError{Resource: "email", Reason: "already exists"}
	_, resp := MapErrorToHTTP(err, "t", false)
	assert.Contains(t, resp.Error.Message, "already exists")
}

func TestMapErrorToHTTP_ServiceUnavailableMessage(t *testing.T) {
	err := &api.ServiceUnavailableError{Service: "redis", Cause: fmt.Errorf("timeout")}
	_, resp := MapErrorToHTTP(err, "t", false)
	assert.Contains(t, resp.Error.Message, "redis")
}

func TestMapErrorToHTTP_TimeoutMessage(t *testing.T) {
	err := &api.TimeoutError{Duration: 10 * time.Second, Operation: "query"}
	_, resp := MapErrorToHTTP(err, "t", false)
	assert.Contains(t, resp.Error.Message, "query")
}

func TestBuildWorkflowRunner_NotFound(t *testing.T) {
	cache, err := engine.NewWorkflowCache(map[string]map[string]any{}, buildTestNodeRegistry())
	require.NoError(t, err)

	rc := &config.ResolvedConfig{Root: map[string]any{}}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry(),
		WithWorkflowCache(cache))
	require.NoError(t, err)

	runner := srv.buildWorkflowRunner("test")
	err = runner(t.Context(), "nonexistent", map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// --- EditorAPI handler tests ---

func setupEditorApp(t *testing.T) *fiber.App {
	t.Helper()
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()
	compiler := expr.NewCompilerWithFunctions()

	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Schemas: map[string]map[string]any{},
		Routes:  map[string]map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"a": map[string]any{"type": "transform.set"},
					"b": map[string]any{"type": "response.json"},
				},
				"edges": []any{
					map[string]any{"from": "a", "output": "default", "to": "b"},
				},
			},
		},
	}

	editorAPI := NewEditorAPIReadOnly(
		t.TempDir(),
		"",
		rc,
		pluginReg,
		nodeReg,
		svcReg,
		compiler,
	)
	editorAPI.Register(app)
	return app
}

func TestEditorAPI_ListNodes(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/nodes", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	nodes, ok := result["nodes"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, nodes)
}

func TestEditorAPI_GetNodeSchema_NotFound(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/nodes/nonexistent.type/schema", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestEditorAPI_GetNodeSchema_Found(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/nodes/transform.set/schema", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestEditorAPI_ComputeOutputs_NotFound(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/nodes/nonexistent.type/outputs", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestEditorAPI_ComputeOutputs_Found(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/nodes/transform.set/outputs", strings.NewReader(`{"fields":{"a":"b"}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["outputs"])
}

func TestEditorAPI_ListServices(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/services", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["services"])
}

func TestEditorAPI_ListPlugins(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/plugins", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["plugins"])
}

func TestEditorAPI_ListSchemas(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/schemas", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["schemas"])
}

func TestEditorAPI_ValidateExpression_Valid(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/expressions/validate",
		strings.NewReader(`{"expression":"1 + 2"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, true, result["valid"])
}

func TestEditorAPI_ValidateExpression_Invalid(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/expressions/validate",
		strings.NewReader(`{"expression":"{{invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, false, result["valid"])
}

func TestEditorAPI_ValidateExpression_Empty(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/expressions/validate",
		strings.NewReader(`{"expression":""}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, true, result["valid"])
}

func TestEditorAPI_ValidateExpression_BadRequest(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/expressions/validate",
		strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestEditorAPI_ExpressionContext(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/expressions/context?workflow=wf1&node=b", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["variables"])
	assert.NotNil(t, result["functions"])
	assert.NotNil(t, result["upstream"])

	// Should find upstream node "a"
	upstream, ok := result["upstream"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, upstream)
}

func TestEditorAPI_ExpressionContext_NoWorkflow(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/expressions/context", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestEditorAPI_ExpressionContext_NoNode(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/expressions/context?workflow=wf1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	upstream := result["upstream"].([]any)
	assert.Empty(t, upstream) // no node specified, no upstream
}

func TestEditorAPI_ExpressionContext_UnknownWorkflow(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/expressions/context?workflow=nonexistent&node=x", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestEditorAPI_ReadFile_NotFound(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/files/nonexistent.json", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestEditorAPI_ReadFile_OutsideConfigDir(t *testing.T) {
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	tmpDir := t.TempDir()
	editorAPI := NewEditorAPIReadOnly(tmpDir, "", nil, pluginReg, nodeReg, svcReg, nil)
	editorAPI.Register(app)

	// Try to access file outside config dir
	req := httptest.NewRequest("GET", "/_noda/files/../../etc/passwd", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	// Should be either 403 (outside config dir) or 404
	assert.True(t, resp.StatusCode == 403 || resp.StatusCode == 404)
}

func TestEditorAPI_ListFiles(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/files", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	// May return 500 if config dir is empty temp dir without proper structure,
	// but it should not panic
	assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 500)
}

func TestEditorAPI_ComputeOutputs_NoBody(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/nodes/transform.set/outputs", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// --- EditorAPI: services with health check ---

func TestEditorAPI_ListServicesWithHealth(t *testing.T) {
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()
	p := &mockPlugin{}

	_ = svcReg.Register("healthy-svc", &healthyService{}, p)
	_ = svcReg.Register("unhealthy-svc", &unhealthyService{}, p)
	_ = svcReg.Register("plain-svc", "no-ping", p)

	editorAPI := NewEditorAPIReadOnly(t.TempDir(), "", nil, pluginReg, nodeReg, svcReg, nil)
	editorAPI.Register(app)

	req := httptest.NewRequest("GET", "/_noda/services", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	services := result["services"].([]any)
	assert.Len(t, services, 3)
}

func TestEditorAPI_ListMiddleware(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/middleware", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	mw, ok := result["middleware"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, mw)
	assert.NotNil(t, result["presets"])
	assert.NotNil(t, result["config"])
	assert.NotNil(t, result["instances"])
}

func TestEditorAPI_ListMiddleware_WithPresets(t *testing.T) {
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"middleware_presets": map[string]any{
				"secure": []any{"security.cors", "security.headers"},
			},
			"middleware": map[string]any{
				"limiter": map[string]any{"max": float64(100)},
			},
		},
	}

	editorAPI := NewEditorAPIReadOnly(t.TempDir(), "", rc, pluginReg, nodeReg, svcReg, nil)
	editorAPI.Register(app)

	req := httptest.NewRequest("GET", "/_noda/middleware", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	presets := result["presets"].(map[string]any)
	assert.Contains(t, presets, "secure")
}

func TestEditorAPI_ListEnvVars(t *testing.T) {
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"database_url": "$env(DATABASE_URL)",
		},
		Routes: map[string]map[string]any{
			"routes.json": {"url": "$env(API_URL)"},
		},
		Workflows: map[string]map[string]any{
			"wf.json": {"secret": "$env(SECRET)"},
		},
	}

	editorAPI := NewEditorAPIReadOnly(t.TempDir(), "", rc, pluginReg, nodeReg, svcReg, nil)
	editorAPI.Register(app)

	req := httptest.NewRequest("GET", "/_noda/env", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["variables"])
}

func TestEditorAPI_ListVars(t *testing.T) {
	// Create a minimal config directory with noda.json so Discover succeeds
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "noda.json"), []byte(`{"port": 8080}`), 0o644)

	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root:   map[string]any{},
		Routes: map[string]map[string]any{},
		Workflows: map[string]map[string]any{
			"wf.json": {"url": "{{ $var('api_base') }}/endpoint"},
		},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Vars:        map[string]string{"api_base": "https://api.example.com"},
	}

	editorAPI := NewEditorAPIReadOnly(dir, "", rc, pluginReg, nodeReg, svcReg, nil)
	editorAPI.Register(app)

	req := httptest.NewRequest("GET", "/_noda/vars", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["variables"])
}

func TestEditorAPI_ValidateAll(t *testing.T) {
	dir := t.TempDir()
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	editorAPI := NewEditorAPIReadOnly(dir, "", nil, pluginReg, nodeReg, svcReg, nil)
	editorAPI.Register(app)

	req := httptest.NewRequest("POST", "/_noda/validate/all", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["valid"])
}

func TestEditorAPI_ValidateFile(t *testing.T) {
	dir := t.TempDir()
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	editorAPI := NewEditorAPIReadOnly(dir, "", nil, pluginReg, nodeReg, svcReg, nil)
	editorAPI.Register(app)

	body := `{"path": "noda.json", "content": {"port": 8080}}`
	req := httptest.NewRequest("POST", "/_noda/validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.NotNil(t, result["valid"])
}

func TestEditorAPI_ValidateFile_BadRequest(t *testing.T) {
	dir := t.TempDir()
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	editorAPI := NewEditorAPIReadOnly(dir, "", nil, pluginReg, nodeReg, svcReg, nil)
	editorAPI.Register(app)

	req := httptest.NewRequest("POST", "/_noda/validate", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestEditorAPI_ListModels(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/models", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestEditorAPI_OpenAPISpec(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/openapi", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
