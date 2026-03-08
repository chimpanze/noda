package registry

import (
	"fmt"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// servicePlugin implements api.Plugin with service support for lifecycle testing.
type servicePlugin struct {
	name        string
	prefix      string
	createFunc  func(map[string]any) (any, error)
	healthFunc  func(any) error
	shutdownLog *[]string
}

func (p *servicePlugin) Name() string                  { return p.name }
func (p *servicePlugin) Prefix() string                { return p.prefix }
func (p *servicePlugin) Nodes() []api.NodeRegistration { return nil }
func (p *servicePlugin) HasServices() bool             { return true }
func (p *servicePlugin) CreateService(config map[string]any) (any, error) {
	if p.createFunc != nil {
		return p.createFunc(config)
	}
	return map[string]any{"created": true}, nil
}
func (p *servicePlugin) HealthCheck(service any) error {
	if p.healthFunc != nil {
		return p.healthFunc(service)
	}
	return nil
}
func (p *servicePlugin) Shutdown(service any) error {
	if p.shutdownLog != nil {
		*p.shutdownLog = append(*p.shutdownLog, p.name)
	}
	return nil
}

func TestInitializeServices_Success(t *testing.T) {
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(&servicePlugin{name: "test-db", prefix: "db"}))

	servicesConfig := map[string]any{
		"main-db": map[string]any{
			"plugin": "db",
			"host":   "localhost",
		},
	}

	registry, errs := InitializeServices(servicesConfig, plugins)
	assert.Empty(t, errs)

	inst, ok := registry.Get("main-db")
	assert.True(t, ok)
	assert.NotNil(t, inst)
}

func TestInitializeServices_UnknownPlugin(t *testing.T) {
	plugins := NewPluginRegistry()

	servicesConfig := map[string]any{
		"main-db": map[string]any{
			"plugin": "nonexistent",
		},
	}

	_, errs := InitializeServices(servicesConfig, plugins)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unknown plugin")
	assert.Contains(t, errs[0].Error(), "nonexistent")
}

func TestInitializeServices_CreateFailure(t *testing.T) {
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(&servicePlugin{
		name:   "bad-db",
		prefix: "db",
		createFunc: func(config map[string]any) (any, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}))

	servicesConfig := map[string]any{
		"main-db": map[string]any{"plugin": "db"},
		"other":   map[string]any{"plugin": "db"},
	}

	_, errs := InitializeServices(servicesConfig, plugins)
	// Both should fail but both are attempted
	assert.Len(t, errs, 2)
}

func TestInitializeServices_MissingPluginField(t *testing.T) {
	plugins := NewPluginRegistry()

	servicesConfig := map[string]any{
		"main-db": map[string]any{
			"host": "localhost",
		},
	}

	_, errs := InitializeServices(servicesConfig, plugins)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "plugin")
}

func TestHealthCheckAll_Healthy(t *testing.T) {
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(&servicePlugin{name: "db", prefix: "db"}))

	servicesConfig := map[string]any{
		"main-db": map[string]any{"plugin": "db"},
	}

	registry, errs := InitializeServices(servicesConfig, plugins)
	require.Empty(t, errs)

	healthErrs := HealthCheckAll(registry)
	assert.Empty(t, healthErrs)
}

func TestHealthCheckAll_Unhealthy(t *testing.T) {
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(&servicePlugin{
		name:   "db",
		prefix: "db",
		healthFunc: func(service any) error {
			return fmt.Errorf("connection lost")
		},
	}))

	servicesConfig := map[string]any{
		"main-db": map[string]any{"plugin": "db"},
	}

	registry, errs := InitializeServices(servicesConfig, plugins)
	require.Empty(t, errs)

	healthErrs := HealthCheckAll(registry)
	require.Len(t, healthErrs, 1)
	assert.Contains(t, healthErrs[0].Error(), "connection lost")
}

func TestShutdownAll_ReverseOrder(t *testing.T) {
	var shutdownLog []string

	plugins := NewPluginRegistry()
	p1 := &servicePlugin{name: "first", prefix: "db", shutdownLog: &shutdownLog}
	p2 := &servicePlugin{name: "second", prefix: "cache", shutdownLog: &shutdownLog}
	require.NoError(t, plugins.Register(p1))
	require.NoError(t, plugins.Register(p2))

	registry := NewServiceRegistry()
	require.NoError(t, registry.Register("svc-first", "inst1", p1))
	require.NoError(t, registry.Register("svc-second", "inst2", p2))

	errs := ShutdownAll(registry)
	assert.Empty(t, errs)
	assert.Equal(t, []string{"second", "first"}, shutdownLog)
}
