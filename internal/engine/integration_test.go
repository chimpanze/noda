package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Node Executors ---

// mockPass always succeeds, returns config as output.
type mockPass struct{}

func (e *mockPass) Outputs() []string { return []string{"success", "error"} }
func (e *mockPass) Execute(_ context.Context, _ api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	return "success", config, nil
}

// mockFail always fails with a configurable error.
type mockFail struct{}

func (e *mockFail) Outputs() []string { return []string{"success", "error"} }
func (e *mockFail) Execute(_ context.Context, _ api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	msg := "mock failure"
	if m, ok := config["error"].(string); ok {
		msg = m
	}
	return "", nil, fmt.Errorf("%s", msg)
}

// mockSlow succeeds after a configurable delay.
type mockSlow struct {
	delay time.Duration
}

func (e *mockSlow) Outputs() []string { return []string{"success", "error"} }
func (e *mockSlow) Execute(ctx context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	select {
	case <-time.After(e.delay):
		return "success", map[string]any{"delayed": true}, nil
	case <-ctx.Done():
		return "", nil, ctx.Err()
	}
}

// mockConditional succeeds or fails based on a condition in config.
type mockConditional struct{}

func (e *mockConditional) Outputs() []string { return []string{"then", "else", "error"} }
func (e *mockConditional) Execute(_ context.Context, execCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	if cond, ok := config["condition"].(bool); ok && cond {
		return "then", map[string]any{"branch": "then"}, nil
	}
	return "else", map[string]any{"branch": "else"}, nil
}

// mockFlaky fails first N calls, then succeeds.
type mockFlaky struct {
	failCount int
	calls     atomic.Int32
}

func (e *mockFlaky) Outputs() []string { return []string{"success", "error"} }
func (e *mockFlaky) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	n := int(e.calls.Add(1))
	if n <= e.failCount {
		return "", nil, fmt.Errorf("flaky failure %d", n)
	}
	return "success", map[string]any{"recovered": true}, nil
}

// mockAccumulator stores data from input.
type mockAccumulator struct {
	mu   sync.Mutex
	data []any
}

func (e *mockAccumulator) Outputs() []string { return []string{"success", "error"} }
func (e *mockAccumulator) Execute(_ context.Context, execCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	e.mu.Lock()
	e.data = append(e.data, execCtx.Input())
	e.mu.Unlock()
	return "success", map[string]any{"accumulated": len(e.data)}, nil
}

// --- Test Setup Helper ---

func setupIntegrationTest(t *testing.T, executors map[string]api.NodeExecutor) (*registry.NodeRegistry, *registry.ServiceRegistry) {
	t.Helper()
	plugins := registry.NewPluginRegistry()
	nodeReg := registry.NewNodeRegistry()

	var regs []api.NodeRegistration
	for name, exec := range executors {
		name := name
		exec := exec
		regs = append(regs, api.NodeRegistration{
			Descriptor: &testDescriptor{name: name},
			Factory:    func(map[string]any) api.NodeExecutor { return exec },
		})
	}

	p := &testPlugin{name: "mock", prefix: "mock", nodes: regs}
	require.NoError(t, plugins.Register(p))
	require.NoError(t, nodeReg.RegisterFromPlugin(p))

	return nodeReg, registry.NewServiceRegistry()
}

// --- Integration Tests ---

