package registry

import (
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubPlugin is a minimal plugin implementation for testing.
type stubPlugin struct {
	name   string
	prefix string
}

func (p *stubPlugin) Name() string                                  { return p.name }
func (p *stubPlugin) Prefix() string                                { return p.prefix }
func (p *stubPlugin) Nodes() []api.NodeRegistration                 { return nil }
func (p *stubPlugin) HasServices() bool                             { return false }
func (p *stubPlugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *stubPlugin) HealthCheck(service any) error                 { return nil }
func (p *stubPlugin) Shutdown(service any) error                    { return nil }

func TestPluginRegistry_RegisterAndGet(t *testing.T) {
	reg := NewPluginRegistry()
	p := &stubPlugin{name: "test-db", prefix: "db"}

	err := reg.Register(p)
	require.NoError(t, err)

	got, ok := reg.Get("db")
	assert.True(t, ok)
	assert.Equal(t, "test-db", got.Name())
}

func TestPluginRegistry_MultiplePlugins(t *testing.T) {
	reg := NewPluginRegistry()
	require.NoError(t, reg.Register(&stubPlugin{name: "db-plugin", prefix: "db"}))
	require.NoError(t, reg.Register(&stubPlugin{name: "cache-plugin", prefix: "cache"}))

	assert.Len(t, reg.All(), 2)
	assert.Len(t, reg.Prefixes(), 2)
}

func TestPluginRegistry_DuplicatePrefix(t *testing.T) {
	reg := NewPluginRegistry()
	require.NoError(t, reg.Register(&stubPlugin{name: "plugin-a", prefix: "db"}))

	err := reg.Register(&stubPlugin{name: "plugin-b", prefix: "db"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plugin-a")
	assert.Contains(t, err.Error(), "plugin-b")
	assert.Contains(t, err.Error(), "duplicate")
}

func TestPluginRegistry_GetNonExistent(t *testing.T) {
	reg := NewPluginRegistry()

	_, ok := reg.Get("nonexistent")
	assert.False(t, ok)
}

func TestPluginRegistry_All(t *testing.T) {
	reg := NewPluginRegistry()
	require.NoError(t, reg.Register(&stubPlugin{name: "a", prefix: "a"}))
	require.NoError(t, reg.Register(&stubPlugin{name: "b", prefix: "b"}))
	require.NoError(t, reg.Register(&stubPlugin{name: "c", prefix: "c"}))

	all := reg.All()
	assert.Len(t, all, 3)

	names := make(map[string]bool)
	for _, p := range all {
		names[p.Name()] = true
	}
	assert.True(t, names["a"])
	assert.True(t, names["b"])
	assert.True(t, names["c"])
}
