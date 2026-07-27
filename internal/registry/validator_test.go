package registry

import (
	"strings"
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
	// The cache plugin must be registered: since #385 an unknown plugin name
	// is its own dry-run error, and this test targets only the prefix check.
	cachePlugin := pluginWithNodes("cache", "cache", nil)
	require.NoError(t, plugins.Register(cachePlugin))

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

// registry.WorkflowScopedError must carry the workflow file that produced the
// error, with Error() left byte-identical to the unwrapped message — the
// contract internal/startup.attributeRegistries relies on (errors.As) instead
// of parsing the "workflow %q, ..." text back out.
func TestValidateStartupDryRun_NodeConfigSchemaErrorIsWorkflowScoped(t *testing.T) {
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

	var wfErr *WorkflowScopedError
	require.ErrorAs(t, errs[0], &wfErr)
	assert.Equal(t, "write-wf", wfErr.File)
	assert.Equal(t, wfErr.Err.Error(), errs[0].Error(), "wrapping must not change the error text")
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

// Same contract as ValidateStartupDryRun's counterpart above — ValidateStartup
// and ValidateStartupDryRun are largely duplicated, so both are checked.
func TestValidateStartup_NodeConfigSchemaErrorIsWorkflowScoped(t *testing.T) {
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

	var wfErr *WorkflowScopedError
	require.ErrorAs(t, errs[0], &wfErr)
	assert.Equal(t, "write-wf", wfErr.File)
	assert.Equal(t, wfErr.Err.Error(), errs[0].Error(), "wrapping must not change the error text")
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

func TestValidateStartupDryRun_ServiceConfigSchema_UnknownPluginErrors(t *testing.T) {
	plugins := NewPluginRegistry()
	authPlugin := &schemaOnlyPlugin{name: "auth", prefix: "auth", hasServices: true, schema: authLikeSchema()}
	require.NoError(t, plugins.Register(authPlugin))
	nodes := NewNodeRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"services": map[string]any{
				"mystery": map[string]any{
					"plugin": "does-not-exist",
					"config": map[string]any{},
				},
			},
		},
	}

	// #385: crossrefs only flag an unknown plugin when a node references the
	// service — an unreferenced entry must still fail the dry run instead of
	// passing validate and erroring at boot.
	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `unknown plugin "does-not-exist"`)
	assert.Contains(t, errs[0].Error(), "mystery")
	assert.Contains(t, errs[0].Error(), "known plugins: auth")
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

// outcomeStubDescriptor is a stubDescriptor that declares outcome outputs.
type outcomeStubDescriptor struct {
	stubDescriptor
	outcomes []string
}

func (d *outcomeStubDescriptor) OutcomeOutputs() []string { return d.outcomes }

// setupOutcomeValidation registers a "db"-like plugin with one node that
// declares an outcome output ("create": exists) and one that doesn't
// ("findOne"), so the unwired-outcome-output check (#442) can be exercised
// without pulling in plugins/db.
func setupOutcomeValidation(t *testing.T) (*PluginRegistry, *NodeRegistry) {
	t.Helper()

	plugins := NewPluginRegistry()
	dbPlugin := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &outcomeStubDescriptor{stubDescriptor: stubDescriptor{name: "create"}, outcomes: []string{"exists"}},
			Factory: func(map[string]any) api.NodeExecutor {
				return &stubExecutor{outputs: []string{"success", "exists", "error"}}
			},
		},
		{
			Descriptor: &stubDescriptor{name: "findOne"},
			Factory: func(map[string]any) api.NodeExecutor {
				return &stubExecutor{outputs: []string{"success", "error"}}
			},
		},
	})
	require.NoError(t, plugins.Register(dbPlugin))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(dbPlugin))
	return plugins, nodes
}

// TestValidateStartup_OutcomeOutput_UnwiredRejected guards against the live
// boot path (ValidateStartup, called from bootstrap.go when DryRun:false)
// missing check 5 entirely: a config `noda validate`/dry-run rejects for an
// unwired outcome output must ALSO be rejected by `noda start`/`noda dev`,
// not boot cleanly. Mirrors TestValidateStartupDryRun_OutcomeOutput_UnwiredRejected.
func TestValidateStartup_OutcomeOutput_UnwiredRejected(t *testing.T) {
	plugins, nodes := setupOutcomeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"insert": edgeWorkflowNode("db.create", nil),
					"next":   edgeWorkflowNode("db.findOne", nil),
				},
				"edges": []any{edge("insert", "next", "success")},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, nil, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `workflow "wf1", node "insert" (db.create): outcome output "exists" has no outbound edge`)
}