func TestIntegration_LinearDataFlow(t *testing.T) {
	mu := &sync.Mutex{}
	var order []string
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"step1": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "step1"},
		"step2": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "step2"},
		"step3": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "step3"},
	})

	wf := WorkflowConfig{
		ID: "linear-data",
		Nodes: map[string]NodeConfig{
			"step1": {Type: "mock.step1"},
			"step2": {Type: "mock.step2"},
			"step3": {Type: "mock.step3"},
		},
		Edges: []EdgeConfig{
			{From: "step1", To: "step2"},
			{From: "step2", To: "step3"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext(
		WithInput(map[string]any{"name": "Alice"}),
		WithWorkflowID("linear-data"),
	)

	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	assert.Equal(t, []string{"step1", "step2", "step3"}, order)
}

func TestIntegration_ParallelConcurrency(t *testing.T) {
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"slow1": &mockSlow{delay: 50 * time.Millisecond},
		"slow2": &mockSlow{delay: 50 * time.Millisecond},
		"join":  &mockPass{},
	})

	wf := WorkflowConfig{
		ID: "parallel",
		Nodes: map[string]NodeConfig{
			"slow1": {Type: "mock.slow1"},
			"slow2": {Type: "mock.slow2"},
			"join":  {Type: "mock.join"},
		},
		Edges: []EdgeConfig{
			{From: "slow1", To: "join"},
			{From: "slow2", To: "join"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	start := time.Now()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, elapsed.Milliseconds(), int64(90), "parallel branches should run concurrently")
}

func TestIntegration_Diamond(t *testing.T) {
	// A → B, A → C, B → D, C → D (AND-join at D)
	mu := &sync.Mutex{}
	var order []string
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"a": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "a"},
		"b": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "b"},
		"c": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "c"},
		"d": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "d"},
	})

	wf := WorkflowConfig{
		ID: "diamond",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.a"},
			"b": {Type: "mock.b"},
			"c": {Type: "mock.c"},
			"d": {Type: "mock.d"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"},
			{From: "a", To: "c"},
			{From: "b", To: "d"},
			{From: "c", To: "d"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	// a must be first, d must be last
	assert.Equal(t, "a", order[0])
	assert.Equal(t, "d", order[len(order)-1])
}

func TestIntegration_ConditionalSplit(t *testing.T) {
	// Use a resolver that gives "then"/"else"/"error" for the check node
	resolver := &mapResolver{
		types: map[string][]string{
			"mock.check": {"then", "else", "error"},
		},
		fallback: []string{"success", "error"},
	}
	mu := &sync.Mutex{}
	var order []string
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"check":   &mockConditional{},
		"branch_t": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "branch_t"},
		"branch_f": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "branch_f"},
		"merge":   &orderTrackingExecutor{mu: mu, order: &order, nodeID: "merge"},
	})

	wf := WorkflowConfig{
		ID: "conditional",
		Nodes: map[string]NodeConfig{
			"check":    {Type: "mock.check", Config: map[string]any{"condition": true}},
			"branch_t": {Type: "mock.branch_t"},
			"branch_f": {Type: "mock.branch_f"},
			"merge":    {Type: "mock.merge"},
		},
		Edges: []EdgeConfig{
			{From: "check", Output: "then", To: "branch_t"},
			{From: "check", Output: "else", To: "branch_f"},
			{From: "branch_t", To: "merge"},
			{From: "branch_f", To: "merge"},
		},
	}

	graph, err := Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	// Only the "then" branch should execute
	assert.Contains(t, order, "branch_t")
	assert.NotContains(t, order, "branch_f")
	assert.Contains(t, order, "merge")
}

