package response

import (
	"context"
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

func (m *mockContext) Input() any           { return m.input }
func (m *mockContext) Auth() *api.AuthData   { return m.auth }
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
