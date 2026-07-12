package engine

import (
	"context"
	"errors"
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

func TestExecuteGraph_WorkflowTimeout(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"slow": &slowExecutor{delay: 5 * time.Second},
	})

	resolver := &singleOutputResolver{outputs: []string{"success"}}
	wf := WorkflowConfig{
		ID:      "timeout-workflow",
		Timeout: "100ms",
		Nodes: map[string]NodeConfig{
			"slow": {Type: "test.slow"},
		},
		Edges: []EdgeConfig{},
	}

	graph, err := Compile(wf, resolver)
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, graph.Timeout)

	execCtx := NewExecutionContext()
	start := time.Now()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, elapsed, 1*time.Second, "workflow should have timed out well before 1s")
}

func TestExecuteGraph_NoTimeoutStillWorks(t *testing.T) {
	var order []string
	mu := &sync.Mutex{}
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "a"},
	})

	wf := WorkflowConfig{
		ID: "no-timeout",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"},
		},
		Edges: []EdgeConfig{},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), graph.Timeout)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)
	assert.Equal(t, []string{"a"}, order)
}

func TestCompile_InvalidTimeout(t *testing.T) {
	wf := WorkflowConfig{
		ID:      "bad-timeout",
		Timeout: "not-a-duration",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"},
		},
		Edges: []EdgeConfig{},
	}

	_, err := Compile(wf, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid workflow timeout")
}

func TestExecuteGraph_WorkflowStartedTraceEvent(t *testing.T) {
	var order []string
	mu := &sync.Mutex{}
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "a"},
	})

	wf := WorkflowConfig{
		ID: "trace-started",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"},
		},
		Edges: []EdgeConfig{},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	var capturedEvents []struct {
		eventType string
		data      any
	}
	cb := func(eventType, nodeID, nodeType, output, errMsg string, data any) {
		capturedEvents = append(capturedEvents, struct {
			eventType string
			data      any
		}{eventType: eventType, data: data})
	}

	inputData := map[string]any{"user": "alice", "action": "login"}
	authData := &api.AuthData{
		UserID: "user-42",
		Roles:  []string{"admin", "editor"},
		Claims: map[string]any{"tenant": "acme"},
	}

	execCtx := NewExecutionContext(
		WithInput(inputData),
		WithAuth(authData),
		WithTraceCallback(cb),
	)
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	// Find the workflow:started event
	var startedEvent any
	for _, ev := range capturedEvents {
		if ev.eventType == "workflow:started" {
			startedEvent = ev.data
			break
		}
	}
	require.NotNil(t, startedEvent, "workflow:started event not emitted")

	payload, ok := startedEvent.(map[string]any)
	require.True(t, ok, "workflow:started data must be map[string]any")

	// Verify input is present
	assert.Equal(t, inputData, payload["input"])

	// Verify auth is present and correctly shaped
	authPayload, ok := payload["auth"].(map[string]any)
	require.True(t, ok, "auth must be map[string]any")
	assert.Equal(t, "user-42", authPayload["user_id"])
	assert.Equal(t, []string{"admin", "editor"}, authPayload["roles"])
	assert.Equal(t, map[string]any{"tenant": "acme"}, authPayload["claims"])
}

func TestExecuteGraph_WorkflowStartedTraceEvent_NoAuth(t *testing.T) {
	var order []string
	mu := &sync.Mutex{}
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "a"},
	})

	wf := WorkflowConfig{
		ID: "trace-started-noauth",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"},
		},
		Edges: []EdgeConfig{},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	var startedData any
	cb := func(eventType, nodeID, nodeType, output, errMsg string, data any) {
		if eventType == "workflow:started" {
			startedData = data
		}
	}

	execCtx := NewExecutionContext(
		WithInput(map[string]any{"key": "value"}),
		WithTraceCallback(cb),
	)
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	require.NotNil(t, startedData)
	payload := startedData.(map[string]any)
	assert.Equal(t, map[string]any{"key": "value"}, payload["input"])
	assert.Nil(t, payload["auth"])
}

// errExecutor returns a fixed output/data/error for testing error paths.
type errExecutor struct {
	output string
	data   any
	err    error
}

