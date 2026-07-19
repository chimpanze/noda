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

func TestValidateStartupDryRun_WrongPrefix(t *testing.T) {
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
	require.NoError(t, plugins.Register(dbPlugin))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(dbPlugin))

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"my-cache": map[string]any{
					"plugin": "cache",
				},
			},
		},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type": "db.query",
						"services": map[string]any{
							"database": "my-cache", // cache service in a db slot
						},
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "prefix")
	assert.Contains(t, errs[0].Error(), "cache")
	assert.Contains(t, errs[0].Error(), "db")
}

func TestValidateStartupDryRun_NodeConfigSchemaEnforced(t *testing.T) {
	kvPlugin := &testKVPlugin{}
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(kvPlugin))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(kvPlugin))

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"my-store": map[string]any{"plugin": "kv"},
			},
		},
		Workflows: map[string]map[string]any{
			"write-wf": {
				"nodes": map[string]any{
					"write": map[string]any{
						"type":     "kv.set",
						"services": map[string]any{"store": "my-store"},
						"config":   map[string]any{"value": "hello"}, // missing required "key"
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "missing required config field")
	assert.Contains(t, errs[0].Error(), "write-wf")
	assert.Contains(t, errs[0].Error(), "write")
}

func TestValidateStartupDryRun_MissingConfigTreatedAsEmpty(t *testing.T) {
	kvPlugin := &testKVPlugin{}
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(kvPlugin))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(kvPlugin))

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"my-store": map[string]any{"plugin": "kv"},
			},
		},
		Workflows: map[string]map[string]any{
			"write-wf": {
				"nodes": map[string]any{
					"write": map[string]any{
						"type":     "kv.set",
						"services": map[string]any{"store": "my-store"},
						// no "config" key at all
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "missing required config field")
	assert.Contains(t, errs[0].Error(), "write-wf")
	assert.Contains(t, errs[0].Error(), "write")
}

func TestValidateStartup_NodeConfigSchemaEnforced(t *testing.T) {
	kvPlugin := &testKVPlugin{}
	plugins := NewPluginRegistry()
	require.NoError(t, plugins.Register(kvPlugin))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(kvPlugin))

	services := NewServiceRegistry()
	require.NoError(t, services.Register("my-store", "kv", kvPlugin))

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"write-wf": {
				"nodes": map[string]any{
					"write": map[string]any{
						"type":     "kv.set",
						"services": map[string]any{"store": "my-store"},
						"config":   map[string]any{"value": "hello"}, // missing required "key"
					},
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, services, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "missing required config field")
	assert.Contains(t, errs[0].Error(), "write-wf")
	assert.Contains(t, errs[0].Error(), "write")
}

// schemaOnlyPlugin is a service-only plugin stub whose ServiceConfigSchema is
// configurable per test, exercising ValidateStartupDryRun's #376
// service-config-schema enforcement without any node registrations.
type schemaOnlyPlugin struct {
	name        string
	prefix      string
	hasServices bool
	schema      map[string]any
}

func (p *schemaOnlyPlugin) Name() string                                     { return p.name }
func (p *schemaOnlyPlugin) Prefix() string                                   { return p.prefix }
func (p *schemaOnlyPlugin) Nodes() []api.NodeRegistration                    { return nil }
func (p *schemaOnlyPlugin) HasServices() bool                                { return p.hasServices }
func (p *schemaOnlyPlugin) ServiceConfigSchema() map[string]any              { return p.schema }
func (p *schemaOnlyPlugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *schemaOnlyPlugin) HealthCheck(service any) error                    { return nil }
func (p *schemaOnlyPlugin) Shutdown(service any) error                       { return nil }

func authLikeSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"database": map[string]any{"type": "string"},
		},
		"required":             []any{"database"},
		"additionalProperties": false,
	}
}

