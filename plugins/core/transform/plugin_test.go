package transform

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

	got, ok := plugins.Get("transform")
	assert.True(t, ok)
	assert.Equal(t, "core.transform", got.Name())
}

func TestPlugin_AllNodesRegistered(t *testing.T) {
	nodeReg := registry.NewNodeRegistry()
	p := &Plugin{}
	require.NoError(t, nodeReg.RegisterFromPlugin(p))

	types := nodeReg.AllTypes()
	assert.Contains(t, types, "transform.set")
	assert.Contains(t, types, "transform.map")
	assert.Contains(t, types, "transform.filter")
	assert.Contains(t, types, "transform.merge")
	assert.Contains(t, types, "transform.delete")
	assert.Contains(t, types, "transform.validate")
}