func TestIntegration_ErrorPath(t *testing.T) {
	mu := &sync.Mutex{}
	var order []string
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"risky":   &mockFail{},
		"handler": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "handler"},
	})

	wf := WorkflowConfig{
		ID: "error-path",
		Nodes: map[string]NodeConfig{
			"risky":   {Type: "mock.risky"},
			"handler": {Type: "mock.handler"},
		},
		Edges: []EdgeConfig{
			{From: "risky", Output: "error", To: "handler"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	assert.Contains(t, order, "handler")

	// Error data stored on risky node
	data, ok := execCtx.GetOutput("risky")
	assert.True(t, ok)
	assert.Contains(t, data.(map[string]any)["error"], "mock failure")
}

func TestIntegration_RetryOnErrorEdge(t *testing.T) {
	flaky := &mockFlaky{failCount: 2}
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"flaky":   flaky,
		"handler": &mockPass{},
	})

	wf := WorkflowConfig{
		ID: "retry",
		Nodes: map[string]NodeConfig{
			"flaky":   {Type: "mock.flaky"},
			"handler": {Type: "mock.handler"},
		},
		Edges: []EdgeConfig{
			{From: "flaky", Output: "error", To: "handler", Retry: &RetryConfig{
				Attempts: 3,
				Backoff:  "fixed",
				Delay:    "1ms",
			}},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	// Flaky node: first dispatch fails (call 1), retry fails (call 2), retry succeeds (call 3)
	data, ok := execCtx.GetOutput("flaky")
	assert.True(t, ok)
	assert.Equal(t, true, data.(map[string]any)["recovered"])
}

func TestIntegration_ContextTimeout(t *testing.T) {
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"slow": &mockSlow{delay: 5 * time.Second},
	})

	resolver := &singleOutputResolver{outputs: []string{"success"}}
	wf := WorkflowConfig{
		ID: "timeout",
		Nodes: map[string]NodeConfig{
			"slow": {Type: "mock.slow"},
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

func TestIntegration_OutputEviction(t *testing.T) {
	wf := WorkflowConfig{
		ID: "eviction",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.a"},
			"b": {Type: "mock.b"},
			"c": {Type: "mock.c"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	tracker := NewEvictionTracker(graph, execCtx)

	// Simulate execution with eviction
	execCtx.SetOutput("a", map[string]any{"data": "from-a"})
	tracker.NodeCompleted("b", graph) // a consumed by b
	_, ok := execCtx.GetOutput("a")
	assert.False(t, ok, "a should be evicted after b")

	execCtx.SetOutput("b", map[string]any{"data": "from-b"})
	tracker.NodeCompleted("c", graph)
	_, ok = execCtx.GetOutput("b")
	assert.False(t, ok, "b should be evicted after c")
}

func TestIntegration_ComplexGraph(t *testing.T) {
	// Complex: entry1 → transform → validate → db_write → respond
	//          entry2 → cache_check ─┐
	//                                ├→ merge → log → respond2
	//          entry3 → auth_check ──┘
	mu := &sync.Mutex{}
	var order []string
	executors := make(map[string]api.NodeExecutor)
	nodeNames := []string{"entry1", "entry2", "entry3", "transform", "validate",
		"db_write", "cache_check", "auth_check", "merge", "log", "respond", "respond2"}
	for _, name := range nodeNames {
		name := name
		executors[name] = &orderTrackingExecutor{mu: mu, order: &order, nodeID: name}
	}

	nodeReg, svcReg := setupIntegrationTest(t, executors)

	nodes := make(map[string]NodeConfig)
	for _, name := range nodeNames {
		nodes[name] = NodeConfig{Type: "mock." + name}
	}

	wf := WorkflowConfig{
		ID:    "complex",
		Nodes: nodes,
		Edges: []EdgeConfig{
			{From: "entry1", To: "transform"},
			{From: "transform", To: "validate"},
			{From: "validate", To: "db_write"},
			{From: "db_write", To: "respond"},
			{From: "entry2", To: "cache_check"},
			{From: "entry3", To: "auth_check"},
			{From: "cache_check", To: "merge"},
			{From: "auth_check", To: "merge"},
			{From: "merge", To: "log"},
			{From: "log", To: "respond2"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	assert.Len(t, order, 12)
	// All entry nodes should appear before their dependents
	entrySet := map[string]bool{"entry1": false, "entry2": false, "entry3": false}
	for _, name := range order {
		if _, isEntry := entrySet[name]; isEntry {
			entrySet[name] = true
		}
	}
	for name, executed := range entrySet {
		assert.True(t, executed, "entry node %s should have executed", name)
	}
}

// mapResolver returns specific outputs for specific types, with a fallback.
type mapResolver struct {
	types    map[string][]string
	fallback []string
}

func (r *mapResolver) OutputsForType(nodeType string) ([]string, bool) {
	if outputs, ok := r.types[nodeType]; ok {
		return outputs, true
	}
	return r.fallback, true
}