func TestValidateStartupDryRun_ServiceConfigSchema_MissingRequiredField(t *testing.T) {
	plugins := NewPluginRegistry()
	authPlugin := &schemaOnlyPlugin{name: "auth", prefix: "auth", hasServices: true, schema: authLikeSchema()}
	require.NoError(t, plugins.Register(authPlugin))

	nodes := NewNodeRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"auth": map[string]any{
					"plugin": "auth",
					"config": map[string]any{}, // missing required "database"
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `service "auth" (plugin "auth")`)
}

func TestValidateStartupDryRun_ServiceConfigSchema_ValidConfigPasses(t *testing.T) {
	plugins := NewPluginRegistry()
	authPlugin := &schemaOnlyPlugin{name: "auth", prefix: "auth", hasServices: true, schema: authLikeSchema()}
	require.NoError(t, plugins.Register(authPlugin))

	nodes := NewNodeRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"auth": map[string]any{
					"plugin": "auth",
					"config": map[string]any{"database": "main-db"},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_ServiceConfigSchema_UnknownPluginSkipped(t *testing.T) {
	plugins := NewPluginRegistry()
	nodes := NewNodeRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"auth": map[string]any{
					"plugin": "does-not-exist",
					"config": map[string]any{},
				},
			},
		},
	}

	// Unknown plugin name: dry-run skips service-schema validation for it
	// (crossref validation elsewhere is responsible for flagging it).
	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_ServiceConfigSchema_ExtraKeyRejected(t *testing.T) {
	plugins := NewPluginRegistry()
	authPlugin := &schemaOnlyPlugin{name: "auth", prefix: "auth", hasServices: true, schema: authLikeSchema()}
	require.NoError(t, plugins.Register(authPlugin))

	nodes := NewNodeRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"auth": map[string]any{
					"plugin": "auth",
					"config": map[string]any{
						"database": "main-db",
						"databse":  "typo", // additionalProperties: false
					},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `service "auth" (plugin "auth")`)
}

func TestValidateStartupDryRun_ServiceConfigSchema_ServiceLessPluginNeverValidated(t *testing.T) {
	plugins := NewPluginRegistry()
	// HasServices=false, no schema — must never be run through service
	// schema validation even if a "services" entry incorrectly names it.
	noServicePlugin := &schemaOnlyPlugin{name: "control", prefix: "control", hasServices: false, schema: nil}
	require.NoError(t, plugins.Register(noServicePlugin))

	nodes := NewNodeRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"ctrl": map[string]any{
					"plugin": "control",
					"config": map[string]any{"anything": "goes"},
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

// setupEdgeValidation registers a "control" plugin whose two node types mimic
// the real control.if (fixed [then, else, error]) and control.switch
// (config-aware: [cases..., default, error]) output shapes so the edge-output
// dry-run check can be exercised without pulling in plugins/core/control.
func setupEdgeValidation(t *testing.T) (*PluginRegistry, *NodeRegistry) {
	t.Helper()

	plugins := NewPluginRegistry()
	controlPlugin := pluginWithNodes("test-control", "control", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "if"},
			Factory: func(map[string]any) api.NodeExecutor {
				return &stubExecutor{outputs: []string{"then", "else", "error"}}
			},
		},
		{
			Descriptor: &stubDescriptor{name: "switch"},
			Factory: func(cfg map[string]any) api.NodeExecutor {
				outputs := []string{}
				if cases, ok := cfg["cases"].([]any); ok {
					for _, c := range cases {
						if s, ok := c.(string); ok {
							outputs = append(outputs, s)
						}
					}
				}
				outputs = append(outputs, "default", "error")
				return &stubExecutor{outputs: outputs}
			},
		},
	})
	require.NoError(t, plugins.Register(controlPlugin))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(controlPlugin))

	return plugins, nodes
}

func edgeWorkflowNode(nodeType string, cfg map[string]any) map[string]any {
	n := map[string]any{"type": nodeType}
	if cfg != nil {
		n["config"] = cfg
	}
	return n
}

func edge(from, to, output string) map[string]any {
	e := map[string]any{"from": from, "to": to}
	if output != "" {
		e["output"] = output
	}
	return e
}