// TestValidateStartup_OutcomeOutput_WiredAccepted is the live-boot-path
// counterpart of TestValidateStartupDryRun_OutcomeOutput_WiredAccepted: once
// the outcome output is wired, ValidateStartup must not reject it.
func TestValidateStartup_OutcomeOutput_WiredAccepted(t *testing.T) {
	plugins, nodes := setupOutcomeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"insert": edgeWorkflowNode("db.create", nil),
					"next":   edgeWorkflowNode("db.findOne", nil),
				},
				"edges": []any{
					edge("insert", "next", "success"),
					edge("insert", "next", "exists"),
				},
			},
		},
	}

	errs := ValidateStartup(rc, plugins, nil, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_OutcomeOutput_UnwiredRejected(t *testing.T) {
	plugins, nodes := setupOutcomeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"insert": edgeWorkflowNode("db.create", nil),
					"next":   edgeWorkflowNode("db.findOne", nil),
				},
				"edges": []any{edge("insert", "next", "success")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `workflow "wf1", node "insert" (db.create): outcome output "exists" has no outbound edge`)
}

func TestValidateStartupDryRun_OutcomeOutput_WiredAccepted(t *testing.T) {
	plugins, nodes := setupOutcomeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"insert": edgeWorkflowNode("db.create", nil),
					"next":   edgeWorkflowNode("db.findOne", nil),
				},
				"edges": []any{
					edge("insert", "next", "success"),
					edge("insert", "next", "exists"),
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_OutcomeOutput_NoEdgesAtAllRejected(t *testing.T) {
	plugins, nodes := setupOutcomeValidation(t)

	// A workflow with no "edges" key at all still fails: the outcome output
	// is unwired regardless of whether the edge list exists.
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"insert": edgeWorkflowNode("db.create", nil),
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `outcome output "exists" has no outbound edge`)
}

