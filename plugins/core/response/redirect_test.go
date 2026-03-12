package response

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockExecCtx struct{}

func (m *mockExecCtx) Input() any                                                 { return nil }
func (m *mockExecCtx) Auth() *api.AuthData                                        { return nil }
func (m *mockExecCtx) Trigger() api.TriggerData                                   { return api.TriggerData{} }
func (m *mockExecCtx) Resolve(expr string) (any, error)                           { return expr, nil }
func (m *mockExecCtx) ResolveWithVars(expr string, _ map[string]any) (any, error) { return expr, nil }
func (m *mockExecCtx) GetOutput(string) (any, bool)                               { return nil, false }
func (m *mockExecCtx) Log(string, string, map[string]any)                         {}

func TestRedirect_ValidURLs(t *testing.T) {
	executor := &redirectExecutor{}

	tests := []struct {
		name string
		url  string
	}{
		{"relative path", "/dashboard"},
		{"http url", "http://example.com/page"},
		{"https url", "https://example.com/page"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]any{"url": tt.url}
			output, data, err := executor.Execute(context.Background(), &mockExecCtx{}, config, nil)
			require.NoError(t, err)
			assert.Equal(t, api.OutputSuccess, output)
			resp := data.(*api.HTTPResponse)
			assert.Equal(t, tt.url, resp.Headers["Location"])
			assert.Equal(t, 302, resp.Status)
		})
	}
}

func TestRedirect_HeaderInjection(t *testing.T) {
	executor := &redirectExecutor{}

	tests := []struct {
		name string
		url  string
	}{
		{"newline injection", "/page\nX-Injected: true"},
		{"carriage return injection", "/page\rX-Injected: true"},
		{"crlf injection", "http://example.com\r\nX-Injected: true"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]any{"url": tt.url}
			_, _, err := executor.Execute(context.Background(), &mockExecCtx{}, config, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid characters")
		})
	}
}

func TestRedirect_InvalidScheme(t *testing.T) {
	executor := &redirectExecutor{}

	tests := []struct {
		name string
		url  string
	}{
		{"javascript scheme", "javascript:alert(1)"},
		{"data scheme", "data:text/html,<h1>hi</h1>"},
		{"protocol-relative", "//evil.com/page"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := map[string]any{"url": tt.url}
			_, _, err := executor.Execute(context.Background(), &mockExecCtx{}, config, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must start with")
		})
	}
}

func TestRedirect_CustomStatus(t *testing.T) {
	executor := &redirectExecutor{}
	config := map[string]any{"url": "/new-location", "status": float64(301)}

	output, data, err := executor.Execute(context.Background(), &mockExecCtx{}, config, nil)
	require.NoError(t, err)
	assert.Equal(t, api.OutputSuccess, output)
	resp := data.(*api.HTTPResponse)
	assert.Equal(t, 301, resp.Status)
}
