package registry

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDescriptor is a minimal NodeDescriptor for testing.
type stubDescriptor struct {
	name string
	deps map[string]api.ServiceDep
}

func (d *stubDescriptor) Name() string                           { return d.name }
func (d *stubDescriptor) ServiceDeps() map[string]api.ServiceDep { return d.deps }
func (d *stubDescriptor) ConfigSchema() map[string]any           { return nil }

// stubExecutor is a minimal NodeExecutor for testing.
type stubExecutor struct {
	outputs []string
}

func (e *stubExecutor) Outputs() []string { return e.outputs }
func (e *stubExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return "ok", nil, nil
}

// nodePlugin is a plugin with node registrations for testing.
type nodePlugin struct {
	name   string
	prefix string
	nodes  []api.NodeRegistration
}

func (p *nodePlugin) Name() string                                     { return p.name }
func (p *nodePlugin) Prefix() string                                   { return p.prefix }
func (p *nodePlugin) Nodes() []api.NodeRegistration                    { return p.nodes }
func (p *nodePlugin) HasServices() bool                                { return false }
func (p *nodePlugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *nodePlugin) HealthCheck(service any) error                    { return nil }
func (p *nodePlugin) Shutdown(service any) error                       { return nil }

func pluginWithNodes(name, prefix string, nodes []api.NodeRegistration) *nodePlugin {
	return &nodePlugin{name: name, prefix: prefix, nodes: nodes}
}

func TestNodeRegistry_RegisterAndGet(t *testing.T) {
	reg := NewNodeRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query"},
			Factory: func(config map[string]any) api.NodeExecutor {
				return &stubExecutor{outputs: []string{"ok", "error"}}
			},
		},
	})

	err := reg.RegisterFromPlugin(p)
	require.NoError(t, err)

	desc, ok := reg.GetDescriptor("db.query")
	assert.True(t, ok)
	assert.Equal(t, "query", desc.Name())
}

func TestNodeRegistry_GetFactory(t *testing.T) {
	reg := NewNodeRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query"},
			Factory: func(config map[string]any) api.NodeExecutor {
				return &stubExecutor{outputs: []string{"ok"}}
			},
		},
	})
	require.NoError(t, reg.RegisterFromPlugin(p))

	factory, ok := reg.GetFactory("db.query")
	assert.True(t, ok)

	executor := factory(nil)
	assert.Equal(t, []string{"ok"}, executor.Outputs())
}

func TestNodeRegistry_AllTypes(t *testing.T) {
	reg := NewNodeRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{Descriptor: &stubDescriptor{name: "query"}, Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} }},
		{Descriptor: &stubDescriptor{name: "exec"}, Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} }},
	})
	require.NoError(t, reg.RegisterFromPlugin(p))

	types := reg.AllTypes()
	assert.Len(t, types, 2)
	assert.Contains(t, types, "db.query")
	assert.Contains(t, types, "db.exec")
}

func TestNodeRegistry_TypesByPrefix(t *testing.T) {
	reg := NewNodeRegistry()
	require.NoError(t, reg.RegisterFromPlugin(pluginWithNodes("db", "db", []api.NodeRegistration{
		{Descriptor: &stubDescriptor{name: "query"}, Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} }},
	})))
	require.NoError(t, reg.RegisterFromPlugin(pluginWithNodes("cache", "cache", []api.NodeRegistration{
		{Descriptor: &stubDescriptor{name: "get"}, Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} }},
		{Descriptor: &stubDescriptor{name: "set"}, Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} }},
	})))

	dbTypes := reg.TypesByPrefix("db")
	assert.Len(t, dbTypes, 1)
	assert.Contains(t, dbTypes, "db.query")

	cacheTypes := reg.TypesByPrefix("cache")
	assert.Len(t, cacheTypes, 2)
}

func TestNodeRegistry_DuplicateType(t *testing.T) {
	reg := NewNodeRegistry()
	nodes := []api.NodeRegistration{
		{Descriptor: &stubDescriptor{name: "query"}, Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} }},
	}
	require.NoError(t, reg.RegisterFromPlugin(pluginWithNodes("db1", "db", nodes)))

	err := reg.RegisterFromPlugin(pluginWithNodes("db2", "db", nodes))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.query")
	assert.Contains(t, err.Error(), "duplicate")
}

func TestNodeRegistry_GetNonExistent(t *testing.T) {
	reg := NewNodeRegistry()
	_, ok := reg.GetDescriptor("nonexistent.type")
	assert.False(t, ok)

	_, ok = reg.GetFactory("nonexistent.type")
	assert.False(t, ok)
}
