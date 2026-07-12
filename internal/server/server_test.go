package server

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testConfig() *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Root:      map[string]any{},
		Routes:    map[string]map[string]any{},
		Workflows: map[string]map[string]any{},
		Schemas:   map[string]map[string]any{},
	}
}

func TestNewServer_Defaults(t *testing.T) {
	rc := testConfig()
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	assert.Equal(t, 3000, srv.Port())
	assert.NotNil(t, srv.App())
}

func TestNewServer_CustomPort(t *testing.T) {
	rc := testConfig()
	rc.Root["server"] = map[string]any{
		"port": float64(8080),
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	assert.Equal(t, 8080, srv.Port())
}

func TestNewServer_Timeouts(t *testing.T) {
	rc := testConfig()
	rc.Root["server"] = map[string]any{
		"read_timeout":  "30s",
		"write_timeout": "60s",
		"body_limit":    float64(1048576),
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	assert.NotNil(t, srv.App())
}

func TestServer_StopWithoutStart(t *testing.T) {
	rc := testConfig()
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	// Stop without starting should not panic
	err = srv.Stop(context.Background())
	assert.NoError(t, err)
}

func TestNewServer_BodyLimitFromEnvString(t *testing.T) {
	rc := testConfig()
	rc.Root["server"] = map[string]any{"body_limit": "1073741824"}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	assert.Equal(t, 1073741824, srv.App().Config().BodyLimit)
}

func TestNewServer_PortFromEnvString(t *testing.T) {
	rc := testConfig()
	rc.Root["server"] = map[string]any{"port": "8080"}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	assert.Equal(t, 8080, srv.Port())
}

func TestNewServer_InvalidScalarsFailLoudly(t *testing.T) {
	cases := []struct {
		name   string
		server map[string]any
		want   string
	}{
		{"garbage body_limit", map[string]any{"body_limit": "10GB"}, "body_limit"},
		{"garbage port", map[string]any{"port": "http"}, "port"},
		{"bad read_timeout", map[string]any{"read_timeout": "banana"}, "read_timeout"},
		{"bad write_timeout", map[string]any{"write_timeout": "fast"}, "write_timeout"},
		{"trust_proxy trusts nothing", map[string]any{"trust_proxy": map[string]any{"enabled": true}}, "trusts nothing"},
		{"trust_proxy bad entry", map[string]any{"trust_proxy": map[string]any{"enabled": true, "proxies": []any{"10.0.0.0/99"}}}, "proxies[0]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rc := testConfig()
			rc.Root["server"] = tc.server
			_, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestNewServer_TrustProxyOffByDefault(t *testing.T) {
	rc := testConfig()
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	assert.False(t, srv.App().Config().TrustProxy)
	assert.Empty(t, srv.App().Config().ProxyHeader)
}

func TestNewServer_TrustProxyMapping(t *testing.T) {
	rc := testConfig()
	rc.Root["server"] = map[string]any{"trust_proxy": map[string]any{
		"enabled": true,
		"proxies": []any{"10.0.0.0/8"},
		"private": true,
		"header":  "X-Real-Ip",
	}}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	cfg := srv.App().Config()
	assert.True(t, cfg.TrustProxy)
	assert.Equal(t, "X-Real-Ip", cfg.ProxyHeader)
	assert.Equal(t, []string{"10.0.0.0/8"}, cfg.TrustProxyConfig.Proxies)
	assert.True(t, cfg.TrustProxyConfig.Private)
	assert.False(t, cfg.TrustProxyConfig.Loopback)
}

// app.Test connections arrive from 0.0.0.0 (fiber v3.1.0 testConn), so
// "0.0.0.0/0" makes the test client a trusted proxy and "198.51.100.0/24"
// makes it untrusted.
func trustProxyTestServer(t *testing.T, proxies []any) *Server {
	t.Helper()
	rc := testConfig()
	rc.Root["server"] = map[string]any{"trust_proxy": map[string]any{
		"enabled": true, "proxies": proxies,
	}}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.NoError(t, err)
	srv.App().Get("/ip", func(c fiber.Ctx) error { return c.SendString(c.IP()) })
	return srv
}

func TestServer_TrustedProxy_IPFromForwardedHeader(t *testing.T) {
	srv := trustProxyTestServer(t, []any{"0.0.0.0/0"})
	req := httptest.NewRequest("GET", "/ip", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "203.0.113.7", string(body))
}

// Polarity check: this test MUST fail against a build that blindly trusts the
// header (e.g. ProxyHeader set without TrustProxy filtering).
func TestServer_UntrustedProxy_ForwardedHeaderIgnored(t *testing.T) {
	srv := trustProxyTestServer(t, []any{"198.51.100.0/24"})
	req := httptest.NewRequest("GET", "/ip", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	assert.NotEqual(t, "203.0.113.7", string(body))
	assert.Equal(t, "0.0.0.0", string(body)) // socket remote IP, not the spoofed header
}

func TestServer_LimiterKeysOnForwardedIP(t *testing.T) {
	srv := trustProxyTestServer(t, []any{"0.0.0.0/0"})
	// Check parseLimiterConfig (middleware.go:245) for the exact config keys
	// if this construction fails; "max" is read as float64 at :250.
	h, err := newLimiterMiddleware(map[string]any{"max": float64(1), "expiration": "1m"}, nil)
	require.NoError(t, err)
	srv.App().Use(h)
	srv.App().Get("/limited", func(c fiber.Ctx) error { return c.SendString("ok") })

	do := func(xff string) int {
		req := httptest.NewRequest("GET", "/limited", nil)
		req.Header.Set("X-Forwarded-For", xff)
		resp, err := srv.App().Test(req)
		require.NoError(t, err)
		return resp.StatusCode
	}
	assert.Equal(t, 200, do("203.0.113.7")) // client A, first hit
	assert.Equal(t, 200, do("203.0.113.8")) // client B gets own bucket
	assert.Equal(t, 429, do("203.0.113.7")) // client A exhausted its bucket
}