func TestValidateStartupDryRun_OutcomeOutput_ControlFlowExempt(t *testing.T) {
	plugins, nodes := setupEdgeValidation(t)

	// control.if with only "then" wired is a legitimate shape ("do nothing
	// when false") — stubDescriptor implements no OutcomeOutputsProvider,
	// exactly like the real control descriptors.
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

// workflowScopedFixture is what one row of
// TestValidateStartup_EveryWorkflowErrorIsWorkflowScoped needs to call both
// ValidateStartup and ValidateStartupDryRun. services may be nil — passed
// straight through to ValidateStartup, which accepts a nil *ServiceRegistry
// fine as long as the fixture's node has no required service slots.
type workflowScopedFixture struct {
	rc       *config.ResolvedConfig
	plugins  *PluginRegistry
	nodes    *NodeRegistry
	services *ServiceRegistry
	deferred map[string]DeferredService
}

// TestValidateStartup_EveryWorkflowErrorIsWorkflowScoped pins that every
// "for wfName, wf := range rc.Workflows" error producer in both
// ValidateStartup and ValidateStartupDryRun wraps its error in a
// *WorkflowScopedError naming the declaring file — not just the
// config-schema case exercised elsewhere in this file.
//
// This exists because a re-review of the #456 attribution fix deleted the
// wrap from the edge-unknown-source-node check and reran
// ./internal/registry/ ./internal/startup/ ./internal/editor/: everything
// stayed green. Only the config-schema call sites and the synthetic
// attributeRegistries unit tests had coverage that would catch a dropped
// wrap; the node-type-prefix, unknown-node-type, service-slot, edge-output,
// expression/static-field, and outcome-output wraps had nothing pinning
// them. This table is that pin, one row per producer group, each verified
// (see the fix report) to redden when its wrap is removed.
//
// Each row runs against both entry points except where noted — the two
// functions are largely duplicated, which is exactly how a wrap lands in one
// and is missed in the other.
func TestValidateStartup_EveryWorkflowErrorIsWorkflowScoped(t *testing.T) {
	cases := []struct {
		name             string
		file             string // the rc.Workflows key the error must name
		wantErrSubstr    string
		dryRunOnly       bool
		dryRunOnlyReason string
		build            func(t *testing.T) workflowScopedFixture
	}{
		{
			name:          "unknown node type prefix",
			file:          "prefix-wf",
			wantErrSubstr: "unknown node type prefix",
			build: func(t *testing.T) workflowScopedFixture {
				plugins, services, nodes := setupValidation(t)
				rc := &config.ResolvedConfig{
					Workflows: map[string]map[string]any{
						"prefix-wf": {
							"nodes": map[string]any{
								"step1": map[string]any{"type": "email.send"},
							},
						},
					},
				}
				return workflowScopedFixture{rc: rc, plugins: plugins, nodes: nodes, services: services}
			},
		},
		{
			name:          "unknown node type",
			file:          "type-wf",
			wantErrSubstr: "unknown node type",
			build: func(t *testing.T) workflowScopedFixture {
				// setupValidation registers "db" with only a "query" node, so
				// "db.unknown" has a known prefix but no matching descriptor.
				plugins, services, nodes := setupValidation(t)
				rc := &config.ResolvedConfig{
					Workflows: map[string]map[string]any{
						"type-wf": {
							"nodes": map[string]any{
								"step1": map[string]any{"type": "db.unknown"},
							},
						},
					},
				}
				return workflowScopedFixture{rc: rc, plugins: plugins, nodes: nodes, services: services}
			},
		},
		{
			name:          "node config schema violation",
			file:          "config-schema-wf",
			wantErrSubstr: "missing required config field",
			build: func(t *testing.T) workflowScopedFixture {
				kvPlugin := &testKVPlugin{}
				plugins := NewPluginRegistry()
				require.NoError(t, plugins.Register(kvPlugin))
				nodes := NewNodeRegistry()
				require.NoError(t, nodes.RegisterFromPlugin(kvPlugin))
				services := NewServiceRegistry()
				require.NoError(t, services.Register("my-store", "kv", kvPlugin))

				rc := &config.ResolvedConfig{
					Root: map[string]any{
						"services": map[string]any{
							"my-store": map[string]any{"plugin": "kv"},
						},
					},
					Workflows: map[string]map[string]any{
						"config-schema-wf": {
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
				return workflowScopedFixture{rc: rc, plugins: plugins, nodes: nodes, services: services}
			},
		},
		{
			name:          "service slot / service reference failure",
			file:          "slot-wf",
			wantErrSubstr: "missing required service slot",
			build: func(t *testing.T) workflowScopedFixture {
				plugins, services, nodes := setupValidation(t)
				rc := &config.ResolvedConfig{
					Workflows: map[string]map[string]any{
						"slot-wf": {
							"nodes": map[string]any{
								"fetch": map[string]any{"type": "db.query"}, // no "services" key at all
							},
						},
					},
				}
				return workflowScopedFixture{rc: rc, plugins: plugins, nodes: nodes, services: services}
			},
		},
		{
			name:             "edge output failure (edge references unknown source node)",
			file:             "edge-wf",
			wantErrSubstr:    `edge references unknown source node "missing"`,
			dryRunOnly:       true,
			dryRunOnlyReason: "ValidateStartup has no edge-output-validation phase at all — only ValidateStartupDryRun mirrors engine.Compile's edge checks (#379)",
			build: func(t *testing.T) workflowScopedFixture {
				plugins, nodes := setupEdgeValidation(t)
				rc := &config.ResolvedConfig{
					Workflows: map[string]map[string]any{
						"edge-wf": {
							"nodes": map[string]any{
								"next": edgeWorkflowNode("control.if", nil),
							},
							"edges": []any{edge("missing", "next", "then")},
						},
					},
				}
				return workflowScopedFixture{rc: rc, plugins: plugins, nodes: nodes}
			},
		},
		{
			name:          "expression failure",
			file:          "expr-wf",
			wantErrSubstr: "expression error",
			build: func(t *testing.T) workflowScopedFixture {
				plugins, nodes := setupEdgeValidation(t)
				rc := &config.ResolvedConfig{
					Workflows: map[string]map[string]any{
						"expr-wf": {
							"nodes": map[string]any{
								// "{{ input.name ++ }}" is TestCompile_InvalidSyntax's
								// known-bad expression (internal/expr/compiler_test.go).
								"decide": edgeWorkflowNode("control.if", map[string]any{
									"message": "{{ input.name ++ }}",
								}),
							},
						},
					},
				}
				return workflowScopedFixture{rc: rc, plugins: plugins, nodes: nodes}
			},
		},
		{
			name:          "outcome output failure",
			file:          "outcome-wf",
			wantErrSubstr: "outcome output",
			build: func(t *testing.T) workflowScopedFixture {
				plugins, nodes := setupOutcomeValidation(t)
				rc := &config.ResolvedConfig{
					Workflows: map[string]map[string]any{
						"outcome-wf": {
							"nodes": map[string]any{
								"insert": edgeWorkflowNode("db.create", nil), // "exists" never wired
							},
						},
					},
				}
				return workflowScopedFixture{rc: rc, plugins: plugins, nodes: nodes}
			},
		},
	}

	entryPoints := []struct {
		name string
		run  func(workflowScopedFixture) []error
	}{
		{"ValidateStartup", func(f workflowScopedFixture) []error {
			return ValidateStartup(f.rc, f.plugins, f.services, f.nodes, expr.NewCompilerWithFunctions(), f.deferred)
		}},
		{"ValidateStartupDryRun", func(f workflowScopedFixture) []error {
			return ValidateStartupDryRun(f.rc, f.plugins, f.nodes, expr.NewCompilerWithFunctions(), f.deferred)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, ep := range entryPoints {
				if tc.dryRunOnly && ep.name == "ValidateStartup" {
					t.Logf("skipping ValidateStartup: %s", tc.dryRunOnlyReason)
					continue
				}
				t.Run(ep.name, func(t *testing.T) {
					f := tc.build(t)
					errs := ep.run(f)
					require.NotEmpty(t, errs, "fixture must produce at least one error")

					var target error
					for _, e := range errs {
						if strings.Contains(e.Error(), tc.wantErrSubstr) {
							target = e
							break
						}
					}
					require.NotNil(t, target, "expected an error containing %q, got: %v", tc.wantErrSubstr, errs)

					var wfErr *WorkflowScopedError
					require.ErrorAs(t, target, &wfErr, "error must be a *WorkflowScopedError")
					assert.Equal(t, tc.file, wfErr.File)
				})
			}
		})
	}
}
