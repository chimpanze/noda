package registry

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
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
func (p *servicePlugin) ServiceConfigSchema() map[string]any { return nil }
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

	registry, errs := InitializeServices(context.Background(), servicesConfig, plugins, 0)
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

	_, errs := InitializeServices(context.Background(), servicesConfig, plugins, 0)
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

	_, errs := InitializeServices(context.Background(), servicesConfig, plugins, 0)
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

	_, errs := InitializeServices(context.Background(), servicesConfig, plugins, 0)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "plugin")
}

func TestHealthCheckAll_Healthy(t *testing.T) {
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(&servicePlugin{name: "db", prefix: "db"}))

	servicesConfig := map[string]any{
		"main-db": map[string]any{"plugin": "db"},
	}

	registry, errs := InitializeServices(context.Background(), servicesConfig, plugins, 0)
	require.Empty(t, errs)

	healthErrs := registry.HealthCheckAll()
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

	registry, errs := InitializeServices(context.Background(), servicesConfig, plugins, 0)
	require.Empty(t, errs)

	healthErrs := registry.HealthCheckAll()
	require.Len(t, healthErrs, 1)
	assert.Contains(t, healthErrs["main-db"].Error(), "connection lost")
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

	errs := registry.ShutdownAll(t.Context())
	assert.Empty(t, errs)
	assert.Equal(t, []string{"second", "first"}, shutdownLog)
}

// hungPlugin is a test-only api.Plugin whose CreateService blocks until
// the released channel is closed.
type hungPlugin struct {
	released chan struct{}
}

func (p *hungPlugin) Name() string                  { return "hungplugin" }
func (p *hungPlugin) Prefix() string                { return "hungplugin" }
func (p *hungPlugin) HasServices() bool             { return true }
func (p *hungPlugin) ServiceConfigSchema() map[string]any { return nil }
func (p *hungPlugin) Nodes() []api.NodeRegistration { return nil }
func (p *hungPlugin) HealthCheck(_ any) error       { return nil }
func (p *hungPlugin) Shutdown(_ any) error          { return nil }
func (p *hungPlugin) CreateService(_ map[string]any) (any, error) {
	<-p.released
	return struct{}{}, nil
}

// TestInitializeServices_LateCreateShutDownViaPlugin verifies that when a
// plugin's CreateService completes AFTER the create timeout has already
// fired (a "late" result), the cleanup goroutine tears the resulting
// instance down via the plugin's Shutdown method — not via a Close()
// type-assertion, which no service instance implements (platform-4).
func TestInitializeServices_LateCreateShutDownViaPlugin(t *testing.T) {
	shutdownCalled := make(chan any, 1)
	lateInstance := &struct{ id int }{id: 1}

	p := &servicePlugin{
		name:   "late-db",
		prefix: "db",
		createFunc: func(config map[string]any) (any, error) {
			time.Sleep(200 * time.Millisecond)
			return lateInstance, nil
		},
	}
	// Wrap Shutdown behavior via a dedicated plugin type so we can assert
	// on the exact instance passed, rather than just a name string.
	shutdownPlugin := &shutdownRecordingPlugin{servicePlugin: p, onShutdown: func(inst any) error {
		shutdownCalled <- inst
		return nil
	}}

	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(shutdownPlugin))

	servicesConfig := map[string]any{
		"main-db": map[string]any{"plugin": "db"},
	}

	// Short create timeout so CreateService's 200ms sleep triggers the
	// "creation timed out" path, exercising the cleanup goroutine.
	_, errs := InitializeServices(context.Background(), servicesConfig, plugins, 20*time.Millisecond)
	require.NotEmpty(t, errs)
	require.Contains(t, errs[0].Error(), "timed out")

	select {
	case inst := <-shutdownCalled:
		assert.Same(t, lateInstance, inst, "cleanup must Shutdown the late-completing instance via the plugin")
	case <-time.After(2 * time.Second):
		t.Fatal("plugin.Shutdown was not called on the late-completing instance")
	}
}

// shutdownRecordingPlugin wraps a servicePlugin to record/override Shutdown
// calls with a full instance value, rather than just a name string.
type shutdownRecordingPlugin struct {
	*servicePlugin
	onShutdown func(inst any) error
}

func (p *shutdownRecordingPlugin) Shutdown(service any) error {
	return p.onShutdown(service)
}

func TestInitializeServices_HungCreate_GoroutineExitsOnShutdown(t *testing.T) {
	defer goleak.VerifyNone(t)

	released := make(chan struct{})
	hung := &hungPlugin{released: released}

	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(hung))

	ctx, cancel := context.WithCancel(context.Background())

	servicesConfig := map[string]any{
		"hung": map[string]any{"plugin": "hungplugin"},
	}

	// Short timeout to make the test fast.
	_, errs := InitializeServices(ctx, servicesConfig, plugins, 100*time.Millisecond)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "timed out")

	// Cancel — cleanup goroutine should observe ctx.Done() and exit.
	cancel()

	// Brief scheduler hint so the goroutine actually exits before goleak runs.
	time.Sleep(50 * time.Millisecond)

	// Now release. Cleanup goroutine has already exited; this just lets
	// the underlying CreateService goroutine finish (and it's tracked
	// only by the runtime, not by us).
	close(released)
}
