package registry

import (
	"fmt"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Validator edge cases
// ---------------------------------------------------------------------------

func TestValidateStartup_UnknownNodeType_KnownPrefix(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query"},
			Factory:    func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))
	services := NewServiceRegistry()

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step1": map[string]any{
						"type": "db.nonexistent", // prefix known, node type unknown
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unknown node type")
	assert.Contains(t, errs[0].Error(), "db.nonexistent")
}

func TestValidateStartup_EmptyNodeType(t *testing.T) {
	plugins, services, nodes := setupValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step1": map[string]any{
						"type": "", // empty type should be skipped
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartup_NonMapNode(t *testing.T) {
	plugins, services, nodes := setupValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step1": "not-a-map", // non-map node should be skipped
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartup_NonMapNodes(t *testing.T) {
	plugins, services, nodes := setupValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": "not-a-map", // non-map nodes section
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartup_DeferredServiceResolution(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-ws", "ws", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "send",
				deps: map[string]api.ServiceDep{
					"connection": {Prefix: "ws", Required: true},
				},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))
	services := NewServiceRegistry()

	deferred := map[string]DeferredService{
		"live-ws": {Prefix: "ws"},
	}

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"send": map[string]any{
						"type":     "ws.send",
						"services": map[string]any{"connection": "live-ws"},
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), deferred)
	assert.Empty(t, errs)
}

func TestValidateStartup_DeferredServiceWrongPrefix(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "query",
				deps: map[string]api.ServiceDep{
					"database": {Prefix: "db", Required: true},
				},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))
	services := NewServiceRegistry()

	deferred := map[string]DeferredService{
		"live-ws": {Prefix: "ws"},
	}

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type":     "db.query",
						"services": map[string]any{"database": "live-ws"},
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), deferred)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "prefix")
}

func TestValidateStartup_InvalidExpression(t *testing.T) {
	plugins, services, nodes := setupValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type": "db.query",
						"services": map[string]any{
							"database": "main-db",
						},
						"config": map[string]any{
							"query": "{{ %%% }}",
						},
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.NotEmpty(t, errs)
}

// ---------------------------------------------------------------------------
// ValidateStartupDryRun
// ---------------------------------------------------------------------------

func TestValidateStartupDryRun_ValidConfig(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "query",
				deps: map[string]api.ServiceDep{
					"database": {Prefix: "db", Required: true},
				},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"main-db": map[string]any{
					"plugin": "db",
				},
			},
		},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type":     "db.query",
						"services": map[string]any{"database": "main-db"},
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_UnknownPrefix(t *testing.T) {
	plugins := NewPluginRegistry()
	nodes := NewNodeRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step1": map[string]any{
						"type": "unknown.node",
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unknown node type prefix")
}

func TestValidateStartupDryRun_UnknownNodeType(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query"},
			Factory:    func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step1": map[string]any{
						"type": "db.nonexistent",
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unknown node type")
}

func TestValidateStartupDryRun_MissingRequiredSlot(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "query",
				deps: map[string]api.ServiceDep{
					"database": {Prefix: "db", Required: true},
				},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))
	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type":     "db.query",
						"services": map[string]any{},
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "missing required service slot")
}

func TestValidateStartupDryRun_ServiceNotInConfig(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "query",
				deps: map[string]api.ServiceDep{
					"database": {Prefix: "db", Required: true},
				},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))
	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type":     "db.query",
						"services": map[string]any{"database": "nonexistent-db"},
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "not found in config")
}

