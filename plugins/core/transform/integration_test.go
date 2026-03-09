package transform

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPassExecutor returns config as output data.
type mockPassExecutor struct{}

func (e *mockPassExecutor) Outputs() []string { return api.DefaultOutputs() }
func (e *mockPassExecutor) Execute(_ context.Context, _ api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	return "success", config, nil
}

type mockDescriptor struct{ name string }

func (d *mockDescriptor) Name() string                           { return d.name }
func (d *mockDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *mockDescriptor) ConfigSchema() map[string]any           { return nil }

type mockPlugin struct {
	name, prefix string
	nodes        []api.NodeRegistration
}

func (p *mockPlugin) Name() string                                     { return p.name }
func (p *mockPlugin) Prefix() string                                   { return p.prefix }
func (p *mockPlugin) Nodes() []api.NodeRegistration                    { return p.nodes }
func (p *mockPlugin) HasServices() bool                                { return false }
func (p *mockPlugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *mockPlugin) HealthCheck(service any) error                    { return nil }
func (p *mockPlugin) Shutdown(service any) error                       { return nil }

type transformResolver struct{}

func (r *transformResolver) OutputsForType(nodeType string) ([]string, bool) {
	switch nodeType {
	case "control.if":
		return []string{"then", "else", "error"}, true
	case "workflow.output":
		return []string{}, true
	default:
		return api.DefaultOutputs(), true
	}
}

func setupIntegration(t *testing.T) (*registry.NodeRegistry, *registry.ServiceRegistry) {
	t.Helper()

	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	mock := &mockPlugin{
		name:   "mock",
		prefix: "mock",
		nodes: []api.NodeRegistration{
			{Descriptor: &mockDescriptor{name: "pass"}, Factory: func(map[string]any) api.NodeExecutor { return &mockPassExecutor{} }},
		},
	}
	require.NoError(t, nodeReg.RegisterFromPlugin(mock))

	return nodeReg, registry.NewServiceRegistry()
}

func TestIntegration_ValidateSetMap(t *testing.T) {
	nodeReg, svcReg := setupIntegration(t)
	resolver := &transformResolver{}

	wf := engine.WorkflowConfig{
		ID: "validate-set-map",
		Nodes: map[string]engine.NodeConfig{
			"validate": {Type: "transform.validate", Config: map[string]any{
				"data": "{{ input }}",
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"items": map[string]any{"type": "array"},
					},
					"required": []any{"items"},
				},
			}},
			"set_total": {Type: "transform.set", Config: map[string]any{
				"fields": map[string]any{
					"items": "{{ input.items }}",
					"count": "{{ len(input.items) }}",
				},
			}},
			"map_names": {Type: "transform.map", Config: map[string]any{
				"collection": "{{ input.items }}",
				"expression": "{{ $item.name }}",
			}},
		},
		Edges: []engine.EdgeConfig{
			{From: "validate", To: "set_total"},
			{From: "set_total", To: "map_names"},
		},
	}

	graph, err := engine.Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{
			map[string]any{"name": "Alice"},
			map[string]any{"name": "Bob"},
		},
	}))

	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	data, ok := execCtx.GetOutput("map_names")
	assert.True(t, ok)
	assert.Equal(t, []any{"Alice", "Bob"}, data)
}

func TestIntegration_FilterMerge(t *testing.T) {
	nodeReg, svcReg := setupIntegration(t)
	resolver := &transformResolver{}

	wf := engine.WorkflowConfig{
		ID: "filter-merge",
		Nodes: map[string]engine.NodeConfig{
			"filter_adults": {Type: "transform.filter", Config: map[string]any{
				"collection": "{{ input.people }}",
				"expression": "{{ $item.age >= 18 }}",
			}},
			"filter_minors": {Type: "transform.filter", Config: map[string]any{
				"collection": "{{ input.people }}",
				"expression": "{{ $item.age < 18 }}",
			}},
			"merge_all": {Type: "transform.merge", Config: map[string]any{
				"mode":   "append",
				"inputs": []any{"{{ filter_adults }}", "{{ filter_minors }}"},
			}},
		},
		Edges: []engine.EdgeConfig{
			{From: "filter_adults", To: "merge_all"},
			{From: "filter_minors", To: "merge_all"},
		},
	}

	graph, err := engine.Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"people": []any{
			map[string]any{"name": "Alice", "age": 25},
			map[string]any{"name": "Bob", "age": 15},
			map[string]any{"name": "Carol", "age": 30},
		},
	}))

	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	data, ok := execCtx.GetOutput("merge_all")
	assert.True(t, ok)
	result := data.([]any)
	// Adults first, then minors
	assert.Len(t, result, 3)
}

