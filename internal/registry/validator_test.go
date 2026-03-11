package registry

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupValidation(t *testing.T) (*PluginRegistry, *ServiceRegistry, *NodeRegistry) {
	t.Helper()

	plugins := NewPluginRegistry()
	dbPlugin := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "query",
				deps: map[string]api.ServiceDep{
					"database": {Prefix: "db", Required: true},
				},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	cachePlugin := pluginWithNodes("test-cache", "cache", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "get",
				deps: map[string]api.ServiceDep{
					"cache": {Prefix: "cache", Required: true},
				},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})

	require.NoError(t, plugins.Register(dbPlugin))
	require.NoError(t, plugins.Register(cachePlugin))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(dbPlugin))
	require.NoError(t, nodes.RegisterFromPlugin(cachePlugin))

	services := NewServiceRegistry()
	require.NoError(t, services.Register("main-db", "db-inst", dbPlugin))
	require.NoError(t, services.Register("redis", "cache-inst", cachePlugin))

	return plugins, services, nodes
}

func TestValidateStartup_ValidConfig(t *testing.T) {
	plugins, services, nodes := setupValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"get-user": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type": "db.query",
						"services": map[string]any{
							"database": "main-db",
						},
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartup_UnknownNodeTypePrefix(t *testing.T) {
	plugins, services, nodes := setupValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step1": map[string]any{
						"type": "email.send",
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unknown node type prefix")
	assert.Contains(t, errs[0].Error(), "email")
	assert.Contains(t, errs[0].Error(), "wf1")
}

func TestValidateStartup_MissingServiceRef(t *testing.T) {
	plugins, services, nodes := setupValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type": "db.query",
						"services": map[string]any{
							"database": "nonexistent-db",
						},
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "nonexistent-db")
	assert.Contains(t, errs[0].Error(), "not found")
}

func TestValidateStartup_WrongPrefixOnSlot(t *testing.T) {
	plugins, services, nodes := setupValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type": "db.query",
						"services": map[string]any{
							"database": "redis", // cache service in a db slot
						},
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "prefix")
	assert.Contains(t, errs[0].Error(), "cache")
	assert.Contains(t, errs[0].Error(), "db")
}

func TestValidateStartup_MissingRequiredSlot(t *testing.T) {
	plugins, services, nodes := setupValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type":     "db.query",
						"services": map[string]any{}, // no database slot
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "missing required service slot")
	assert.Contains(t, errs[0].Error(), "database")
}

func TestValidateStartup_OptionalSlotUnfilled(t *testing.T) {
	plugins := NewPluginRegistry()
	p := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{
				name: "query",
				deps: map[string]api.ServiceDep{
					"cache": {Prefix: "cache", Required: false},
				},
			},
			Factory: func(map[string]any) api.NodeExecutor { return &stubExecutor{} },
		},
	})
	require.NoError(t, plugins.Register(p))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(p))
	services := NewServiceRegistry()

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type": "db.query",
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartup_MultipleErrors(t *testing.T) {
	plugins, services, nodes := setupValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step1": map[string]any{
						"type": "email.send", // unknown prefix
					},
					"step2": map[string]any{
						"type":     "db.query",
						"services": map[string]any{}, // missing required slot
					},
				},
			},
			"wf2": {
				"nodes": map[string]any{
					"step1": map[string]any{
						"type": "db.query",
						"services": map[string]any{
							"database": "redis", // wrong prefix
						},
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Len(t, errs, 3)
}