func TestValidateStartupDryRun_DeferredServiceAccepted(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-ws", "ws", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "send",
				deps: map[string]api.ServiceDep{
					"connection": {Prefix: "ws", Required: true},
				},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))
	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))

	deferred := map[string]DeferredService{
		"live-ws": {Prefix: "ws"},
	}

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"send": map[string]any{
						"type":     "ws.send",
						"services": map[string]any{"connection": "live-ws"},
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), deferred)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_NonMapNodes(t *testing.T) {
	plugins := NewPluginRegistry()
	nodes := NewNodeRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": "not-a-map",
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_NonMapNode(t *testing.T) {
	plugins := NewPluginRegistry()
	nodes := NewNodeRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step1": "not-a-map",
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_EmptyNodeType(t *testing.T) {
	plugins := NewPluginRegistry()
	nodes := NewNodeRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step1": map[string]any{
						"type": "",
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_InvalidExpression(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query"},
			Factory:    func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))
	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step1": map[string]any{
						"type": "db.query",
						"config": map[string]any{
							"query": "{{ %%% }}",
						},
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.NotEmpty(t, errs)
}

func TestValidateStartupDryRun_OptionalSlotUnfilled(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "query",
				deps: map[string]api.ServiceDep{
					"cache": {Prefix: "cache", Required: false},
				},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))
	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type": "db.query",
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

// ---------------------------------------------------------------------------
// Bootstrap edge cases
// ---------------------------------------------------------------------------

func TestBootstrap_DryRunMode(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query", deps: map[string]api.ServiceDep{}},
			Factory:    func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))

	rc := &config.ResolvedConfig{
		Root:        map[string]any{},
		Workflows:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
	}

	result, errs := Bootstrap(rc, plugins, BootstrapOptions{DryRun: true})
	assert.Empty(t, errs)
	assert.NotNil(t, result)
	assert.Contains(t, result.Nodes.AllTypes(), "db.query")
}

func TestBootstrap_DryRunValidationError(t *testing.T) {
	plugins := NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step": map[string]any{"type": "unknown.node"},
				},
			},
		},
		Connections: map[string]map[string]any{},
	}

	_, errs := Bootstrap(rc, plugins, BootstrapOptions{DryRun: true})
	require.NotEmpty(t, errs)
}

func TestBootstrap_ServiceInitFromConfig(t *testing.T) {
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(&servicePlugin{name: "test-db", prefix: "db"}))

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"main-db": map[string]any{
					"plugin": "db",
				},
			},
		},
		Workflows:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
	}

	result, errs := Bootstrap(rc, plugins)
	assert.Empty(t, errs)
	assert.NotNil(t, result)
	_, ok := result.Services.Get("main-db")
	assert.True(t, ok)
}

func TestBootstrap_ServiceInitError(t *testing.T) {
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(&servicePlugin{
		name:   "bad-db",
		prefix: "db",
		createFunc: func(config map[string]any) (any, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}))

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"main-db": map[string]any{
					"plugin": "db",
				},
			},
		},
		Workflows:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
	}

	_, errs := Bootstrap(rc, plugins)
	require.NotEmpty(t, errs)
}

func TestBootstrap_DeferredServiceErrors(t *testing.T) {
	plugins := NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Workflows: map[string]map[string]any{},
		Connections: map[string]map[string]any{
			"connections/bad.json": {
				"endpoints": map[string]any{
					"bad-ep": map[string]any{"type": "unknown-type"},
				},
			},
		},
	}

	_, errs := Bootstrap(rc, plugins)
	require.NotEmpty(t, errs)
}

// ---------------------------------------------------------------------------
// Lifecycle: shutdown error handling
// ---------------------------------------------------------------------------

func TestShutdownAll_WithErrors(t *testing.T) {
	plugins := NewPluginRegistry()
	p := &servicePlugin{
		name:   "failing-db",
		prefix: "db",
		createFunc: func(config map[string]any) (any, error) {
			return "inst", nil
		},
	}
	require.NoError(t, plugins.Register(p))

	// Create a plugin that fails on shutdown
	failPlugin := &failingShutdownPlugin{name: "fail-svc", prefix: "fail"}

	registry := NewServiceRegistry()
	require.NoError(t, registry.Register("svc-a", "inst-a", failPlugin))
	require.NoError(t, registry.Register("svc-b", "inst-b", failPlugin))

	errs := registry.ShutdownAll(t.Context())
	require.Len(t, errs, 2)
	for _, err := range errs {
		assert.Contains(t, err.Error(), "shutdown failed")
	}
}

