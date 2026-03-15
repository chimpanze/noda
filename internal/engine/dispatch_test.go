package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPassExecutor always succeeds with configurable output.
type mockPassExecutor struct{}

func (e *mockPassExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *mockPassExecutor) Execute(_ context.Context, _ api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	return "success", config, nil
}

// mockPanicExecutor panics during execution.
type mockPanicExecutor struct{}

func (e *mockPanicExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *mockPanicExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	panic("unexpected nil pointer")
}

// mockFailExecutor always fails.
type mockFailExecutor struct{}

func (e *mockFailExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *mockFailExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return "", nil, fmt.Errorf("node failed")
}

func setupDispatchTest(t *testing.T) (*registry.NodeRegistry, *registry.PluginRegistry, *registry.ServiceRegistry) {
	t.Helper()
	plugins := registry.NewPluginRegistry()
	services := registry.NewServiceRegistry()
	nodes := registry.NewNodeRegistry()

	// Register mock plugin with pass and fail nodes
	mockPlugin := &testPlugin{
		name:   "mock",
		prefix: "mock",
		nodes: []api.NodeRegistration{
			{
				Descriptor: &testDescriptor{name: "pass", deps: nil},
				Factory:    func(map[string]any) api.NodeExecutor { return &mockPassExecutor{} },
			},
			{
				Descriptor: &testDescriptor{name: "fail", deps: nil},
				Factory:    func(map[string]any) api.NodeExecutor { return &mockFailExecutor{} },
			},
			{
				Descriptor: &testDescriptor{name: "panic", deps: nil},
				Factory:    func(map[string]any) api.NodeExecutor { return &mockPanicExecutor{} },
			},
		},
	}

	require.NoError(t, plugins.Register(mockPlugin))
	require.NoError(t, nodes.RegisterFromPlugin(mockPlugin))

	return nodes, plugins, services
}

func TestDispatchNode_Success(t *testing.T) {
	nodes, _, services := setupDispatchTest(t)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "step1",
		Type:    "mock.pass",
		Config:  map[string]any{"key": "value"},
		Outputs: []string{"success", "error"},
	}

	output, err := dispatchNode(context.Background(), node, execCtx, services, nodes)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	data, ok := execCtx.GetOutput("step1")
	assert.True(t, ok)
	assert.Equal(t, "value", data.(map[string]any)["key"])
}

func TestDispatchNode_FailWithErrorEdge(t *testing.T) {
	nodes, _, services := setupDispatchTest(t)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "step1",
		Type:    "mock.fail",
		Outputs: []string{"success", "error"},
	}

	output, err := dispatchNode(context.Background(), node, execCtx, services, nodes)
	require.NoError(t, err) // error is handled, not propagated
	assert.Equal(t, "error", output)

	data, ok := execCtx.GetOutput("step1")
	assert.True(t, ok)
	assert.Contains(t, data.(map[string]any)["error"], "node failed")
}

func TestDispatchNode_FailWithoutErrorEdge(t *testing.T) {
	nodes, _, services := setupDispatchTest(t)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "step1",
		Type:    "mock.fail",
		Outputs: []string{"success"}, // no error output
	}

	_, err := dispatchNode(context.Background(), node, execCtx, services, nodes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "step1")
}

func TestDispatchNode_AsAlias(t *testing.T) {
	nodes, _, services := setupDispatchTest(t)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "fetch-user-node",
		Type:    "mock.pass",
		As:      "user",
		Config:  map[string]any{"name": "Alice"},
		Outputs: []string{"success", "error"},
	}

	output, err := dispatchNode(context.Background(), node, execCtx, services, nodes)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	// Output stored under alias
	data, ok := execCtx.GetOutput("fetch-user-node")
	assert.True(t, ok)
	assert.Equal(t, "Alice", data.(map[string]any)["name"])
}

func TestDispatchNode_ContextCancellation(t *testing.T) {
	nodes, _, services := setupDispatchTest(t)
	execCtx := NewExecutionContext()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	node := &CompiledNode{
		ID:      "step1",
		Type:    "mock.pass",
		Outputs: []string{"success", "error"},
	}

	// The mock doesn't check context, but the dispatch should pass it through
	output, err := dispatchNode(ctx, node, execCtx, services, nodes)
	// Mock executor ignores context, so it still succeeds
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestDispatchNode_PanicRecovery(t *testing.T) {
	nodes, _, services := setupDispatchTest(t)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "step1",
		Type:    "mock.panic",
		Outputs: []string{"success", "error"},
	}

	output, err := dispatchNode(context.Background(), node, execCtx, services, nodes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "panicked")
	assert.Contains(t, err.Error(), "unexpected nil pointer")
	assert.Equal(t, "", output)
}

func TestDispatchNode_ReturnsNodeExecutionError(t *testing.T) {
	nodes, _, services := setupDispatchTest(t)
	execCtx := NewExecutionContext()

	// Set some outputs so OutputKeys returns them
	execCtx.SetOutput("step0", map[string]any{"val": 1})

	node := &CompiledNode{
		ID:      "step1",
		Type:    "mock.fail",
		Outputs: []string{"success"}, // no error output → fatal path
	}

	_, err := dispatchNode(context.Background(), node, execCtx, services, nodes)
	require.Error(t, err)

	var nodeErr *NodeExecutionError
	require.True(t, errors.As(err, &nodeErr))
	assert.Equal(t, "step1", nodeErr.NodeID)
	assert.Equal(t, "mock.fail", nodeErr.NodeType)
	assert.Contains(t, nodeErr.AvailableNodes, "step0")
	assert.Contains(t, nodeErr.Err.Error(), "node failed")
}

func TestDispatchNode_ErrorEdgeIncludesContext(t *testing.T) {
	nodes, _, services := setupDispatchTest(t)
	execCtx := NewExecutionContext()
	execCtx.SetOutput("prev", "data")

	node := &CompiledNode{
		ID:      "step1",
		Type:    "mock.fail",
		Outputs: []string{"success", "error"}, // has error output
	}

	output, err := dispatchNode(context.Background(), node, execCtx, services, nodes)
	require.NoError(t, err)
	assert.Equal(t, "error", output)

	data, ok := execCtx.GetOutput("step1")
	require.True(t, ok)
	errorData := data.(map[string]any)
	assert.Equal(t, "step1", errorData["node_id"])
	assert.Equal(t, "mock.fail", errorData["node_type"])
	assert.Contains(t, errorData["available_nodes"], "prev")
}

// test helpers for dispatch tests
type testPlugin struct {
	name   string
	prefix string
	nodes  []api.NodeRegistration
}

func (p *testPlugin) Name() string                                     { return p.name }
func (p *testPlugin) Prefix() string                                   { return p.prefix }
func (p *testPlugin) Nodes() []api.NodeRegistration                    { return p.nodes }
func (p *testPlugin) HasServices() bool                                { return false }
func (p *testPlugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *testPlugin) HealthCheck(service any) error                    { return nil }
func (p *testPlugin) Shutdown(service any) error                       { return nil }

type testDescriptor struct {
	name string
	deps map[string]api.ServiceDep
}

func (d *testDescriptor) Name() string                           { return d.name }
func (d *testDescriptor) Description() string                    { return "" }
func (d *testDescriptor) ServiceDeps() map[string]api.ServiceDep { return d.deps }
func (d *testDescriptor) ConfigSchema() map[string]any           { return nil }
func (d *testDescriptor) OutputDescriptions() map[string]string  { return nil }