func (e *errExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *errExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return e.output, e.data, e.err
}

func TestExecuteGraph_ParallelMixedErrorTypes_NoPanic(t *testing.T) {
	// Branch "a" returns a Go error → dispatch wraps it in *engine.NodeExecutionError.
	// Branch "b" emits output "error" with no error edge → executor builds *errors.errorString.
	// Two distinct concrete error types race to record firstErr.
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a": &errExecutor{err: errors.New("branch a failed")},
		"b": &errExecutor{output: "error"},
	})
	wf := WorkflowConfig{
		ID: "mixed",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"},
			"b": {Type: "test.b"},
		},
		// two entry nodes → run in parallel; "b" has no error edge
	}
	// "test.a" declares no "error" output, so its Go error surfaces as
	// *engine.NodeExecutionError from dispatch. "test.b" declares "error" but
	// has no error edge, so the executor builds a plain *errors.errorString
	// via fmt.Errorf. The two concrete error types race to record firstErr.
	resolver := &mapResolver{
		types: map[string][]string{
			"test.a": {"success"},
			"test.b": {"success", "error"},
		},
	}
	graph, err := Compile(wf, resolver)
	require.NoError(t, err)

	// Run repeatedly to make the concurrent record deterministic.
	for i := 0; i < 20; i++ {
		execCtx := NewExecutionContext(WithWorkflowID("mixed"))
		gerr := ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
		require.Error(t, gerr) // a failure is expected; a PANIC (process crash) is not
	}
}

// singleOutputResolver returns a fixed set of outputs for all types.
type singleOutputResolver struct {
	outputs []string
}

func (r *singleOutputResolver) OutputsForType(string) ([]string, bool) {
	return r.outputs, true
}

// ctxIgnoringExecutor sleeps for a fixed duration without observing ctx
// cancellation at all (unlike slowExecutor, which selects on ctx.Done() and
// surfaces context.DeadlineExceeded as its own node error). It simulates a
// node whose underlying work (e.g. blocking I/O) doesn't plumb the context,
// so the *node* never records a failure — only the graph-level deadline
// does. This isolates the engine-2 regression: if ExecuteGraph decided
// success/failure solely from per-node errors, a workflow like this would
// report success even though "next" never ran.
type ctxIgnoringExecutor struct {
	delay time.Duration
}

func (e *ctxIgnoringExecutor) Outputs() []string { return []string{"success"} }
func (e *ctxIgnoringExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	time.Sleep(e.delay)
	return "success", map[string]any{"done": true}, nil
}

func TestExecuteGraph_Timeout_ReturnsTimeoutError(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"slow": &ctxIgnoringExecutor{delay: 150 * time.Millisecond},
		"next": &mockPassExecutor{},
	})
	wf := WorkflowConfig{
		ID:      "tmo",
		Timeout: "50ms",
		Nodes:   map[string]NodeConfig{"slow": {Type: "test.slow"}, "next": {Type: "test.next"}},
		Edges:   []EdgeConfig{{From: "slow", To: "next"}},
	}
	graph, err := Compile(wf, nil)
	require.NoError(t, err)
	execCtx := NewExecutionContext(WithWorkflowID("tmo"))
	gerr := ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)

	var toErr *api.TimeoutError
	require.ErrorAs(t, gerr, &toErr, "timeout must return *api.TimeoutError, not nil")
	_, ran := execCtx.GetOutput("next")
	require.False(t, ran, "downstream node must not be reported as run after timeout")
}

// #272: a non-deadline cancellation must surface as the wrapped "aborted"
// error, not a TimeoutError and not success.
func TestExecuteGraph_ParentCancel_ReturnsAbortedError(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"slow": &ctxIgnoringExecutor{delay: 150 * time.Millisecond},
		"next": &mockPassExecutor{},
	})
	wf := WorkflowConfig{
		ID:    "cancel",
		Nodes: map[string]NodeConfig{"slow": {Type: "test.slow"}, "next": {Type: "test.next"}},
		Edges: []EdgeConfig{{From: "slow", To: "next"}},
	}
	graph, err := Compile(wf, nil)
	require.NoError(t, err)
	execCtx := NewExecutionContext(WithWorkflowID("cancel"))

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	gerr := ExecuteGraph(ctx, graph, execCtx, svcReg, nodeReg)

	require.Error(t, gerr)
	var toErr *api.TimeoutError
	require.False(t, errors.As(gerr, &toErr), "plain cancel must not map to TimeoutError")
	assert.Contains(t, gerr.Error(), "aborted")
	assert.ErrorIs(t, gerr, context.Canceled)
}

