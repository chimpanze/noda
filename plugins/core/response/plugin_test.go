package response

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockContext implements api.ExecutionContext for testing.
type mockContext struct {
	input   any
	auth    *api.AuthData
	trigger api.TriggerData
}

func (m *mockContext) Input() any               { return m.input }
func (m *mockContext) Auth() *api.AuthData      { return m.auth }
func (m *mockContext) Trigger() api.TriggerData { return m.trigger }
func (m *mockContext) Resolve(expr string) (any, error) {
	// Simple: return the expression as-is (treats it as a literal)
	return expr, nil
}
func (m *mockContext) ResolveWithVars(expr string, _ map[string]any) (any, error) {
	return expr, nil
}
func (m *mockContext) Log(string, string, map[string]any) {}

func newTestContext() *mockContext {
	return &mockContext{
		trigger: api.TriggerData{
			Type:      "http",
			Timestamp: time.Now(),
			TraceID:   "test-trace-id",
		},
	}
}

func TestPlugin_Registration(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "core.response", p.Name())
	assert.Equal(t, "response", p.Prefix())
	assert.Len(t, p.Nodes(), 3)
	assert.False(t, p.HasServices())
}

func TestJSONExecutor_BasicResponse(t *testing.T) {
	exec := newJSONExecutor(nil)
	assert.Equal(t, []string{"success", "error"}, exec.Outputs())

	output, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status": "200",
		"body":   "hello",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	resp, ok := data.(*api.HTTPResponse)
	require.True(t, ok)
	assert.Equal(t, 200, resp.Status)
	assert.Equal(t, "hello", resp.Body)
}

func TestJSONExecutor_WithHeaders(t *testing.T) {
	exec := newJSONExecutor(nil)

	output, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status": "201",
		"body":   "created",
		"headers": map[string]any{
			"X-Custom": "value",
		},
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 201, resp.Status)
	assert.Equal(t, "value", resp.Headers["X-Custom"])
}

func TestJSONExecutor_DefaultStatus(t *testing.T) {
	exec := newJSONExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"body": "ok",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 200, resp.Status)
}

func TestRedirectExecutor(t *testing.T) {
	exec := newRedirectExecutor(nil)
	assert.Equal(t, []string{"success", "error"}, exec.Outputs())

	output, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"url": "/new-location",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 302, resp.Status)
	assert.Equal(t, "/new-location", resp.Headers["Location"])
	assert.Nil(t, resp.Body)
}

func TestRedirectExecutor_CustomStatus(t *testing.T) {
	exec := newRedirectExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"url":    "/permanent",
		"status": float64(301),
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 301, resp.Status)
}

func TestRedirectExecutor_MissingURL(t *testing.T) {
	exec := newRedirectExecutor(nil)

	_, _, err := exec.Execute(context.Background(), newTestContext(), map[string]any{}, nil)
	assert.Error(t, err)
}

func TestErrorExecutor(t *testing.T) {
	exec := newErrorExecutor(nil)
	assert.Equal(t, []string{"success", "error"}, exec.Outputs())

	output, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status":  "404",
		"code":    "NOT_FOUND",
		"message": "Task not found",
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 404, resp.Status)

	body := resp.Body.(map[string]any)
	errData := body["error"].(map[string]any)
	assert.Equal(t, "NOT_FOUND", errData["code"])
	assert.Equal(t, "Task not found", errData["message"])
	assert.Equal(t, "test-trace-id", errData["trace_id"])
}

func TestErrorExecutor_WithDetails(t *testing.T) {
	exec := newErrorExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status":  "422",
		"code":    "VALIDATION_ERROR",
		"message": "Invalid input",
		"details": "field errors here",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	body := resp.Body.(map[string]any)
	errData := body["error"].(map[string]any)
	assert.Equal(t, "field errors here", errData["details"])
}

// --- mockResolveContext allows controlling Resolve return values per expression ---

type mockResolveContext struct {
	trigger    api.TriggerData
	resolveMap map[string]any
	resolveErr map[string]error
}

