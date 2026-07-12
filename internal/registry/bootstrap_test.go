package registry

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootstrap_ValidConfig(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "query", deps: map[string]api.ServiceDep{}},
			Factory:    func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))

	rc := &config.ResolvedConfig{
		Root:        map[string]any{},
		Workflows:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
	}

	result, errs := Bootstrap(context.Background(), rc, plugins)
	assert.Empty(t, errs)
	assert.NotNil(t, result)
	assert.Contains(t, result.Nodes.AllTypes(), "db.query")
}

func TestBootstrap_UnknownNodeType(t *testing.T) {
	plugins := NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step": map[string]any{"type": "unknown.node"},
				},
			},
		},
		Connections: map[string]map[string]any{},
	}

	_, errs := Bootstrap(context.Background(), rc, plugins)
	require.NotEmpty(t, errs)
}

func TestBootstrap_ExpressionMemoryBudgetFromEnvString(t *testing.T) {
	plugins := NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"server": map[string]any{"expression_memory_budget": "4096"},
		},
		Workflows:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
	}

	// act: Bootstrap must succeed and NOT report an error
	// (before the fix the string is silently ignored by toUint)
	result, errs := Bootstrap(context.Background(), rc, plugins)
	require.Empty(t, errs)
	require.NotNil(t, result)
}

func TestBootstrap_GarbageExpressionMemoryBudgetErrors(t *testing.T) {
	plugins := NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"server": map[string]any{"expression_memory_budget": "lots"},
		},
		Workflows:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
	}

	_, errs := Bootstrap(context.Background(), rc, plugins)
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "expression_memory_budget")
}

func TestBootstrap_InvalidServiceRef(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "query",
				deps: map[string]api.ServiceDep{"database": {Prefix: "db", Required: true}},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step": map[string]any{
						"type":     "db.query",
						"services": map[string]any{"database": "nonexistent"},
					},
				},
			},
		},
		Connections: map[string]map[string]any{},
	}

	_, errs := Bootstrap(context.Background(), rc, plugins)
	require.NotEmpty(t, errs)
}
