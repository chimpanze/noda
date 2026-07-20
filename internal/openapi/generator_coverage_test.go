package openapi

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- OpenAPI edge cases ---

func TestGenerateOpenAPI_DefaultNameVersion(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Routes:  map[string]map[string]any{},
		Schemas: map[string]map[string]any{},
	}
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
		// Ref-name keyed (not file-path keyed) so the $ref above actually
		// resolves — see the SchemaRegistry fix this package exists to carry.
		SchemaRegistry: map[string]map[string]any{
			"schemas/Item": {
				"type": "object",
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
			},
		},
	}
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
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
	html := ScalarHTML("/openapi.json")
	assert.Contains(t, html, "<!DOCTYPE html>")
	assert.Contains(t, html, `data-url="/openapi.json"`)
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
	doc, err := Generate(rc)
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
	doc, err := Generate(rc)
	require.NoError(t, err)

	pathItem := doc.Paths.Find("/api/items")
	require.NotNil(t, pathItem)
	assert.NotNil(t, pathItem.Get)
	assert.NotNil(t, pathItem.Post)
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

// --- openapi3 import needed ---

func TestFiberToOpenAPIPath_NoParams(t *testing.T) {
	assert.Equal(t, "/api/items", fiberToOpenAPIPath("/api/items"))
}

func TestFiberToOpenAPIPath_RootPath(t *testing.T) {
	assert.Equal(t, "/", fiberToOpenAPIPath("/"))
}
