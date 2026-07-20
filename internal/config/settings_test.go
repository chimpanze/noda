package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rootWith(server map[string]any) map[string]any {
	return map[string]any{"server": server}
}

func TestServerInt(t *testing.T) {
	tests := []struct {
		name    string
		root    map[string]any
		wantVal int
		wantOK  bool
		wantErr string
	}{
		{"json number", rootWith(map[string]any{"body_limit": float64(1048576)}), 1048576, true, ""},
		{"numeric string from env", rootWith(map[string]any{"body_limit": "1073741824"}), 1073741824, true, ""},
		{"numeric string with spaces", rootWith(map[string]any{"body_limit": " 42 "}), 42, true, ""},
		{"absent key", rootWith(map[string]any{}), 0, false, ""},
		{"absent server section", map[string]any{}, 0, false, ""},
		{"garbage string", rootWith(map[string]any{"body_limit": "10GB"}), 0, false, `server.body_limit`},
		{"wrong type", rootWith(map[string]any{"body_limit": true}), 0, false, `server.body_limit`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok, err := ServerInt(tt.root, "body_limit")
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantVal, v)
		})
	}
}

func TestServerDuration(t *testing.T) {
	v, ok, err := ServerDuration(rootWith(map[string]any{"read_timeout": "30s"}), "read_timeout")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 30*time.Second, v)

	_, ok, err = ServerDuration(rootWith(map[string]any{}), "read_timeout")
	require.NoError(t, err)
	assert.False(t, ok)

	_, _, err = ServerDuration(rootWith(map[string]any{"read_timeout": "banana"}), "read_timeout")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server.read_timeout")

	_, _, err = ServerDuration(rootWith(map[string]any{"read_timeout": float64(30)}), "read_timeout")
	require.Error(t, err) // durations are strings; numbers were never supported
}

func TestServerTrustProxy(t *testing.T) {
	t.Run("absent -> nil", func(t *testing.T) {
		tp, err := ServerTrustProxy(rootWith(map[string]any{}))
		require.NoError(t, err)
		assert.Nil(t, tp)
	})
	t.Run("disabled -> nil even with entries", func(t *testing.T) {
		tp, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": false, "proxies": []any{"not-an-ip"},
		}}))
		require.NoError(t, err)
		assert.Nil(t, tp)
	})
	t.Run("enabled full object", func(t *testing.T) {
		tp, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true, "proxies": []any{"10.0.0.0/8", "192.0.2.1"},
			"private": true, "loopback": true, "link_local": true, "header": "X-Real-Ip",
		}}))
		require.NoError(t, err)
		require.NotNil(t, tp)
		assert.Equal(t, []string{"10.0.0.0/8", "192.0.2.1"}, tp.Proxies)
		assert.True(t, tp.Private)
		assert.True(t, tp.Loopback)
		assert.True(t, tp.LinkLocal)
		assert.Equal(t, "X-Real-Ip", tp.Header)
	})
	t.Run("header defaults to X-Forwarded-For", func(t *testing.T) {
		tp, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true, "private": true,
		}}))
		require.NoError(t, err)
		assert.Equal(t, "X-Forwarded-For", tp.Header)
	})
	t.Run("enabled but trusts nothing -> error", func(t *testing.T) {
		_, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true,
		}}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "trusts nothing")
	})
	t.Run("invalid CIDR -> error", func(t *testing.T) {
		_, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true, "proxies": []any{"10.0.0.0/99"},
		}}))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "proxies[0]")
	})
	t.Run("invalid IP -> error", func(t *testing.T) {
		_, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true, "proxies": []any{"caddy"},
		}}))
		require.Error(t, err)
	})
	t.Run("non-string proxies entry -> error", func(t *testing.T) {
		_, err := ServerTrustProxy(rootWith(map[string]any{"trust_proxy": map[string]any{
			"enabled": true, "proxies": []any{float64(7)},
		}}))
		require.Error(t, err)
	})
}

func TestServerOpenAPI_Defaults(t *testing.T) {
	// Block absent -> disabled, defaults materialized.
	s, err := ServerOpenAPI(map[string]any{})
	require.NoError(t, err)
	assert.False(t, s.Enabled)
	assert.True(t, s.Docs)
	assert.Equal(t, "/openapi.json", s.Path)
	assert.Equal(t, "/docs", s.DocsPath)
}

func TestServerOpenAPI_Enabled(t *testing.T) {
	root := map[string]any{"server": map[string]any{"openapi": map[string]any{
		"enabled": true, "docs": false, "path": "/spec.json", "docs_path": "/api-docs",
	}}}
	s, err := ServerOpenAPI(root)
	require.NoError(t, err)
	assert.True(t, s.Enabled)
	assert.False(t, s.Docs)
	assert.Equal(t, "/spec.json", s.Path)
	assert.Equal(t, "/api-docs", s.DocsPath)
}

func TestServerOpenAPI_InvalidPaths(t *testing.T) {
	// Non-absolute path.
	_, err := ServerOpenAPI(map[string]any{"server": map[string]any{"openapi": map[string]any{
		"enabled": true, "path": "openapi.json",
	}}})
	require.Error(t, err)

	// path == docs_path.
	_, err = ServerOpenAPI(map[string]any{"server": map[string]any{"openapi": map[string]any{
		"enabled": true, "path": "/x", "docs_path": "/x",
	}}})
	require.Error(t, err)
}