// failingShutdownPlugin is a plugin whose Shutdown always returns an error.
type failingShutdownPlugin struct {
	name   string
	prefix string
}

func (p *failingShutdownPlugin) Name() string                                     { return p.name }
func (p *failingShutdownPlugin) Prefix() string                                   { return p.prefix }
func (p *failingShutdownPlugin) Nodes() []api.NodeRegistration                    { return nil }
func (p *failingShutdownPlugin) HasServices() bool                                { return true }
func (p *failingShutdownPlugin) CreateService(config map[string]any) (any, error) { return "inst", nil }
func (p *failingShutdownPlugin) HealthCheck(service any) error                    { return nil }
func (p *failingShutdownPlugin) Shutdown(service any) error {
	return fmt.Errorf("shutdown error for %s", p.name)
}

func TestShutdownAll_EmptyRegistry(t *testing.T) {
	registry := NewServiceRegistry()
	errs := registry.ShutdownAll(t.Context())
	assert.Empty(t, errs)
}

func TestHealthCheckAll_EmptyRegistry(t *testing.T) {
	registry := NewServiceRegistry()
	errs := registry.HealthCheckAll()
	assert.Empty(t, errs)
}

// ---------------------------------------------------------------------------
// InitializeServices edge cases
// ---------------------------------------------------------------------------

func TestInitializeServices_NonMapConfig(t *testing.T) {
	plugins := NewPluginRegistry()

	servicesConfig := map[string]any{
		"bad-svc": "not-a-map",
	}

	_, errs := InitializeServices(servicesConfig, plugins)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "config must be a map")
}

func TestInitializeServices_PluginWithoutServices(t *testing.T) {
	plugins := NewPluginRegistry()
	// Register a node-only plugin (HasServices=false)
	require.NoError(t, plugins.Register(&stubPlugin{name: "control", prefix: "control"}))

	servicesConfig := map[string]any{
		"bad": map[string]any{
			"plugin": "control",
		},
	}

	_, errs := InitializeServices(servicesConfig, plugins)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "does not support services")
}

func TestInitializeServices_InnerConfigExtraction(t *testing.T) {
	plugins := NewPluginRegistry()
	var receivedConfig map[string]any
	require.NoError(t, plugins.Register(&servicePlugin{
		name:   "test-db",
		prefix: "db",
		createFunc: func(config map[string]any) (any, error) {
			receivedConfig = config
			return "inst", nil
		},
	}))

	servicesConfig := map[string]any{
		"main-db": map[string]any{
			"plugin": "db",
			"config": map[string]any{
				"host": "localhost",
				"port": 5432,
			},
		},
	}

	_, errs := InitializeServices(servicesConfig, plugins)
	assert.Empty(t, errs)
	assert.Equal(t, "localhost", receivedConfig["host"])
	assert.Equal(t, 5432, receivedConfig["port"])
}

// ---------------------------------------------------------------------------
// Node registry: OutputsForType, OutputsForTypeWithConfig, CountByPrefix, RegisterFactory
// ---------------------------------------------------------------------------

func TestNodeRegistry_OutputsForType(t *testing.T) {
	reg := NewNodeRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query"},
			Factory: func(map[string]any) api.NodeExecutor {
				return &stubExecutor{outputs: []string{"ok", "error"}}
			},
		},
	})
	require.NoError(t, reg.RegisterFromPlugin(p))

	outputs, ok := reg.OutputsForType("db.query")
	assert.True(t, ok)
	assert.Equal(t, []string{"ok", "error"}, outputs)
}

func TestNodeRegistry_OutputsForType_NotFound(t *testing.T) {
	reg := NewNodeRegistry()
	_, ok := reg.OutputsForType("nonexistent.type")
	assert.False(t, ok)
}

