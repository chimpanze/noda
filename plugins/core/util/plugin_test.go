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
