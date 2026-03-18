package main

import (
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/wasm"
	"github.com/chimpanze/noda/internal/worker"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- parseShutdownDeadline ---

func TestParseShutdownDeadline_FromConfig(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"server": map[string]any{
				"shutdown_deadline": "15s",
			},
		},
	}
	d := parseShutdownDeadline(rc, 30*time.Second)
	assert.Equal(t, 15*time.Second, d)
}

func TestParseShutdownDeadline_Default(t *testing.T) {
	rc := &config.ResolvedConfig{Root: map[string]any{}}
	d := parseShutdownDeadline(rc, 30*time.Second)
	assert.Equal(t, 30*time.Second, d)
}

func TestParseShutdownDeadline_InvalidDuration(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"server": map[string]any{
				"shutdown_deadline": "not-a-duration",
			},
		},
	}
	d := parseShutdownDeadline(rc, 30*time.Second)
	assert.Equal(t, 30*time.Second, d) // falls back to default
}

func TestParseShutdownDeadline_MissingServerBlock(t *testing.T) {
	rc := &config.ResolvedConfig{Root: map[string]any{"foo": "bar"}}
	d := parseShutdownDeadline(rc, 10*time.Second)
	assert.Equal(t, 10*time.Second, d)
}

// --- parseWasmModuleConfig ---

func TestParseWasmModuleConfig_FullConfig(t *testing.T) {
	raw := map[string]any{
		"module":    "/path/to/module.wasm",
		"tick_rate": float64(60),
		"encoding":  "msgpack",
		"services":  []any{"cache-main", "db-main"},
		"connections": []any{"ws-endpoint"},
		"config":    map[string]any{"key": "value"},
		"memory_pages": float64(256),
		"tick_timeout": "5s",
		"allowed_workflows": []any{"wf-a", "wf-b"},
		"allow_outbound": map[string]any{
			"http": []any{"api.example.com"},
			"ws":   []any{"ws.example.com"},
		},
	}

	cfg := parseWasmModuleConfig("test-module", raw)
	assert.Equal(t, "test-module", cfg.Name)
	assert.Equal(t, "/path/to/module.wasm", cfg.ModulePath)
	assert.Equal(t, 60, cfg.TickRate)
	assert.Equal(t, "msgpack", cfg.Encoding)
	assert.Equal(t, []string{"cache-main", "db-main"}, cfg.Services)
	assert.Equal(t, []string{"ws-endpoint"}, cfg.Connections)
	assert.Equal(t, map[string]any{"key": "value"}, cfg.Config)
	assert.Equal(t, uint32(256), cfg.MemoryPages)
	assert.Equal(t, 5*time.Second, cfg.TickTimeout)
	assert.Equal(t, []string{"wf-a", "wf-b"}, cfg.AllowedWorkflows)
	assert.Equal(t, []string{"api.example.com"}, cfg.AllowHTTP)
	assert.Equal(t, []string{"ws.example.com"}, cfg.AllowWS)
}

func TestParseWasmModuleConfig_MinimalConfig(t *testing.T) {
	raw := map[string]any{
		"module":    "game.wasm",
		"tick_rate": float64(30),
	}
	cfg := parseWasmModuleConfig("game", raw)
	assert.Equal(t, "game", cfg.Name)
	assert.Equal(t, "game.wasm", cfg.ModulePath)
	assert.Equal(t, 30, cfg.TickRate)
	assert.Empty(t, cfg.Services)
	assert.Empty(t, cfg.Connections)
	assert.Empty(t, cfg.AllowedWorkflows)
}

func TestParseWasmModuleConfig_NonMap(t *testing.T) {
	cfg := parseWasmModuleConfig("test", "not-a-map")
	assert.Equal(t, "test", cfg.Name)
	assert.Equal(t, wasm.ModuleConfig{Name: "test"}, cfg)
}

// --- corePlugins ---

func TestCorePlugins_ReturnsPlugins(t *testing.T) {
	plugins := corePlugins()
	assert.NotEmpty(t, plugins)
	// Each plugin should have a non-empty name
	for _, p := range plugins {
		assert.NotEmpty(t, p.Name(), "plugin should have a name")
		assert.NotEmpty(t, p.Prefix(), "plugin should have a prefix")
	}
}

func TestCorePlugins_ContainsExpectedPlugins(t *testing.T) {
	plugins := corePlugins()
	names := make(map[string]bool)
	for _, p := range plugins {
		names[p.Prefix()] = true
	}
	// Verify key prefixes are present
	for _, prefix := range []string{"control", "transform", "util", "workflow", "response", "db", "cache"} {
		assert.True(t, names[prefix], "should have plugin with prefix %q", prefix)
	}
}

// --- buildCoreNodeRegistry ---

func TestBuildCoreNodeRegistry_Succeeds(t *testing.T) {
	reg, err := buildCoreNodeRegistry()
	require.NoError(t, err)
	assert.NotNil(t, reg)

	// Should have common node types registered
	for _, nodeType := range []string{"transform.set", "response.json", "control.if", "util.log"} {
		_, found := reg.GetDescriptor(nodeType)
		assert.True(t, found, "should have node type %q", nodeType)
	}
}

// --- registerCorePlugins ---

func TestRegisterCorePlugins_Succeeds(t *testing.T) {
	plugins := &registry.PluginRegistry{}
	*plugins = *registry.NewPluginRegistry()
	err := registerCorePlugins(plugins)
	require.NoError(t, err)
}

// --- resolveWorkerMiddleware ---

func TestResolveWorkerMiddleware_DefaultWhenEmpty(t *testing.T) {
	mw := resolveWorkerMiddleware(nil, 30*time.Second)
	assert.NotEmpty(t, mw)
}

func TestResolveWorkerMiddleware_DefaultWhenNoMiddleware(t *testing.T) {
	configs := []worker.WorkerConfig{
		{Middleware: nil},
	}
	mw := resolveWorkerMiddleware(configs, 30*time.Second)
	assert.NotEmpty(t, mw)
}