func TestIntegration_ValidateFailureErrorPath(t *testing.T) {
	nodeReg, svcReg := setupIntegration(t)
	resolver := &transformResolver{}

	wf := engine.WorkflowConfig{
		ID: "validate-fail",
		Nodes: map[string]engine.NodeConfig{
			"validate": {Type: "transform.validate", Config: map[string]any{
				"data": "{{ input }}",
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"email": map[string]any{"type": "string"},
					},
					"required": []any{"email"},
				},
			}},
			"on_success": {Type: "mock.pass", Config: map[string]any{"result": "ok"}},
			"on_error":   {Type: "mock.pass", Config: map[string]any{"result": "failed"}},
		},
		Edges: []engine.EdgeConfig{
			{From: "validate", Output: "success", To: "on_success"},
			{From: "validate", Output: "error", To: "on_error"},
		},
	}

	graph, err := engine.Compile(wf, resolver)
	require.NoError(t, err)

	// Missing email field
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"name": "Alice"}))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	_, hasSuccess := execCtx.GetOutput("on_success")
	assert.False(t, hasSuccess)

	data, hasError := execCtx.GetOutput("on_error")
	assert.True(t, hasError)
	assert.Equal(t, "failed", data.(map[string]any)["result"])
}

func TestIntegration_ComplexPipeline(t *testing.T) {
	nodeReg, svcReg := setupIntegration(t)
	resolver := &transformResolver{}

	wf := engine.WorkflowConfig{
		ID: "complex-pipeline",
		Nodes: map[string]engine.NodeConfig{
			"filter": {Type: "transform.filter", Config: map[string]any{
				"collection": "{{ input.items }}",
				"expression": "{{ $item.active }}",
			}},
			"extract": {Type: "transform.map", Config: map[string]any{
				"collection": "{{ filter }}",
				"expression": "{{ $item.name }}",
			}},
			"output": {Type: "transform.set", Config: map[string]any{
				"fields": map[string]any{
					"names": "{{ extract }}",
				},
			}},
		},
		Edges: []engine.EdgeConfig{
			{From: "filter", To: "extract"},
			{From: "extract", To: "output"},
		},
	}

	graph, err := engine.Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{
			map[string]any{"name": "Alice", "active": true},
			map[string]any{"name": "Bob", "active": false},
			map[string]any{"name": "Carol", "active": true},
		},
	}))

	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	data, ok := execCtx.GetOutput("output")
	assert.True(t, ok)
	result := data.(map[string]any)
	assert.Equal(t, []any{"Alice", "Carol"}, result["names"])
}

func TestIntegration_DeleteAndSet(t *testing.T) {
	nodeReg, svcReg := setupIntegration(t)
	resolver := &transformResolver{}

	wf := engine.WorkflowConfig{
		ID: "delete-set",
		Nodes: map[string]engine.NodeConfig{
			"clean": {Type: "transform.delete", Config: map[string]any{
				"data":   "{{ input }}",
				"fields": []any{"password", "internal_id"},
			}},
			"enrich": {Type: "transform.set", Config: map[string]any{
				"fields": map[string]any{
					"name":    "{{ clean.name }}",
					"display": "{{ \"User: \" + clean.name }}",
				},
			}},
		},
		Edges: []engine.EdgeConfig{
			{From: "clean", To: "enrich"},
		},
	}

	graph, err := engine.Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"name":        "Alice",
		"password":    "secret",
		"internal_id": 42,
	}))

	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	data, ok := execCtx.GetOutput("enrich")
	assert.True(t, ok)
	result := data.(map[string]any)
	assert.Equal(t, "Alice", result["name"])
	assert.Equal(t, "User: Alice", result["display"])
}
