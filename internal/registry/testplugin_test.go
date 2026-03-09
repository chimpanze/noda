package registry

import (
	"context"
	"fmt"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKVPlugin is a full-lifecycle test plugin implementing an in-memory key-value store.
type testKVPlugin struct{}

func (p *testKVPlugin) Name() string      { return "test-kv" }
func (p *testKVPlugin) Prefix() string    { return "kv" }
func (p *testKVPlugin) HasServices() bool { return true }

func (p *testKVPlugin) CreateService(cfg map[string]any) (any, error) {
	return make(map[string]any), nil
}

func (p *testKVPlugin) HealthCheck(service any) error {
	if _, ok := service.(map[string]any); !ok {
		return fmt.Errorf("kv service is not a map")
	}
	return nil
}

func (p *testKVPlugin) Shutdown(service any) error {
	if store, ok := service.(map[string]any); ok {
		for k := range store {
			delete(store, k)
		}
	}
	return nil
}

func (p *testKVPlugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{
			Descriptor: &kvGetDescriptor{},
			Factory: func(cfg map[string]any) api.NodeExecutor {
				return &kvGetExecutor{}
			},
		},
		{
			Descriptor: &kvSetDescriptor{},
			Factory: func(cfg map[string]any) api.NodeExecutor {
				return &kvSetExecutor{}
			},
		},
	}
}

// kv.get descriptor and executor
type kvGetDescriptor struct{}

func (d *kvGetDescriptor) Name() string { return "get" }
func (d *kvGetDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"store": {Prefix: "kv", Required: true},
	}
}
func (d *kvGetDescriptor) ConfigSchema() map[string]any { return nil }

type kvGetExecutor struct{}

func (e *kvGetExecutor) Outputs() []string { return []string{"ok", "not_found"} }
func (e *kvGetExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, cfg map[string]any, services map[string]any) (string, any, error) {
	store, ok := services["store"].(map[string]any)
	if !ok {
		return "", nil, fmt.Errorf("kv.get: store service not available")
	}
	key, _ := cfg["key"].(string)
	val, found := store[key]
	if !found {
		return "not_found", nil, nil
	}
	return "ok", val, nil
}

// kv.set descriptor and executor
type kvSetDescriptor struct{}

func (d *kvSetDescriptor) Name() string { return "set" }
func (d *kvSetDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"store": {Prefix: "kv", Required: true},
	}
}
func (d *kvSetDescriptor) ConfigSchema() map[string]any { return nil }

type kvSetExecutor struct{}

func (e *kvSetExecutor) Outputs() []string { return []string{"ok"} }
func (e *kvSetExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, cfg map[string]any, services map[string]any) (string, any, error) {
	store, ok := services["store"].(map[string]any)
	if !ok {
		return "", nil, fmt.Errorf("kv.set: store service not available")
	}
	key, _ := cfg["key"].(string)
	store[key] = cfg["value"]
	return "ok", nil, nil
}

// TestKVPlugin_FullLifecycle exercises: register → create service → register nodes → validate → health check → execute → shutdown
func TestKVPlugin_FullLifecycle(t *testing.T) {
	plugin := &testKVPlugin{}

	// 1. Register plugin
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(plugin))

	// 2. Create service from config
	servicesConfig := map[string]any{
		"my-store": map[string]any{
			"plugin": "kv",
		},
	}
	services, errs := InitializeServices(servicesConfig, plugins)
	require.Empty(t, errs)

	// 3. Register nodes
	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(plugin))

	// Verify nodes registered
	assert.Contains(t, nodes.AllTypes(), "kv.get")
	assert.Contains(t, nodes.AllTypes(), "kv.set")

	// 4. Validate startup
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"test-wf": {
				"nodes": map[string]any{
					"write": map[string]any{
						"type":     "kv.set",
						"services": map[string]any{"store": "my-store"},
					},
					"read": map[string]any{
						"type":     "kv.get",
						"services": map[string]any{"store": "my-store"},
					},
				},
			},
		},
	}
	valErrs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions())
	assert.Empty(t, valErrs)

	// 5. Health check
	healthErrs := services.HealthCheckAll()
	assert.Empty(t, healthErrs)

	// 6. Execute nodes
	store, ok := services.Get("my-store")
	require.True(t, ok)

	svcMap := map[string]any{"store": store}

	// Set a value
	setFactory, ok := nodes.GetFactory("kv.set")
	require.True(t, ok)
	setExec := setFactory(nil)
	output, _, err := setExec.Execute(context.Background(), nil, map[string]any{"key": "greeting", "value": "hello"}, svcMap)
	require.NoError(t, err)
	assert.Equal(t, "ok", output)

	// Get the value
	getFactory, ok := nodes.GetFactory("kv.get")
	require.True(t, ok)
	getExec := getFactory(nil)
	output, data, err := getExec.Execute(context.Background(), nil, map[string]any{"key": "greeting"}, svcMap)
	require.NoError(t, err)
	assert.Equal(t, "ok", output)
	assert.Equal(t, "hello", data)

	// Get missing key
	output, _, err = getExec.Execute(context.Background(), nil, map[string]any{"key": "missing"}, svcMap)
	require.NoError(t, err)
	assert.Equal(t, "not_found", output)

	// 7. Shutdown
	shutdownErrs := services.ShutdownAll()
	assert.Empty(t, shutdownErrs)

	// Verify store was cleared
	assert.Empty(t, store.(map[string]any))
}