func TestNodeRegistry_OutputsForTypeWithConfig(t *testing.T) {
	reg := NewNodeRegistry()
	p := pluginWithNodes("test-ctrl", "control", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "switch"},
			Factory: func(cfg map[string]any) api.NodeExecutor {
				// Dynamic outputs based on config
				outputs := []string{"default"}
				if cases, ok := cfg["cases"].([]string); ok {
					outputs = append(outputs, cases...)
				}
				return &stubExecutor{outputs: outputs}
			},
		},
	})
	require.NoError(t, reg.RegisterFromPlugin(p))

	outputs, ok := reg.OutputsForTypeWithConfig("control.switch", map[string]any{
		"cases": []string{"case_a", "case_b"},
	})
	assert.True(t, ok)
	assert.Equal(t, []string{"default", "case_a", "case_b"}, outputs)
}

func TestNodeRegistry_OutputsForTypeWithConfig_NotFound(t *testing.T) {
	reg := NewNodeRegistry()
	_, ok := reg.OutputsForTypeWithConfig("nonexistent.type", nil)
	assert.False(t, ok)
}

func TestNodeRegistry_CountByPrefix(t *testing.T) {
	reg := NewNodeRegistry()
	require.NoError(t, reg.RegisterFromPlugin(pluginWithNodes("db", "db", []api.NodeRegistration{
		{Descriptor: &stubDescriptor{name: "query"}, Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} }},
		{Descriptor: &stubDescriptor{name: "exec"}, Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} }},
	})))

	assert.Equal(t, 2, reg.CountByPrefix("db"))
	assert.Equal(t, 0, reg.CountByPrefix("nonexistent"))
}

func TestNodeRegistry_RegisterFactory(t *testing.T) {
	reg := NewNodeRegistry()
	called := false
	reg.RegisterFactory("mock.node", func(config map[string]any) api.NodeExecutor {
		called = true
		return &stubExecutor{outputs: []string{"ok"}}
	})

	factory, ok := reg.GetFactory("mock.node")
	require.True(t, ok)

	executor := factory(nil)
	assert.True(t, called)
	assert.Equal(t, []string{"ok"}, executor.Outputs())
}

// ---------------------------------------------------------------------------
// Plugin registry: GetByName, composite plugin
// ---------------------------------------------------------------------------

func TestPluginRegistry_GetByName(t *testing.T) {
	reg := NewPluginRegistry()
	require.NoError(t, reg.Register(&stubPlugin{name: "postgres", prefix: "db"}))

	p, ok := reg.GetByName("postgres")
	assert.True(t, ok)
	assert.Equal(t, "postgres", p.Name())
}

func TestPluginRegistry_GetByName_FallbackToPrefix(t *testing.T) {
	reg := NewPluginRegistry()
	require.NoError(t, reg.Register(&stubPlugin{name: "db", prefix: "db"}))

	// Name equals prefix, should work via fallback
	p, ok := reg.GetByName("db")
	assert.True(t, ok)
	assert.Equal(t, "db", p.Name())
}

func TestPluginRegistry_GetByName_NotFound(t *testing.T) {
	reg := NewPluginRegistry()
	_, ok := reg.GetByName("nonexistent")
	assert.False(t, ok)
}

func TestPluginRegistry_CompositePlugin_NodesThenServices(t *testing.T) {
	reg := NewPluginRegistry()

	// Register node-only plugin first
	nodePlugin := pluginWithNodes("db-nodes", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query"},
			Factory:    func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, reg.Register(nodePlugin))

	// Register service-only plugin second (same prefix)
	svcPlugin := &servicePlugin{name: "db-svc", prefix: "db"}
	require.NoError(t, reg.Register(svcPlugin))

	// Should be merged
	p, ok := reg.Get("db")
	require.True(t, ok)
	assert.True(t, p.HasServices())
	assert.Len(t, p.Nodes(), 1)
	assert.Equal(t, "db-nodes+db-svc", p.Name())
	assert.Equal(t, "db", p.Prefix())
}

func TestPluginRegistry_CompositePlugin_ServicesThenNodes(t *testing.T) {
	reg := NewPluginRegistry()

	// Register service-only plugin first
	svcPlugin := &servicePlugin{name: "db-svc", prefix: "db"}
	require.NoError(t, reg.Register(svcPlugin))

	// Register node-only plugin second (same prefix)
	np := pluginWithNodes("db-nodes", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query"},
			Factory:    func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, reg.Register(np))

	p, ok := reg.Get("db")
	require.True(t, ok)
	assert.True(t, p.HasServices())
	assert.Len(t, p.Nodes(), 1)
}

