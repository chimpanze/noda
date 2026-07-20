package server

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openapiTestConfig(openapiBlock map[string]any) *config.ResolvedConfig {
	root := map[string]any{}
	if openapiBlock != nil {
		root["server"] = map[string]any{"openapi": openapiBlock}
	}
	return &config.ResolvedConfig{
		Root: root,
		Routes: map[string]map[string]any{
			"hello": {"id": "hello", "method": "GET", "path": "/hello",
				"trigger": map[string]any{"workflow": "hello", "input": map[string]any{}}},
		},
		Workflows: map[string]map[string]any{"hello": {"nodes": map[string]any{}, "edges": []any{}}},
	}
}

func statusFor(t *testing.T, srv *Server, path string) int {
	t.Helper()
	resp, err := srv.App().Test(httptest.NewRequest("GET", path, nil))
	require.NoError(t, err)
	return resp.StatusCode
}

func TestOpenAPIExposure_DisabledByDefault(t *testing.T) {
	srv, err := NewServer(openapiTestConfig(nil), registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	assert.Equal(t, 404, statusFor(t, srv, "/openapi.json"))
	assert.Equal(t, 404, statusFor(t, srv, "/docs"))
}

func TestOpenAPIExposure_EnabledServesBoth(t *testing.T) {
	srv, err := NewServer(openapiTestConfig(map[string]any{"enabled": true}),
		registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	assert.Equal(t, 200, statusFor(t, srv, "/openapi.json"))
	assert.Equal(t, 200, statusFor(t, srv, "/docs"))
}

func TestOpenAPIExposure_DocsFalseHidesUI(t *testing.T) {
	srv, err := NewServer(openapiTestConfig(map[string]any{"enabled": true, "docs": false}),
		registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	assert.Equal(t, 200, statusFor(t, srv, "/openapi.json"))
	assert.Equal(t, 404, statusFor(t, srv, "/docs"))
}

func TestOpenAPIExposure_CustomPaths(t *testing.T) {
	srv, err := NewServer(openapiTestConfig(map[string]any{
		"enabled": true, "path": "/spec.json", "docs_path": "/api-docs"}),
		registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	assert.Equal(t, 200, statusFor(t, srv, "/spec.json"))
	assert.Equal(t, 404, statusFor(t, srv, "/openapi.json"))

	resp, err := srv.App().Test(httptest.NewRequest("GET", "/api-docs", nil))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), `data-url="/spec.json"`)
}