// #273: a parent deadline propagating into a graph with no own timeout must
// report the budget the child actually had, not "after 0s".
func TestExecuteGraph_InheritedDeadline_ReportsBudget(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"slow": &ctxIgnoringExecutor{delay: 300 * time.Millisecond},
		"next": &mockPassExecutor{},
	})
	wf := WorkflowConfig{
		ID:    "inherit",
		Nodes: map[string]NodeConfig{"slow": {Type: "test.slow"}, "next": {Type: "test.next"}},
		Edges: []EdgeConfig{{From: "slow", To: "next"}},
	}
	graph, err := Compile(wf, nil)
	require.NoError(t, err)
	execCtx := NewExecutionContext(WithWorkflowID("inherit"))

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	gerr := ExecuteGraph(ctx, graph, execCtx, svcReg, nodeReg)

	var toErr *api.TimeoutError
	require.ErrorAs(t, gerr, &toErr)
	assert.Greater(t, toErr.Duration, time.Duration(0), "must carry the inherited budget")
	assert.LessOrEqual(t, toErr.Duration, 150*time.Millisecond, "budget ≈ the parent's 100ms deadline")
	assert.NotContains(t, gerr.Error(), "after 0s")
}

// condExecutor emits a chosen output so one downstream leg is never reached.
type condExecutor struct{ out string }

func (c *condExecutor) Outputs() []string { return []string{"go", "skip"} }
func (c *condExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return c.out, nil, nil
}

// twoOutResolver reports that "test.cond" has outputs go/skip; others success/error.
type twoOutResolver struct{}

func (twoOutResolver) OutputsForType(nodeType string) ([]string, bool) {
	if nodeType == "test.cond" {
		return []string{"go", "skip"}, true
	}
	return []string{"success", "error"}, true
}

func TestExecuteGraph_StarvedANDJoin_Errors(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a":    &mockPassExecutor{},
		"cond": &condExecutor{out: "skip"}, // "go" branch (→ b → j) never fires
		"b":    &mockPassExecutor{},
		"j":    &mockPassExecutor{},
	})
	wf := WorkflowConfig{
		ID: "starve",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"}, "cond": {Type: "test.cond"}, "b": {Type: "test.b"}, "j": {Type: "test.j"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "j"},                  // leg 1 fires
			{From: "cond", To: "b", Output: "go"}, // only via "go", which is not emitted
			{From: "b", To: "j"},                  // leg 2 never arrives
		},
	}
	graph, err := Compile(wf, twoOutResolver{})
	require.NoError(t, err)
	require.Equal(t, JoinAND, graph.JoinTypes["j"], "precondition: j is an AND-join")

	execCtx := NewExecutionContext(WithWorkflowID("starve"))
	gerr := ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, gerr)
	require.Contains(t, gerr.Error(), "AND-join \"j\"")
}

func TestExecuteGraph_ANDJoin_AllLegsFire_OK(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a": &mockPassExecutor{}, "cond": &condExecutor{out: "go"}, "b": &mockPassExecutor{}, "j": &mockPassExecutor{},
	})
	wf := WorkflowConfig{
		ID:    "ok",
		Nodes: map[string]NodeConfig{"a": {Type: "test.a"}, "cond": {Type: "test.cond"}, "b": {Type: "test.b"}, "j": {Type: "test.j"}},
		Edges: []EdgeConfig{{From: "a", To: "j"}, {From: "cond", To: "b", Output: "go"}, {From: "b", To: "j"}},
	}
	graph, err := Compile(wf, twoOutResolver{})
	require.NoError(t, err)
	execCtx := NewExecutionContext(WithWorkflowID("ok"))
	require.NoError(t, ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
}
