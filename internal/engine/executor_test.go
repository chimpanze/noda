package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowExecutor sleeps for a configured duration.
type slowExecutor struct {
	delay time.Duration
}

func (e *slowExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *slowExecutor) Execute(ctx context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	select {
	case <-time.After(e.delay):
		return "success", map[string]any{"delayed": true}, nil
	case <-ctx.Done():
		return "", nil, ctx.Err()
	}
}

// orderTrackingExecutor records execution order (thread-safe).
type orderTrackingExecutor struct {
	mu     *sync.Mutex
	order  *[]string
	nodeID string
}

func (e *orderTrackingExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *orderTrackingExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	e.mu.Lock()
	*e.order = append(*e.order, e.nodeID)
	e.mu.Unlock()
	return "success", map[string]any{"node": e.nodeID}, nil
}

func setupExecutorTest(t *testing.T, executors map[string]api.NodeExecutor) (*registry.NodeRegistry, *registry.ServiceRegistry) {
	t.Helper()
	plugins := registry.NewPluginRegistry()
	nodeReg := registry.NewNodeRegistry()

	var nodeRegs []api.NodeRegistration
	for name, exec := range executors {
		name := name
		exec := exec
		nodeRegs = append(nodeRegs, api.NodeRegistration{
			Descriptor: &testDescriptor{name: name},
			Factory:    func(map[string]any) api.NodeExecutor { return exec },
		})
	}

	p := &testPlugin{name: "test", prefix: "test", nodes: nodeRegs}
	require.NoError(t, plugins.Register(p))
	require.NoError(t, nodeReg.RegisterFromPlugin(p))

	return nodeReg, registry.NewServiceRegistry()
}

func TestExecuteGraph_LinearWorkflow(t *testing.T) {
	var order []string
	mu := &sync.Mutex{}
	executors := map[string]api.NodeExecutor{
		"a": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "a"},
		"b": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "b"},
		"c": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "c"},
	}
	nodeReg, svcReg := setupExecutorTest(t, executors)

	wf := WorkflowConfig{
		ID: "linear",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"},
			"b": {Type: "test.b"},
			"c": {Type: "test.c"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext(WithWorkflowID("linear"))
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	assert.Equal(t, []string{"a", "b", "c"}, order)
}

func TestExecuteGraph_ParallelBranches(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a": &slowExecutor{delay: 50 * time.Millisecond},
		"b": &slowExecutor{delay: 50 * time.Millisecond},
		"c": &mockPassExecutor{},
	})

	wf := WorkflowConfig{
		ID: "parallel",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"},
			"b": {Type: "test.b"},
			"c": {Type: "test.c"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "c"},
			{From: "b", To: "c"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	start := time.Now()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	elapsed := time.Since(start)

	require.NoError(t, err)
	// Parallel: should be ~50ms, not ~100ms
	assert.Less(t, elapsed.Milliseconds(), int64(90))
}

func TestExecuteGraph_ANDJoin(t *testing.T) {
	var order []string
	mu := &sync.Mutex{}
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "a"},
		"b": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "b"},
		"c": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "c"},
	})

	wf := WorkflowConfig{
		ID: "and-join",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"},
			"b": {Type: "test.b"},
			"c": {Type: "test.c"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "c"},
			{From: "b", To: "c"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	// c must be last
	assert.Equal(t, "c", order[len(order)-1])
}

func TestExecuteGraph_ContextCancellation(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"slow": &slowExecutor{delay: 5 * time.Second},
	})

	// Use resolver that only gives "success" output (so context error propagates)
	resolver := &singleOutputResolver{outputs: []string{"success"}}
	wf := WorkflowConfig{
		ID: "timeout",
		Nodes: map[string]NodeConfig{
			"slow": {Type: "test.slow"},
		},
		Edges: []EdgeConfig{},
	}

	graph, err := Compile(wf, resolver)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	execCtx := NewExecutionContext()
	err = ExecuteGraph(ctx, graph, execCtx, svcReg, nodeReg)
	require.Error(t, err)
}

func TestExecuteGraph_UnhandledError(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"fail": &mockFailExecutor{},
		"next": &mockPassExecutor{},
	})

	wf := WorkflowConfig{
		ID: "error",
		Nodes: map[string]NodeConfig{
			"fail": {Type: "test.fail"},
			"next": {Type: "test.next"},
		},
		Edges: []EdgeConfig{
			{From: "fail", To: "next"},
		},
	}

	// Use resolver that only gives "success" output (no error edge)
	resolver := &singleOutputResolver{outputs: []string{"success"}}
	graph, err := Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fail")
}

func TestExecuteGraph_ErrorEdgeFollowed(t *testing.T) {
	var order []string
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"fail":    &mockFailExecutor{},
		"handler": &orderTrackingExecutor{mu: &sync.Mutex{}, order: &order, nodeID: "handler"},
	})

	wf := WorkflowConfig{
		ID: "error-edge",
		Nodes: map[string]NodeConfig{
			"fail":    {Type: "test.fail"},
			"handler": {Type: "test.handler"},
		},
		Edges: []EdgeConfig{
			{From: "fail", Output: "error", To: "handler"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)
	assert.Contains(t, order, "handler")
}

// singleOutputResolver returns a fixed set of outputs for all types.
type singleOutputResolver struct {
	outputs []string
}

func (r *singleOutputResolver) OutputsForType(string) ([]string, bool) {
	return r.outputs, true
}