func (m *mockResolveContext) Input() any               { return nil }
func (m *mockResolveContext) Auth() *api.AuthData      { return nil }
func (m *mockResolveContext) Trigger() api.TriggerData { return m.trigger }
func (m *mockResolveContext) Resolve(expr string) (any, error) {
	if m.resolveErr != nil {
		if err, ok := m.resolveErr[expr]; ok {
			return nil, err
		}
	}
	if m.resolveMap != nil {
		if v, ok := m.resolveMap[expr]; ok {
			return v, nil
		}
	}
	return expr, nil
}
func (m *mockResolveContext) ResolveWithVars(expr string, _ map[string]any) (any, error) {
	return m.Resolve(expr)
}
func (m *mockResolveContext) Log(string, string, map[string]any) {}

func newResolveContext(resolveMap map[string]any) *mockResolveContext {
	return &mockResolveContext{
		trigger: api.TriggerData{
			Type:    "http",
			TraceID: "test-trace-id",
		},
		resolveMap: resolveMap,
	}
}

func newErrorResolveContext(resolveErr map[string]error) *mockResolveContext {
	return &mockResolveContext{
		trigger: api.TriggerData{
			Type:    "http",
			TraceID: "test-trace-id",
		},
		resolveErr: resolveErr,
	}
}

// --- Plugin methods ---

func TestPlugin_CreateService(t *testing.T) {
	p := &Plugin{}
	svc, err := p.CreateService(nil)
	assert.NoError(t, err)
	assert.Nil(t, svc)
}

func TestPlugin_HealthCheck(t *testing.T) {
	p := &Plugin{}
	assert.NoError(t, p.HealthCheck(nil))
}

func TestPlugin_Shutdown(t *testing.T) {
	p := &Plugin{}
	assert.NoError(t, p.Shutdown(nil))
}

// --- JSON Descriptor ---

func TestJSONDescriptor(t *testing.T) {
	d := &jsonDescriptor{}
	assert.Equal(t, "json", d.Name())
	assert.Nil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	require.NotNil(t, schema)
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "status")
	assert.Contains(t, props, "body")
	assert.Contains(t, props, "headers")
	assert.Contains(t, props, "cookies")
	req := schema["required"].([]any)
	assert.Contains(t, req, "status")
	assert.Contains(t, req, "body")
}

// --- Error Descriptor ---

func TestErrorDescriptor(t *testing.T) {
	d := &errorDescriptor{}
	assert.Equal(t, "error", d.Name())
	assert.Nil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	require.NotNil(t, schema)
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "status")
	assert.Contains(t, props, "code")
	assert.Contains(t, props, "message")
	assert.Contains(t, props, "details")
	req := schema["required"].([]any)
	assert.Contains(t, req, "status")
	assert.Contains(t, req, "code")
	assert.Contains(t, req, "message")
}

// --- Redirect Descriptor ---

func TestRedirectDescriptor(t *testing.T) {
	d := &redirectDescriptor{}
	assert.Equal(t, "redirect", d.Name())
	assert.Nil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	require.NotNil(t, schema)
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "url")
	assert.Contains(t, props, "status")
	req := schema["required"].([]any)
	assert.Contains(t, req, "url")
}

// --- JSON Executor: map body (resolveDeep on map) ---

func TestJSONExecutor_MapBody(t *testing.T) {
	exec := newJSONExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status": "200",
		"body": map[string]any{
			"name":  "Alice",
			"count": "42",
		},
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	body := resp.Body.(map[string]any)
	assert.Equal(t, "Alice", body["name"])
	assert.Equal(t, "42", body["count"])
}

// --- JSON Executor: array body (resolveDeep on []any) ---

func TestJSONExecutor_ArrayBody(t *testing.T) {
	exec := newJSONExecutor(nil)

	// Body as []any triggers the default branch (not string, not map)
	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status": "200",
		"body":   []any{"a", "b", "c"},
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	body := resp.Body.([]any)
	assert.Equal(t, []any{"a", "b", "c"}, body)
}

// --- JSON Executor: numeric status (float64, non-string) ---

func TestJSONExecutor_NumericStatus(t *testing.T) {
	exec := newJSONExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status": float64(201),
		"body":   "created",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 201, resp.Status)
}

