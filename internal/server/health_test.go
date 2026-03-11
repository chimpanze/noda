package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealth_Live(t *testing.T) {
	srv := setupHealthServer(t)
	resp, err := srv.App().Test(mustReq(http.MethodGet, "/health/live"))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body := readJSON(t, resp)
	assert.Equal(t, "ok", body["status"])
}

func TestHealth_Ready_NotReady(t *testing.T) {
	readyFlag.Store(false)
	srv := setupHealthServer(t)
	resp, err := srv.App().Test(mustReq(http.MethodGet, "/health/ready"))
	require.NoError(t, err)
	assert.Equal(t, 503, resp.StatusCode)
	body := readJSON(t, resp)
	assert.Equal(t, "not_ready", body["status"])
}

func TestHealth_Ready_AfterSetReady(t *testing.T) {
	readyFlag.Store(false)
	srv := setupHealthServer(t)
	SetReady()
	resp, err := srv.App().Test(mustReq(http.MethodGet, "/health/ready"))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body := readJSON(t, resp)
	assert.Equal(t, "ready", body["status"])
}

type healthyService struct{}

func (s *healthyService) Ping() error { return nil }

type unhealthyService struct{}

func (s *unhealthyService) Ping() error { return fmt.Errorf("connection refused") }

func TestHealth_AllServicesHealthy(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	p := &mockPlugin{}
	_ = svcReg.Register("main-db", &healthyService{}, p)
	_ = svcReg.Register("app-cache", &healthyService{}, p)

	srv := setupHealthServerWithServices(t, svcReg)
	resp, err := srv.App().Test(mustReq(http.MethodGet, "/health"))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body := readJSON(t, resp)
	assert.Equal(t, "healthy", body["status"])
	services := body["services"].(map[string]any)
	assert.Equal(t, "ok", services["main-db"])
	assert.Equal(t, "ok", services["app-cache"])
}

func TestHealth_OneUnhealthyService(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	p := &mockPlugin{}
	_ = svcReg.Register("main-db", &healthyService{}, p)
	_ = svcReg.Register("app-cache", &unhealthyService{}, p)

	srv := setupHealthServerWithServices(t, svcReg)
	resp, err := srv.App().Test(mustReq(http.MethodGet, "/health"))
	require.NoError(t, err)
	assert.Equal(t, 503, resp.StatusCode)

	body := readJSON(t, resp)
	assert.Equal(t, "unhealthy", body["status"])
	services := body["services"].(map[string]any)
	assert.Equal(t, "ok", services["main-db"])
	assert.Contains(t, services["app-cache"], "unhealthy")
}

func TestHealth_ServiceWithoutPing(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	p := &mockPlugin{}
	_ = svcReg.Register("simple-svc", "just-a-string", p)

	srv := setupHealthServerWithServices(t, svcReg)
	resp, err := srv.App().Test(mustReq(http.MethodGet, "/health"))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body := readJSON(t, resp)
	assert.Equal(t, "healthy", body["status"])
	services := body["services"].(map[string]any)
	assert.Equal(t, "ok", services["simple-svc"])
}

// --- helpers ---

type mockPlugin struct{}

func (p *mockPlugin) Name() string                              { return "mock" }
func (p *mockPlugin) Prefix() string                            { return "mock" }
func (p *mockPlugin) Nodes() []api.NodeRegistration             { return nil }
func (p *mockPlugin) HasServices() bool                         { return false }
func (p *mockPlugin) CreateService(map[string]any) (any, error) { return nil, nil }
func (p *mockPlugin) HealthCheck(any) error                     { return nil }
func (p *mockPlugin) Shutdown(any) error                        { return nil }

func setupHealthServer(t *testing.T) *Server {
	t.Helper()
	return setupHealthServerWithServices(t, registry.NewServiceRegistry())
}

func setupHealthServerWithServices(t *testing.T, svcReg *registry.ServiceRegistry) *Server {
	t.Helper()
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
	}
	srv, err := NewServer(rc, svcReg, registry.NewNodeRegistry())
	require.NoError(t, err)
	srv.registerHealthRoutes()
	return srv
}

func mustReq(method, url string) *http.Request {
	req, _ := http.NewRequest(method, url, nil)
	return req
}

func readJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(data, &result))
	return result
}
