package server

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func triggerTest(t *testing.T, method, path string, body any, headers map[string]string, triggerCfg map[string]any) *TriggerResult {
	t.Helper()

	app := fiber.New()
	compiler := expr.NewCompilerWithFunctions()

	var result *TriggerResult
	var triggerErr error

	app.All("/test/:id?", func(c fiber.Ctx) error {
		result, triggerErr = MapTrigger(c, triggerCfg, compiler)
		if triggerErr != nil {
			return c.Status(500).SendString(triggerErr.Error())
		}
		return c.SendString("ok")
	})

	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = json.Marshal(body)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(bodyBytes))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, triggerErr)

	return result
}

func TestMapTrigger_BodyMapping(t *testing.T) {
	result := triggerTest(t, "POST", "/test", map[string]any{
		"name":  "Alice",
		"email": "alice@example.com",
	}, nil, map[string]any{
		"input": map[string]any{
			"name":  "{{ request.body.name }}",
			"email": "{{ request.body.email }}",
		},
	})

	assert.Equal(t, "Alice", result.Input["name"])
	assert.Equal(t, "alice@example.com", result.Input["email"])
}

func TestMapTrigger_PathParams(t *testing.T) {
	result := triggerTest(t, "GET", "/test/42", nil, nil, map[string]any{
		"input": map[string]any{
			"task_id": "{{ request.params.id }}",
		},
	})

	assert.Equal(t, "42", result.Input["task_id"])
}

func TestMapTrigger_QueryParams(t *testing.T) {
	result := triggerTest(t, "GET", "/test?page=2&per_page=10", nil, nil, map[string]any{
		"input": map[string]any{
			"page":     "{{ request.query.page }}",
			"per_page": "{{ request.query.per_page }}",
		},
	})

	assert.Equal(t, "2", result.Input["page"])
	assert.Equal(t, "10", result.Input["per_page"])
}

func TestMapTrigger_Headers(t *testing.T) {
	result := triggerTest(t, "GET", "/test", nil, map[string]string{
		"X-Custom": "custom-value",
	}, map[string]any{
		"input": map[string]any{
			"custom": "{{ request.headers[\"X-Custom\"] }}",
		},
	})

	assert.Equal(t, "custom-value", result.Input["custom"])
}

func TestMapTrigger_DefaultValues(t *testing.T) {
	result := triggerTest(t, "GET", "/test", nil, nil, map[string]any{
		"input": map[string]any{
			"page": "{{ request.query.page ?? 1 }}",
		},
	})

	// When page is missing, ?? should return the default
	assert.Equal(t, 1, result.Input["page"])
}

func TestMapTrigger_TriggerMetadata(t *testing.T) {
	result := triggerTest(t, "GET", "/test", nil, nil, map[string]any{
		"input": map[string]any{},
	})

	assert.Equal(t, "http", result.Trigger.Type)
	assert.NotEmpty(t, result.Trigger.TraceID)
	assert.False(t, result.Trigger.Timestamp.IsZero())
}

func TestMapTrigger_AuthFromJWT(t *testing.T) {
	app := fiber.New()
	compiler := expr.NewCompilerWithFunctions()

	var result *TriggerResult

	// Simulate JWT middleware setting locals
	app.Use(func(c fiber.Ctx) error {
		c.Locals("jwt_claims", map[string]any{"sub": "user-1", "email": "test@test.com"})
		c.Locals("jwt_user_id", "user-1")
		c.Locals("jwt_roles", []string{"admin"})
		return c.Next()
	})

	app.Get("/test", func(c fiber.Ctx) error {
		var err error
		result, err = MapTrigger(c, map[string]any{"input": map[string]any{}}, compiler)
		if err != nil {
			return err
		}
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	require.NotNil(t, result.Auth)
	assert.Equal(t, "user-1", result.Auth.UserID)
	assert.Equal(t, []string{"admin"}, result.Auth.Roles)
	assert.Equal(t, "test@test.com", result.Auth.Claims["email"])
}

func TestMapTrigger_NoAuth(t *testing.T) {
	result := triggerTest(t, "GET", "/test", nil, nil, map[string]any{
		"input": map[string]any{},
	})

	assert.Nil(t, result.Auth)
}

func TestMapTrigger_RawBody(t *testing.T) {
	app := fiber.New()
	compiler := expr.NewCompilerWithFunctions()

	var result *TriggerResult

	app.Post("/test", func(c fiber.Ctx) error {
		var err error
		result, err = MapTrigger(c, map[string]any{
			"raw_body": true,
			"input": map[string]any{
				"event": "{{ request.body }}",
			},
		}, compiler)
		if err != nil {
			return err
		}
		return c.SendString("ok")
	})

	bodyBytes, _ := json.Marshal(map[string]any{"type": "webhook"})
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)

	// Input mapping should still work
	assert.NotNil(t, result.Input["event"])
}

func TestMapTrigger_StaticValues(t *testing.T) {
	result := triggerTest(t, "GET", "/test", nil, nil, map[string]any{
		"input": map[string]any{
			"static_num": float64(42),
			"static_str": "literal-value",
		},
	})

	// Non-expression values pass through as-is
	assert.Equal(t, float64(42), result.Input["static_num"])
	// String values are resolved but a literal string just returns itself
	assert.Equal(t, "literal-value", result.Input["static_str"])
}
