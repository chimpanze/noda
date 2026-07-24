package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chimpanze/noda/pkg/api"
)

// asymmetricDiamond builds the shape from issue #433:
//
//	cond --go----> work --> j
//	cond --skip----------> j   (direct edge, no intermediate node)
func asymmetricDiamond() WorkflowConfig {
	return WorkflowConfig{
		ID: "asym",
		Nodes: map[string]NodeConfig{
			"cond": {Type: "test.cond"}, "work": {Type: "test.work"}, "j": {Type: "test.j"},
		},
		Edges: []EdgeConfig{
			{From: "cond", Output: "go", To: "work"},
			{From: "work", To: "j"},
			{From: "cond", Output: "skip", To: "j"},
		},
	}
}

// The direct-edge branch is the one that failed unconditionally before the fix.
func TestExecuteGraph_AsymmetricDiamond_DirectEdgeBranch(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"cond": &condExecutor{out: "skip"}, "work": &mockPassExecutor{}, "j": &mockPassExecutor{},
	})
	graph, err := Compile(asymmetricDiamond(), twoOutResolver{})
	require.NoError(t, err)
	require.Equal(t, JoinOR, graph.JoinTypes["j"])

	execCtx := NewExecutionContext(WithWorkflowID("asym"))
	require.NoError(t, ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg),
		"direct-edge branch must complete; before the fix this failed as a starved AND-join")
}

// The indirect branch of the same graph was equally broken.
func TestExecuteGraph_AsymmetricDiamond_IndirectBranch(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"cond": &condExecutor{out: "go"}, "work": &mockPassExecutor{}, "j": &mockPassExecutor{},
	})
	graph, err := Compile(asymmetricDiamond(), twoOutResolver{})
	require.NoError(t, err)

	execCtx := NewExecutionContext(WithWorkflowID("asym"))
	require.NoError(t, ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
}

// Asymmetric diamond: one leg reaches the join through an intermediate node,
// the other leg is a direct edge from the conditional itself.
//
//	check --then--> work --> merge
//	check --else-----------> merge
func TestCompile_ConditionalORJoin_DirectEdgeLeg(t *testing.T) {
	wf := WorkflowConfig{
		ID: "asymmetric",
		Nodes: map[string]NodeConfig{
			"check": {Type: "control.if"},
			"work":  {Type: "mock.pass"},
			"merge": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "check", Output: "then", To: "work"},
			{From: "work", To: "merge"},
			{From: "check", Output: "else", To: "merge"},
		},
	}

	g, err := Compile(wf, &conditionalResolver{})
	require.NoError(t, err)

	assert.Equal(t, JoinOR, g.JoinTypes["merge"],
		"then/else are mutually exclusive, so merge must be an OR-join even when one leg is a direct edge")
}

// Mirror of the above with the direct edge on "then" instead of "else",
// guarding against an ordering-dependent fix.
func TestCompile_ConditionalORJoin_DirectEdgeLegMirrored(t *testing.T) {
	wf := WorkflowConfig{
		ID: "asymmetric-mirror",
		Nodes: map[string]NodeConfig{
			"check": {Type: "control.if"},
			"work":  {Type: "mock.pass"},
			"merge": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "check", Output: "then", To: "merge"},
			{From: "check", Output: "else", To: "work"},
			{From: "work", To: "merge"},
		},
	}

	g, err := Compile(wf, &conditionalResolver{})
	require.NoError(t, err)

	assert.Equal(t, JoinOR, g.JoinTypes["merge"])
}

// A genuine concurrent fan-out must stay an AND-join: both legs run on the
// same output, so they are not mutually exclusive.
func TestCompile_ParallelFanOutStaysAND(t *testing.T) {
	wf := WorkflowConfig{
		ID: "fanout",
		Nodes: map[string]NodeConfig{
			"start": {Type: "mock.pass"},
			"work":  {Type: "mock.pass"},
			"merge": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "start", To: "work"},
			{From: "work", To: "merge"},
			{From: "start", To: "merge"},
		},
	}

	g, err := Compile(wf, &conditionalResolver{})
	require.NoError(t, err)

	assert.Equal(t, JoinAND, g.JoinTypes["merge"],
		"both legs descend from the same output of start, so they run concurrently")
}
