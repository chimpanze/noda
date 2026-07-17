package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
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

func TestMapTrigger_RequestMeta(t *testing.T) {
	result := triggerTest(t, "GET", "/test", nil, map[string]string{
		"User-Agent": "curl/8.6.0",
	}, map[string]any{})

	assert.Equal(t, "curl/8.6.0", result.Trigger.UserAgent)
	assert.NotEmpty(t, result.Trigger.ClientIP)
}

func TestMapTrigger_BodyMapping(t *testing.T) {
	result := triggerTest(t, "POST", "/test", map[string]any{
		"name":  "Alice",
		"email": "alice@example.com",
	}, nil, map[string]any{
		"input": map[string]any{
			"name":  "{{ body.name }}",
			"email": "{{ body.email }}",
		},
	})

	assert.Equal(t, "Alice", result.Input["name"])
	assert.Equal(t, "alice@example.com", result.Input["email"])
}

func TestMapTrigger_PathParams(t *testing.T) {
	result := triggerTest(t, "GET", "/test/42", nil, nil, map[string]any{
		"input": map[string]any{
			"task_id": "{{ params.id }}",
		},
	})

	assert.Equal(t, 42, result.Input["task_id"])
}

func TestMapTrigger_QueryParams(t *testing.T) {
	result := triggerTest(t, "GET", "/test?page=2&per_page=10", nil, nil, map[string]any{
		"input": map[string]any{
			"page":     "{{ query.page }}",
			"per_page": "{{ query.per_page }}",
		},
	})

	assert.Equal(t, 2, result.Input["page"])
	assert.Equal(t, 10, result.Input["per_page"])
}

func TestMapTrigger_Headers(t *testing.T) {
	result := triggerTest(t, "GET", "/test", nil, map[string]string{
		"X-Custom": "custom-value",
	}, map[string]any{
		"input": map[string]any{
			"custom": "{{ headers[\"X-Custom\"] }}",
		},
	})

	assert.Equal(t, "custom-value", result.Input["custom"])
}

