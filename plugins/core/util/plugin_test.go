package util

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlugin_Registration(t *testing.T) {
	plugins := registry.NewPluginRegistry()
	p := &Plugin{}
	require.NoError(t, plugins.Register(p))

	got, ok := plugins.Get("util")
	assert.True(t, ok)
	assert.Equal(t, "core.util", got.Name())
}

func TestPlugin_AllNodesRegistered(t *testing.T) {
	nodeReg := registry.NewNodeRegistry()
	p := &Plugin{}
	require.NoError(t, nodeReg.RegisterFromPlugin(p))

	types := nodeReg.AllTypes()
	assert.Contains(t, types, "util.log")
	assert.Contains(t, types, "util.uuid")
	assert.Contains(t, types, "util.delay")
	assert.Contains(t, types, "util.timestamp")
}

func TestPlugin_NodeCount(t *testing.T) {
	p := &Plugin{}
	nodes := p.Nodes()
	assert.Len(t, nodes, 4)

	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Descriptor.Name()
	}
	assert.ElementsMatch(t, []string{"log", "uuid", "delay", "timestamp"}, names)
}

func TestPlugin_HasNoServices(t *testing.T) {
	p := &Plugin{}
	assert.False(t, p.HasServices())

	svc, err := p.CreateService(nil)
	assert.NoError(t, err)
	assert.Nil(t, svc)

	assert.NoError(t, p.HealthCheck(nil))
	assert.NoError(t, p.Shutdown(nil))
}

func TestPlugin_NameAndPrefix(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "core.util", p.Name())
	assert.Equal(t, "util", p.Prefix())
}

func TestPlugin_NodeFactoriesCreateExecutors(t *testing.T) {
	p := &Plugin{}
	for _, reg := range p.Nodes() {
		t.Run(reg.Descriptor.Name(), func(t *testing.T) {
			executor := reg.Factory(map[string]any{})
			require.NotNil(t, executor)
			assert.NotEmpty(t, executor.Outputs())
		})
	}
}
