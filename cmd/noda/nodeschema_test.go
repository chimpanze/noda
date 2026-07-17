package main

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/require"
)

// buildFullNodeRegistry registers every built-in plugin's nodes, exactly as
// server boot and `noda validate` do.
func buildFullNodeRegistry(t *testing.T) *registry.NodeRegistry {
	t.Helper()
	plugins := registry.NewPluginRegistry()
	require.NoError(t, registerCorePlugins(plugins))
	nodes := registry.NewNodeRegistry()
	for _, p := range plugins.All() {
		require.NoError(t, nodes.RegisterFromPlugin(p))
	}
	return nodes
}

// Every node ConfigSchema must stay within the vocabulary the config
// validator implements — otherwise a constraint would be silently ignored.
func TestNodeConfigSchemas_SupportedVocabulary(t *testing.T) {
	nodes := buildFullNodeRegistry(t)
	types := nodes.AllTypes()
	require.NotEmpty(t, types)
	for _, nodeType := range types {
		desc, ok := nodes.GetDescriptor(nodeType)
		require.True(t, ok, nodeType)
		schema := desc.ConfigSchema()
		if schema == nil {
			continue
		}
		for _, err := range registry.CheckSchemaVocabulary(schema) {
			t.Errorf("%s: %v", nodeType, err)
		}
	}
}