func TestMapTrigger_DefaultValues(t *testing.T) {
	result := triggerTest(t, "GET", "/test", nil, nil, map[string]any{
		"input": map[string]any{
			"page": "{{ query.page ?? 1 }}",
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
		c.Locals(api.LocalJWTClaims, map[string]any{"sub": "user-1", "email": "test@test.com"})
		c.Locals(api.LocalJWTUserID, "user-1")
		c.Locals(api.LocalJWTRoles, []string{"admin"})
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
				"event": "{{ body }}",
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

func TestMapTrigger_RawBodyMirrorToRequest(t *testing.T) {
	// Test that raw_body is mirrored to request.raw_body when flag is enabled
	app := fiber.New()
	compiler := expr.NewCompilerWithFunctions()

	var result *TriggerResult

	app.Post("/test", func(c fiber.Ctx) error {
		var err error
		result, err = MapTrigger(c, map[string]any{
			"raw_body": true,
			"input": map[string]any{
				"raw":     "{{ raw_body }}",
				"req_raw": "{{ request.raw_body }}",
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

	// Both raw_body and request.raw_body should resolve to the same value
	assert.Equal(t, result.Input["raw"], result.Input["req_raw"])
	assert.NotEmpty(t, result.Input["req_raw"])
}

func TestMapTrigger_RawBodyAbsentWhenDisabled(t *testing.T) {
	// Test that request.raw_body is absent when raw_body flag is not set
	app := fiber.New()
	compiler := expr.NewCompilerWithFunctions()

	var result *TriggerResult
	var resolveErr error

	app.Post("/test", func(c fiber.Ctx) error {
		var err error
		result, err = MapTrigger(c, map[string]any{
			// raw_body: false (or not set)
			"input": map[string]any{
				"has_raw": "{{ request.raw_body ?? 'absent' }}",
			},
		}, compiler)
		if err != nil {
			resolveErr = err
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

	// When raw_body flag is not set, request.raw_body should be absent,
	// so the ?? operator should use the default value 'absent'
	assert.Equal(t, "absent", result.Input["has_raw"])
	assert.Nil(t, resolveErr)
}

func TestBuildRawRequestContext_HasRequestAlias(t *testing.T) {
	app := fiber.New()
	var got map[string]any
	app.Post("/x/:name", func(c fiber.Ctx) error {
		got = buildRawRequestContext(c)
		return c.SendStatus(200)
	})
	req := httptest.NewRequest("POST", "/x/alice?q=1", strings.NewReader(`{"k":"v"}`))
	req.Header.Set("Content-Type", "application/json")
	_, _ = app.Test(req)

	require.Contains(t, got, "request")
	reqMap, ok := got["request"].(map[string]any)
	require.True(t, ok)
	// request.params mirrors top-level params
	require.Equal(t, got["params"], reqMap["params"])
	require.Equal(t, got["body"], reqMap["body"])
	require.Equal(t, got["query"], reqMap["query"])
}

// triggerTestRaw is like triggerTest but sends a raw body with an explicit content type.
func triggerTestRaw(t *testing.T, method, path, rawBody, contentType string, triggerCfg map[string]any) *TriggerResult {
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

	req := httptest.NewRequest(method, path, strings.NewReader(rawBody))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, triggerErr)

	return result
}

func TestMapTrigger_JSONBodyStringsNotCoerced(t *testing.T) {
	// #331: JSON preserves types — numeric-looking strings must stay strings.
	result := triggerTest(t, "POST", "/test", map[string]any{"zip": "0042", "count": 7}, nil, map[string]any{
		"input": map[string]any{
			"zip":   "{{ body.zip }}",
			"count": "{{ body.count }}",
		},
	})
	assert.Equal(t, "0042", result.Input["zip"])
	assert.Equal(t, float64(7), result.Input["count"]) // JSON number passes through untouched
}

func TestMapTrigger_FormBodyStringsCoerced(t *testing.T) {
	// Form bodies are string-typed transport — coercion keeps working there.
	result := triggerTestRaw(t, "POST", "/test", "amount=42&note=0042x",
		"application/x-www-form-urlencoded", map[string]any{
			"input": map[string]any{
				"amount": "{{ body.amount }}",
				"note":   "{{ body.note }}",
			},
		})
	assert.Equal(t, 42, result.Input["amount"])
	assert.Equal(t, "0042x", result.Input["note"])
}

func TestMapTrigger_FormBodyStringsCoerced_UppercaseContentType(t *testing.T) {
	// Content-Type media types are case-insensitive per RFC 7231 — an
	// uppercase form media type must still be treated as string-typed.
	result := triggerTestRaw(t, "POST", "/test", "amount=42&note=0042x",
		"APPLICATION/X-WWW-FORM-URLENCODED", map[string]any{
			"input": map[string]any{
				"amount": "{{ body.amount }}",
				"note":   "{{ body.note }}",
			},
		})
	assert.Equal(t, 42, result.Input["amount"])
	assert.Equal(t, "0042x", result.Input["note"])
}

func TestMapTrigger_MultipartFormBody_UppercaseContentType(t *testing.T) {
	// #339: Content-Type media types are case-insensitive per RFC 7231 — an
	// uppercase multipart media type (boundary parameter kept verbatim) must
	// still be parsed into form fields.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.WriteField("field", "hello"))
	require.NoError(t, mw.Close())

	contentType := "MULTIPART/FORM-DATA; boundary=" + mw.Boundary()

	result := triggerTestRaw(t, "POST", "/test", buf.String(), contentType, map[string]any{
		"input": map[string]any{
			"field": "{{ body.field }}",
		},
	})
	assert.Equal(t, "hello", result.Input["field"])
}

func TestMapTrigger_FormBodyPartialParseLeniency(t *testing.T) {
	// #339: a malformed percent-escape in one pair must not discard the
	// pairs that did parse successfully (regression pin for the
	// `len(values) > 0` gate around url.ParseQuery's error).
	result := triggerTestRaw(t, "POST", "/test", "a=1&b=%zz",
		"application/x-www-form-urlencoded", map[string]any{
			"input": map[string]any{
				"a": "{{ body.a }}",
			},
		})
	assert.Equal(t, 1, result.Input["a"])
}

func TestMapTrigger_TransportRefsStillCoerced(t *testing.T) {
	result := triggerTest(t, "GET", "/test/0042?page=2", nil, map[string]string{
		"X-Page-Size": "50",
	}, map[string]any{
		"input": map[string]any{
			"id":    "{{ params.id }}",
			"page":  "{{ query.page }}",
			"size":  "{{ headers[\"X-Page-Size\"] }}",
			"page2": "{{ request.query.page }}",
		},
	})
	assert.Equal(t, 42, result.Input["id"])
	assert.Equal(t, 2, result.Input["page"])
	assert.Equal(t, 50, result.Input["size"])
	assert.Equal(t, 2, result.Input["page2"])
}

func TestMapTrigger_ComputedAndLiteralNotCoerced(t *testing.T) {
	result := triggerTest(t, "GET", "/test?a=4", nil, nil, map[string]any{
		"input": map[string]any{
			"lit":  "9",                     // literal string: author's type wins
			"comp": "{{ query.a + \"1\" }}", // computed: expression's result type wins
		},
	})
	assert.Equal(t, "9", result.Input["lit"])
	assert.Equal(t, "41", result.Input["comp"])
}

func TestMapTrigger_CoerceOptOut(t *testing.T) {
	result := triggerTest(t, "GET", "/test/0042?page=2", nil, nil, map[string]any{
		"coerce": false,
		"input": map[string]any{
			"id":   "{{ params.id }}",
			"page": "{{ query.page }}",
		},
	})
	assert.Equal(t, "0042", result.Input["id"])
	assert.Equal(t, "2", result.Input["page"])
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

func TestShouldCoerce(t *testing.T) {
	tests := []struct {
		name            string
		exprStr         string
		bodyStringTyped bool
		want            bool
	}{
		{"nested body ref, form body", "{{ body.user.zip }}", true, true},
		{"nested body ref, json body", "{{ body.user.zip }}", false, false},
		{"bare query namespace, not a member access", "{{ query }}", false, false},
		{"computed default with ??", `{{ query.a ?? "x" }}`, false, false},
		{"ref embedded in surrounding text", "a{{ query.b }}c", false, false},
		{"request.* alias params ref", "{{ request.params.id }}", false, true},
		{"bracket header ref", `{{ headers["X-Y"] }}`, false, true},
		{"plain literal", `"9"`, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldCoerce(tt.exprStr, tt.bodyStringTyped))
		})
	}
}

func TestParseHeadersLowercasesKeys(t *testing.T) {
	app := fiber.New()
	var got map[string]any
	app.Post("/t", func(c fiber.Ctx) error {
		got = parseHeaders(c)
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/t", nil)
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("Content-Type", "application/json")
	_, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, "issues", got["x-github-event"])
	assert.Equal(t, "application/json", got["content-type"])
	_, canonical := got["X-Github-Event"]
	assert.False(t, canonical, "canonical-case key must not be present")
}