// --- JSON Executor: integer status (non-string) ---

func TestJSONExecutor_IntStatus(t *testing.T) {
	exec := newJSONExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status": 204,
		"body":   "no content",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 204, resp.Status)
}

// --- JSON Executor: non-convertible status defaults to 200 ---

func TestJSONExecutor_NonConvertibleStatus(t *testing.T) {
	exec := newJSONExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status": true, // bool can't be converted to int by ToInt
		"body":   "ok",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 200, resp.Status)
}

// --- JSON Executor: cookies with all fields ---

func TestJSONExecutor_CookiesWithAllFields(t *testing.T) {
	exec := newJSONExecutor(nil)
	mCtx := newResolveContext(map[string]any{
		"cookie-expr": []any{
			map[string]any{
				"name":      "session",
				"value":     "abc123",
				"path":      "/",
				"domain":    "example.com",
				"max_age":   float64(3600),
				"secure":    true,
				"http_only": true,
				"same_site": "Strict",
			},
		},
	})

	_, data, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status":  "200",
		"body":    "ok",
		"cookies": "cookie-expr",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	require.Len(t, resp.Cookies, 1)
	c := resp.Cookies[0]
	assert.Equal(t, "session", c.Name)
	assert.Equal(t, "abc123", c.Value)
	assert.Equal(t, "/", c.Path)
	assert.Equal(t, "example.com", c.Domain)
	assert.Equal(t, 3600, c.MaxAge)
	assert.True(t, c.Secure)
	assert.True(t, c.HTTPOnly)
	assert.Equal(t, "Strict", c.SameSite)
}

// --- JSON Executor: cookies with multiple items ---

func TestJSONExecutor_MultipleCookies(t *testing.T) {
	exec := newJSONExecutor(nil)
	mCtx := newResolveContext(map[string]any{
		"cookies-expr": []any{
			map[string]any{"name": "a", "value": "1"},
			map[string]any{"name": "b", "value": "2"},
		},
	})

	_, data, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status":  "200",
		"body":    "ok",
		"cookies": "cookies-expr",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	require.Len(t, resp.Cookies, 2)
	assert.Equal(t, "a", resp.Cookies[0].Name)
	assert.Equal(t, "b", resp.Cookies[1].Name)
}

// --- JSON Executor: non-array cookies returns nil ---

func TestJSONExecutor_NonArrayCookies(t *testing.T) {
	exec := newJSONExecutor(nil)
	mCtx := newResolveContext(map[string]any{
		"bad-cookies": "not-an-array",
	})

	_, data, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status":  "200",
		"body":    "ok",
		"cookies": "bad-cookies",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Nil(t, resp.Cookies)
}

// --- JSON Executor: cookies array with non-map item ---

func TestJSONExecutor_CookiesNonMapItem(t *testing.T) {
	exec := newJSONExecutor(nil)
	mCtx := newResolveContext(map[string]any{
		"mixed-cookies": []any{
			"not-a-map",
			map[string]any{"name": "valid", "value": "yes"},
		},
	})

	_, data, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status":  "200",
		"body":    "ok",
		"cookies": "mixed-cookies",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	require.Len(t, resp.Cookies, 1)
	assert.Equal(t, "valid", resp.Cookies[0].Name)
}

// --- JSON Executor: cookies resolve error ---

func TestJSONExecutor_CookiesResolveError(t *testing.T) {
	exec := newJSONExecutor(nil)
	mCtx := newErrorResolveContext(map[string]error{
		"bad-cookie": fmt.Errorf("cookie resolve failed"),
	})

	_, _, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status":  "200",
		"body":    "ok",
		"cookies": "bad-cookie",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cookies")
}

// --- JSON Executor: status resolve error ---

func TestJSONExecutor_StatusResolveError(t *testing.T) {
	exec := newJSONExecutor(nil)
	mCtx := newErrorResolveContext(map[string]error{
		"bad-status": fmt.Errorf("status resolve failed"),
	})

	_, _, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status": "bad-status",
		"body":   "ok",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status")
}

// --- JSON Executor: body resolve error (string body) ---