func TestPluginRegistry_CompositePlugin_HealthCheckAndShutdown(t *testing.T) {
	reg := NewPluginRegistry()

	np := pluginWithNodes("db-nodes", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query"},
			Factory:    func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, reg.Register(np))

	svcPlugin := &servicePlugin{name: "db-svc", prefix: "db"}
	require.NoError(t, reg.Register(svcPlugin))

	p, ok := reg.Get("db")
	require.True(t, ok)

	// Test composite plugin service methods
	inst, err := p.CreateService(map[string]any{})
	assert.NoError(t, err)
	assert.NotNil(t, inst)

	assert.NoError(t, p.HealthCheck(inst))
	assert.NoError(t, p.Shutdown(inst))
}

func TestPluginRegistry_CompositePlugin_DuplicateBothHaveNodes(t *testing.T) {
	reg := NewPluginRegistry()

	p1 := pluginWithNodes("db1", "db", []api.NodeRegistration{
		{Descriptor: &stubDescriptor{name: "query"}, Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} }},
	})
	p2 := pluginWithNodes("db2", "db", []api.NodeRegistration{
		{Descriptor: &stubDescriptor{name: "exec"}, Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} }},
	})

	require.NoError(t, reg.Register(p1))
	err := reg.Register(p2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate plugin prefix")
}

// ---------------------------------------------------------------------------
// Service registry: GetWithPlugin non-existent, Order
// ---------------------------------------------------------------------------

func TestServiceRegistry_GetWithPlugin_NotFound(t *testing.T) {
	reg := NewServiceRegistry()
	inst, p, ok := reg.GetWithPlugin("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, inst)
	assert.Nil(t, p)
}

func TestServiceRegistry_Order(t *testing.T) {
	reg := NewServiceRegistry()
	plugin := &stubPlugin{name: "db", prefix: "db"}

	require.NoError(t, reg.Register("first", "inst1", plugin))
	require.NoError(t, reg.Register("second", "inst2", plugin))
	require.NoError(t, reg.Register("third", "inst3", plugin))

	order := reg.Order()
	assert.Equal(t, []string{"first", "second", "third"}, order)
}

// ---------------------------------------------------------------------------
// extractPrefix edge case
// ---------------------------------------------------------------------------

func TestExtractPrefix_NoDot(t *testing.T) {
	result := extractPrefix("nodot")
	assert.Equal(t, "nodot", result)
}

func TestExtractPrefix_MultipleDots(t *testing.T) {
	result := extractPrefix("a.b.c")
	assert.Equal(t, "a", result)
}

// ---------------------------------------------------------------------------
// CollectDeferredServices edge cases
// ---------------------------------------------------------------------------

func TestCollectDeferredServices_NonMapEndpoint(t *testing.T) {
	rc := &config.ResolvedConfig{
		Connections: map[string]map[string]any{
			"connections/bad.json": {
				"endpoints": map[string]any{
					"bad": "not-a-map",
				},
			},
		},
	}

	deferred, errs := CollectDeferredServices(rc)
	assert.Empty(t, errs)
	assert.Empty(t, deferred)
}

func TestCollectDeferredServices_NonMapEndpoints(t *testing.T) {
	rc := &config.ResolvedConfig{
		Connections: map[string]map[string]any{
			"connections/bad.json": {
				"endpoints": "not-a-map",
			},
		},
	}

	deferred, errs := CollectDeferredServices(rc)
	assert.Empty(t, errs)
	assert.Empty(t, deferred)
}

func TestCollectDeferredServices_EmptyConfig(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root:        map[string]any{},
		Connections: map[string]map[string]any{},
	}

	deferred, errs := CollectDeferredServices(rc)
	assert.Empty(t, errs)
	assert.Empty(t, deferred)
}
