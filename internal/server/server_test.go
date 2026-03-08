package server

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
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
	err = srv.Stop()
	assert.NoError(t, err)
}