func TestJSONExecutor_BodyResolveError(t *testing.T) {
	exec := newJSONExecutor(nil)
	mCtx := newErrorResolveContext(map[string]error{
		"bad-body": fmt.Errorf("body resolve failed"),
	})

	_, _, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status": "200",
		"body":   "bad-body",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "body")
}

// --- JSON Executor: map body resolve error (resolveDeep) ---

func TestJSONExecutor_MapBodyResolveError(t *testing.T) {
	exec := newJSONExecutor(nil)
	mCtx := newErrorResolveContext(map[string]error{
		"bad-field": fmt.Errorf("field resolve failed"),
	})

	_, _, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status": "200",
		"body": map[string]any{
			"key": "bad-field",
		},
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "body")
}

// --- JSON Executor: empty status string defaults to 200 ---

func TestJSONExecutor_EmptyStatusString(t *testing.T) {
	exec := newJSONExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status": "",
		"body":   "ok",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 200, resp.Status)
}

// --- JSON Executor: scalar body (non-string, non-map) ---

func TestJSONExecutor_ScalarBody(t *testing.T) {
	exec := newJSONExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status": "200",
		"body":   float64(42),
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, float64(42), resp.Body)
}

// --- JSON Executor: nil body ---

func TestJSONExecutor_NilBody(t *testing.T) {
	exec := newJSONExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status": "200",
		"body":   nil,
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Nil(t, resp.Body)
}

// --- Error Executor: default 500 status (no status key) ---

func TestErrorExecutor_Default500Status(t *testing.T) {
	exec := newErrorExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"code":    "INTERNAL",
		"message": "Something went wrong",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 500, resp.Status)
}

// --- Error Executor: numeric status (non-string, float64) ---

func TestErrorExecutor_NumericStatus(t *testing.T) {
	exec := newErrorExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status":  float64(403),
		"code":    "FORBIDDEN",
		"message": "Access denied",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 403, resp.Status)
}

// --- Error Executor: empty status string defaults to 500 ---

func TestErrorExecutor_EmptyStatusString(t *testing.T) {
	exec := newErrorExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status":  "",
		"code":    "INTERNAL",
		"message": "Error",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 500, resp.Status)
}

// --- Error Executor: status resolve error ---

func TestErrorExecutor_StatusResolveError(t *testing.T) {
	exec := newErrorExecutor(nil)
	mCtx := newErrorResolveContext(map[string]error{
		"bad-status": fmt.Errorf("status error"),
	})

	_, _, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status":  "bad-status",
		"code":    "ERR",
		"message": "msg",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status")
}

// --- Error Executor: code resolve error ---

func TestErrorExecutor_CodeResolveError(t *testing.T) {
	exec := newErrorExecutor(nil)
	mCtx := newErrorResolveContext(map[string]error{
		"bad-code": fmt.Errorf("code error"),
	})

	_, _, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status":  "500",
		"code":    "bad-code",
		"message": "msg",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "code")
}

// --- Error Executor: message resolve error ---

func TestErrorExecutor_MessageResolveError(t *testing.T) {
	exec := newErrorExecutor(nil)
	mCtx := newErrorResolveContext(map[string]error{
		"bad-msg": fmt.Errorf("message error"),
	})

	_, _, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status":  "500",
		"code":    "ERR",
		"message": "bad-msg",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "message")
}

// --- Error Executor: details resolve error ---

func TestErrorExecutor_DetailsResolveError(t *testing.T) {
	exec := newErrorExecutor(nil)
	mCtx := newErrorResolveContext(map[string]error{
		"bad-details": fmt.Errorf("details error"),
	})

	_, _, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status":  "500",
		"code":    "ERR",
		"message": "msg",
		"details": "bad-details",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "details")
}

// --- Error Executor: no details field omits details from body ---

func TestErrorExecutor_NoDetails(t *testing.T) {
	exec := newErrorExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status":  "400",
		"code":    "BAD_REQUEST",
		"message": "Bad request",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	body := resp.Body.(map[string]any)
	errData := body["error"].(map[string]any)
	_, hasDetails := errData["details"]
	assert.False(t, hasDetails)
}

