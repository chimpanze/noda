package control

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPassExecutor returns config as output data.
type mockPassExecutor struct{}

func (e *mockPassExecutor) Outputs() []string { return api.DefaultOutputs() }
func (e *mockPassExecutor) Execute(_ context.Context, _ api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	return "success", config, nil
}

type mockDescriptor struct {
	name string
}

func (d *mockDescriptor) Name() string                           { return d.name }
func (d *mockDescriptor) Description() string                    { return "" }
func (d *mockDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *mockDescriptor) ConfigSchema() map[string]any           { return nil }

type mockPlugin struct {
	name   string
	prefix string
	nodes  []api.NodeRegistration
}

func (p *mockPlugin) Name() string                                     { return p.name }
func (p *mockPlugin) Prefix() string                                   { return p.prefix }
func (p *mockPlugin) Nodes() []api.NodeRegistration                    { return p.nodes }
func (p *mockPlugin) HasServices() bool                                { return false }
func (p *mockPlugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *mockPlugin) HealthCheck(service any) error                    { return nil }
func (p *mockPlugin) Shutdown(service any) error                       { return nil }

func setupIntegration(t *testing.T) (*registry.NodeRegistry, *registry.ServiceRegistry) {
	t.Helper()

	plugins := registry.NewPluginRegistry()
	nodeReg := registry.NewNodeRegistry()

	// Register the control plugin
	controlPlugin := &Plugin{}
	require.NoError(t, plugins.Register(controlPlugin))
	require.NoError(t, nodeReg.RegisterFromPlugin(controlPlugin))

	// Register mock plugin for pass-through nodes
	mock := &mockPlugin{
		name:   "mock",
		prefix: "mock",
		nodes: []api.NodeRegistration{
			{
				Descriptor: &mockDescriptor{name: "pass"},
				Factory:    func(map[string]any) api.NodeExecutor { return &mockPassExecutor{} },
			},
		},
	}
	require.NoError(t, plugins.Register(mock))
	require.NoError(t, nodeReg.RegisterFromPlugin(mock))

	return nodeReg, registry.NewServiceRegistry()
}

// controlResolver returns correct outputs per node type.
type controlResolver struct{}

func (r *controlResolver) OutputsForType(nodeType string) ([]string, bool) {
	switch nodeType {
	case "control.if":
		return []string{"then", "else", "error"}, true
	case "control.switch":
		return []string{"admin", "user", "default", "error"}, true
	case "control.loop":
		return []string{"done", "error"}, true
	default:
		return api.DefaultOutputs(), true
	}
}

func TestIntegration_IfBranching(t *testing.T) {
	nodeReg, svcReg := setupIntegration(t)
	resolver := &controlResolver{}

	wf := engine.WorkflowConfig{
		ID: "if-test",
		Nodes: map[string]engine.NodeConfig{
			"check":   {Type: "control.if", Config: map[string]any{"condition": `{{ input.admin }}`}},
			"granted": {Type: "mock.pass", Config: map[string]any{"result": "granted"}},
			"denied":  {Type: "mock.pass", Config: map[string]any{"result": "denied"}},
		},
		Edges: []engine.EdgeConfig{
			{From: "check", Output: "then", To: "granted"},
			{From: "check", Output: "else", To: "denied"},
		},
	}

	graph, err := engine.Compile(wf, resolver)
	require.NoError(t, err)

	// Admin path
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"admin": true}))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	data, ok := execCtx.GetOutput("granted")
	assert.True(t, ok)
	assert.Equal(t, "granted", data.(map[string]any)["result"])

	_, ok = execCtx.GetOutput("denied")
	assert.False(t, ok)

	// Non-admin path
	execCtx2 := engine.NewExecutionContext(engine.WithInput(map[string]any{"admin": false}))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx2, svcReg, nodeReg)
	require.NoError(t, err)

	_, ok = execCtx2.GetOutput("granted")
	assert.False(t, ok)
	data, ok = execCtx2.GetOutput("denied")
	assert.True(t, ok)
	assert.Equal(t, "denied", data.(map[string]any)["result"])
}

func TestIntegration_SwitchRouting(t *testing.T) {
	nodeReg, svcReg := setupIntegration(t)

	// Build a switch resolver that knows the specific cases
	switchResolver := &mapOutputResolver{
		types: map[string][]string{
			"control.switch": {"admin", "user", "default", "error"},
		},
		fallback: []string{"success", "error"},
	}

	wf := engine.WorkflowConfig{
		ID: "switch-test",
		Nodes: map[string]engine.NodeConfig{
			"route": {Type: "control.switch", Config: map[string]any{
				"expression": "{{ input.role }}",
				"cases":      []any{"admin", "user"},
			}},
			"admin_handler":   {Type: "mock.pass", Config: map[string]any{"handler": "admin"}},
			"user_handler":    {Type: "mock.pass", Config: map[string]any{"handler": "user"}},
			"default_handler": {Type: "mock.pass", Config: map[string]any{"handler": "default"}},
		},
		Edges: []engine.EdgeConfig{
			{From: "route", Output: "admin", To: "admin_handler"},
			{From: "route", Output: "user", To: "user_handler"},
			{From: "route", Output: "default", To: "default_handler"},
		},
	}

	graph, err := engine.Compile(wf, switchResolver)
	require.NoError(t, err)

	// Admin case
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"role": "admin"}))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	data, ok := execCtx.GetOutput("admin_handler")
	assert.True(t, ok)
	assert.Equal(t, "admin", data.(map[string]any)["handler"])

	// Default case
	execCtx2 := engine.NewExecutionContext(engine.WithInput(map[string]any{"role": "guest"}))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx2, svcReg, nodeReg)
	require.NoError(t, err)

	data, ok = execCtx2.GetOutput("default_handler")
	assert.True(t, ok)
	assert.Equal(t, "default", data.(map[string]any)["handler"])
}

// mapOutputResolver returns specific outputs for specific types.
type mapOutputResolver struct {
	types    map[string][]string
	fallback []string
}

func (r *mapOutputResolver) OutputsForType(nodeType string) ([]string, bool) {
	if outputs, ok := r.types[nodeType]; ok {
		return outputs, true
	}
	return r.fallback, true
}

func TestIntegration_PluginRegistration(t *testing.T) {
	plugins := registry.NewPluginRegistry()
	controlPlugin := &Plugin{}
	require.NoError(t, plugins.Register(controlPlugin))

	p, ok := plugins.Get("control")
	assert.True(t, ok)
	assert.Equal(t, "core.control", p.Name())

	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(controlPlugin))

	// All control node types registered
	types := nodeReg.AllTypes()
	assert.Contains(t, types, "control.if")
	assert.Contains(t, types, "control.switch")
	assert.Contains(t, types, "control.loop")
}
