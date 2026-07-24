package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/chimpanze/noda/pkg/api"
)

// mixedJoin builds a join fed by BOTH a mutually-exclusive conditional pair
// and an independent, always-firing leg:
//
//	cond --go----> work --> j
//	cond --skip----------> j    (mutually exclusive with the leg above)
//	side ----------------> j    (concurrent with whichever of the two runs)
//
// Exactly two of the three legs arrive on any request, so a plain AND-join
// (waits for 3) starves and a plain OR-join (fires on the 1st) can fire
// before "side" has produced its output.
func mixedJoin() WorkflowConfig {
	return WorkflowConfig{
		ID: "mixed",
		Nodes: map[string]NodeConfig{
			"cond": {Type: "test.cond"}, "work": {Type: "test.work"},
			"side": {Type: "test.side"}, "j": {Type: "test.j"},
		},
		Edges: []EdgeConfig{
			{From: "cond", Output: "go", To: "work"},
			{From: "work", To: "j"},
			{From: "cond", Output: "skip", To: "j"},
			{From: "side", To: "j"},
		},
	}
}

// The compiler must see three legs but only two groups.
func TestCompile_MixedJoin_Grouping(t *testing.T) {
	g, err := Compile(mixedJoin(), twoOutResolver{})
	require.NoError(t, err)

	require.Equal(t, JoinMixed, g.JoinTypes["j"])
	require.Equal(t, 3, g.DepCount["j"], "three inbound edges")
	require.Equal(t, 2, g.JoinGroupCount["j"], "but only two groups must deliver")

	groups := g.JoinGroups["j"]
	require.Equal(t, groups["work"], groups["cond"], "the conditional legs are one exclusive group")
	require.NotEqual(t, groups["side"], groups["work"], "side runs concurrently with them")
}

// slowOrderExecutor records execution order and can stall first, so a join
// firing too early is observable as an out-of-order entry.
type slowOrderExecutor struct {
	mu     *sync.Mutex
	order  *[]string
	nodeID string
	delay  time.Duration
}

func (e *slowOrderExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *slowOrderExecutor) Execute(ctx context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	if e.delay > 0 {
		select {
		case <-time.After(e.delay):
		case <-ctx.Done():
			return "error", nil, ctx.Err()
		}
	}
	e.mu.Lock()
	*e.order = append(*e.order, e.nodeID)
	e.mu.Unlock()
	return "success", map[string]any{"node": e.nodeID}, nil
}

// A mixed join must not fire early: "side" is concurrent with the conditional
// pair, so the join has to wait for it even though a conditional leg has
// already arrived. "side" is deliberately slow — if the join fired on the
// conditional leg alone, "j" would be recorded before "side".
func TestExecuteGraph_MixedJoin_WaitsForConcurrentLeg(t *testing.T) {
	var order []string
	mu := &sync.Mutex{}
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"cond": &condExecutor{out: "skip"},
		"work": &mockPassExecutor{},
		"side": &slowOrderExecutor{mu: mu, order: &order, nodeID: "side", delay: 50 * time.Millisecond},
		"j":    &slowOrderExecutor{mu: mu, order: &order, nodeID: "j"},
	})
	graph, err := Compile(mixedJoin(), twoOutResolver{})
	require.NoError(t, err)

	execCtx := NewExecutionContext(WithWorkflowID("mixed"))
	require.NoError(t, ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))

	require.Equal(t, []string{"side", "j"}, order,
		"join must wait for the concurrent leg and run exactly once")
}

// outExecutor fires a chosen output.
type outExecutor struct{ out string }

func (e *outExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *outExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return e.out, map[string]any{"out": e.out}, nil
}

// The "graceful fallback in a parallel branch" pattern from
// docs/04-guides/workflow-patterns.md: two branches run in parallel and one of
// them has an error edge to a fallback node. "respond" has three inbound legs,
// but fetch_prefs and default_prefs are mutually exclusive, so only two ever
// arrive.
func fallbackPatternWorkflow() WorkflowConfig {
	return WorkflowConfig{
		ID: "parallel-with-errors",
		Nodes: map[string]NodeConfig{
			"fetch_user":     {Type: "test.fetch_user"},
			"fetch_prefs":    {Type: "test.fetch_prefs"},
			"default_prefs":  {Type: "test.default_prefs"},
			"respond":        {Type: "test.respond"},
			"error_response": {Type: "test.error_response"},
		},
		Edges: []EdgeConfig{
			{From: "fetch_user", To: "respond", Output: "success"},
			{From: "fetch_user", To: "error_response", Output: "error"},
			{From: "fetch_prefs", To: "respond", Output: "success"},
			{From: "fetch_prefs", To: "default_prefs", Output: "error"},
			{From: "default_prefs", To: "respond", Output: "success"},
		},
	}
}

func TestCompile_DocumentedFallbackPattern_IsMixedJoin(t *testing.T) {
	g, err := Compile(fallbackPatternWorkflow(), nil)
	require.NoError(t, err)

	require.Equal(t, 3, g.DepCount["respond"], "three inbound edges")
	require.Equal(t, 2, g.JoinGroupCount["respond"], "but only two legs can ever arrive")

	groups := g.JoinGroups["respond"]
	require.Equal(t, groups["fetch_prefs"], groups["default_prefs"],
		"the cache-hit and fallback legs are mutually exclusive")
	require.NotEqual(t, groups["fetch_user"], groups["fetch_prefs"],
		"fetch_user runs concurrently with the prefs branch")
}

// The cache-miss path is the one that was broken: fetch_prefs fires "error",
// so respond is fed by fetch_user + default_prefs — 2 of its 3 legs.
func TestExecuteGraph_DocumentedFallbackPattern_CacheMiss(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"fetch_user":     &outExecutor{out: "success"},
		"fetch_prefs":    &outExecutor{out: "error"}, // cache miss → fallback
		"default_prefs":  &mockPassExecutor{},
		"respond":        &mockPassExecutor{},
		"error_response": &mockPassExecutor{},
	})
	graph, err := Compile(fallbackPatternWorkflow(), nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext(WithWorkflowID("parallel-with-errors"))
	require.NoError(t, ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
}

// The cache-hit path: fetch_user + fetch_prefs arrive, default_prefs never runs.
func TestExecuteGraph_DocumentedFallbackPattern_CacheHit(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"fetch_user":     &outExecutor{out: "success"},
		"fetch_prefs":    &outExecutor{out: "success"},
		"default_prefs":  &mockPassExecutor{},
		"respond":        &mockPassExecutor{},
		"error_response": &mockPassExecutor{},
	})
	graph, err := Compile(fallbackPatternWorkflow(), nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext(WithWorkflowID("parallel-with-errors"))
	require.NoError(t, ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
}

func TestExecuteGraph_MixedJoin_DirectEdgeBranch(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"cond": &condExecutor{out: "skip"}, "work": &mockPassExecutor{},
		"side": &mockPassExecutor{}, "j": &mockPassExecutor{},
	})
	graph, err := Compile(mixedJoin(), twoOutResolver{})
	require.NoError(t, err)

	execCtx := NewExecutionContext(WithWorkflowID("mixed"))
	require.NoError(t, ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
}

func TestExecuteGraph_MixedJoin_IndirectBranch(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"cond": &condExecutor{out: "go"}, "work": &mockPassExecutor{},
		"side": &mockPassExecutor{}, "j": &mockPassExecutor{},
	})
	graph, err := Compile(mixedJoin(), twoOutResolver{})
	require.NoError(t, err)

	execCtx := NewExecutionContext(WithWorkflowID("mixed"))
	require.NoError(t, ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
}