// --- Error Executor: non-convertible numeric status ---

func TestErrorExecutor_NonConvertibleStatus(t *testing.T) {
	exec := newErrorExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status":  true,
		"code":    "ERR",
		"message": "msg",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 500, resp.Status)
}

// --- resolveDeep: nested arrays ---

func TestResolveDeep_NestedArrays(t *testing.T) {
	mCtx := newResolveContext(map[string]any{
		"inner-val": "resolved",
	})

	result, err := resolveDeep(mCtx, []any{
		"inner-val",
		[]any{"inner-val", "literal"},
		map[string]any{"key": "inner-val"},
	})
	require.NoError(t, err)

	arr := result.([]any)
	assert.Equal(t, "resolved", arr[0])

	innerArr := arr[1].([]any)
	assert.Equal(t, "resolved", innerArr[0])
	assert.Equal(t, "literal", innerArr[1])

	innerMap := arr[2].(map[string]any)
	assert.Equal(t, "resolved", innerMap["key"])
}

// --- resolveDeep: scalar passthrough ---

func TestResolveDeep_ScalarPassthrough(t *testing.T) {
	mCtx := newTestContext()

	result, err := resolveDeep(mCtx, float64(42))
	require.NoError(t, err)
	assert.Equal(t, float64(42), result)

	result, err = resolveDeep(mCtx, true)
	require.NoError(t, err)
	assert.Equal(t, true, result)

	result, err = resolveDeep(mCtx, nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// --- resolveDeep: array resolve error ---

func TestResolveDeep_ArrayResolveError(t *testing.T) {
	mCtx := newErrorResolveContext(map[string]error{
		"bad-elem": fmt.Errorf("element error"),
	})

	_, err := resolveDeep(mCtx, []any{"bad-elem"})
	assert.Error(t, err)
}

// --- resolveDeep: map resolve error ---

func TestResolveDeep_MapResolveError(t *testing.T) {
	mCtx := newErrorResolveContext(map[string]error{
		"bad-val": fmt.Errorf("value error"),
	})

	_, err := resolveDeep(mCtx, map[string]any{"k": "bad-val"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "field")
}

// --- resolveDeep: string resolve error ---

func TestResolveDeep_StringResolveError(t *testing.T) {
	mCtx := newErrorResolveContext(map[string]error{
		"bad-str": fmt.Errorf("string error"),
	})

	_, err := resolveDeep(mCtx, "bad-str")
	assert.Error(t, err)
}

// --- Redirect Executor: URL resolve error ---

func TestRedirectExecutor_URLResolveError(t *testing.T) {
	exec := newRedirectExecutor(nil)
	mCtx := newErrorResolveContext(map[string]error{
		"bad-url": fmt.Errorf("url error"),
	})

	_, _, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"url": "bad-url",
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "url")
}

// --- JSON Executor: headers resolve error ---

func TestJSONExecutor_HeadersResolveError(t *testing.T) {
	exec := newJSONExecutor(nil)

	// Pass headers as non-map to trigger error from ResolveHeaders
	_, _, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status":  "200",
		"body":    "ok",
		"headers": "not-a-map",
	}, nil)
	assert.Error(t, err)
}

// --- JSON Executor: empty cookies string is not resolved ---

func TestJSONExecutor_EmptyCookiesString(t *testing.T) {
	exec := newJSONExecutor(nil)

	_, data, err := exec.Execute(context.Background(), newTestContext(), map[string]any{
		"status":  "200",
		"body":    "ok",
		"cookies": "",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Nil(t, resp.Cookies)
}

// --- JSON Executor: status resolved to non-int string keeps default ---

func TestJSONExecutor_StatusNonIntString(t *testing.T) {
	exec := newJSONExecutor(nil)
	mCtx := newResolveContext(map[string]any{
		"non-int-status": "not-a-number",
	})

	_, data, err := exec.Execute(context.Background(), mCtx, map[string]any{
		"status": "non-int-status",
		"body":   "ok",
	}, nil)
	require.NoError(t, err)

	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 200, resp.Status)
}