func TestValidateStartupDryRun_EdgeOutput_ControlIfInvalidOutput(t *testing.T) {
	plugins, nodes := setupEdgeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"decide": edgeWorkflowNode("control.if", nil),
					"next":   edgeWorkflowNode("control.if", nil),
				},
				// #378 bug class: "true" is not a declared control.if output.
				"edges": []any{edge("decide", "next", "true")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `workflow "wf1": edge "decide" -> "next": output "true" not among declared outputs [then else error]`)
}

func TestValidateStartupDryRun_EdgeOutput_ControlIfValidOutput(t *testing.T) {
	plugins, nodes := setupEdgeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"decide": edgeWorkflowNode("control.if", nil),
					"next":   edgeWorkflowNode("control.if", nil),
				},
				"edges": []any{edge("decide", "next", "then")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_EdgeOutput_EmptyDefaultsToSuccess(t *testing.T) {
	plugins := NewPluginRegistry()
	successPlugin := pluginWithNodes("test-success", "control", []api.NodeRegistration{
		{
			Descriptor: &stubDescriptor{name: "noop"},
			Factory: func(map[string]any) api.NodeExecutor {
				return &stubExecutor{outputs: []string{"success", "error"}}
			},
		},
	})
	require.NoError(t, plugins.Register(successPlugin))
	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(successPlugin))

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"step": edgeWorkflowNode("control.noop", nil),
					"next": edgeWorkflowNode("control.noop", nil),
				},
				// no "output" key at all -> defaults to "success", which the
				// [success, error] node declares.
				"edges": []any{edge("step", "next", "")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_EdgeOutput_SwitchConfigAware(t *testing.T) {
	plugins, nodes := setupEdgeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"route": edgeWorkflowNode("control.switch", map[string]any{
						"cases": []any{"opened", "closed"},
					}),
					"next": edgeWorkflowNode("control.switch", nil),
				},
				"edges": []any{edge("route", "next", "opened")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_EdgeOutput_SwitchConfigAwareInvalid(t *testing.T) {
	plugins, nodes := setupEdgeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"route": edgeWorkflowNode("control.switch", map[string]any{
						"cases": []any{"opened", "closed"},
					}),
					"next": edgeWorkflowNode("control.switch", nil),
				},
				"edges": []any{edge("route", "next", "openedd")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `workflow "wf1": edge "route" -> "next": output "openedd" not among declared outputs [opened closed default error]`)
}

func TestValidateStartupDryRun_EdgeOutput_UnknownNodeTypeSkipsEdgeCheck(t *testing.T) {
	plugins, nodes := setupEdgeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"decide": edgeWorkflowNode("mystery.node", nil),
					"next":   edgeWorkflowNode("control.if", nil),
				},
				// "mystery.node" has an unknown prefix; the node-type check
				// already flags it. The edge check must not pile on with a
				// second, misleading error about the (nonexistent) outputs.
				"edges": []any{edge("decide", "next", "anything")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unknown node type prefix")
}

// TestValidateStartupDryRun_EdgeUnknownSourceNode guards against I4 (final
// review): the edge check used to skip edges whose "from" node was missing
// with a comment claiming another check owned that case, but no such check
// existed — so a config referencing a nonexistent source node validated as
// clean via `noda validate`/MCP/editor while engine.Compile
// (internal/engine/compiler.go) rejects the same config at boot.
func TestValidateStartupDryRun_EdgeUnknownSourceNode(t *testing.T) {
	plugins, nodes := setupEdgeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"next": edgeWorkflowNode("control.if", nil),
				},
				"edges": []any{edge("missing", "next", "then")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `workflow "wf1": edge references unknown source node "missing"`)
}

// TestValidateStartupDryRun_EdgeUnknownTargetNode is I4's "to" counterpart:
// engine.Compile rejects an edge whose target node doesn't exist, but the
// dry-run edge check never looked at "to" at all.
func TestValidateStartupDryRun_EdgeUnknownTargetNode(t *testing.T) {
	plugins, nodes := setupEdgeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"decide": edgeWorkflowNode("control.if", nil),
				},
				"edges": []any{edge("decide", "missing", "then")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `workflow "wf1": edge references unknown target node "missing"`)
}
