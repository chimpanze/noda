package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// conditionalResolver returns different outputs for control.if nodes.
type conditionalResolver struct{}

func (r *conditionalResolver) OutputsForType(nodeType string) ([]string, bool) {
	if nodeType == "control.if" {
		return []string{"then", "else", "error"}, true
	}
	return []string{"success", "error"}, true
}

func TestCompile_LinearWorkflow(t *testing.T) {
	wf := WorkflowConfig{
		ID: "linear",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass"},
			"c": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		},
	}

	g, err := Compile(wf, nil)
	require.NoError(t, err)

	assert.Equal(t, []string{"a"}, g.EntryNodes)
	assert.Contains(t, g.TerminalNodes, "c")
	assert.Equal(t, 0, g.DepCount["a"])
	assert.Equal(t, 1, g.DepCount["b"])
	assert.Equal(t, 1, g.DepCount["c"])
}

func TestCompile_ParallelANDJoin(t *testing.T) {
	wf := WorkflowConfig{
		ID: "parallel",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass"},
			"c": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "c"},
			{From: "b", To: "c"},
		},
	}

	g, err := Compile(wf, nil)
	require.NoError(t, err)

	assert.Len(t, g.EntryNodes, 2)
	assert.Contains(t, g.EntryNodes, "a")
	assert.Contains(t, g.EntryNodes, "b")
	assert.Equal(t, 2, g.DepCount["c"])
	assert.Equal(t, JoinAND, g.JoinTypes["c"])
}

func TestCompile_ConditionalORJoin(t *testing.T) {
	wf := WorkflowConfig{
		ID: "conditional",
		Nodes: map[string]NodeConfig{
			"check":  {Type: "control.if"},
			"branch_then": {Type: "mock.pass"},
			"branch_else": {Type: "mock.pass"},
			"merge":  {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "check", Output: "then", To: "branch_then"},
			{From: "check", Output: "else", To: "branch_else"},
			{From: "branch_then", To: "merge"},
			{From: "branch_else", To: "merge"},
		},
	}

	g, err := Compile(wf, &conditionalResolver{})
	require.NoError(t, err)

	assert.Equal(t, JoinOR, g.JoinTypes["merge"])
}

func TestCompile_CycleDetection(t *testing.T) {
	wf := WorkflowConfig{
		ID: "cycle",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"},
			{From: "b", To: "a"},
		},
	}

	_, err := Compile(wf, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestCompile_InvalidEdge_UnknownNode(t *testing.T) {
	wf := WorkflowConfig{
		ID: "bad-edge",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "nonexistent"},
		},
	}

	_, err := Compile(wf, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestCompile_InvalidEdge_UnknownSource(t *testing.T) {
	wf := WorkflowConfig{
		ID: "bad-source",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "ghost", To: "a"},
		},
	}

	_, err := Compile(wf, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestCompile_InvalidOutputName(t *testing.T) {
	wf := WorkflowConfig{
		ID: "bad-output",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", Output: "nonexistent_output", To: "b"},
		},
	}

	_, err := Compile(wf, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent_output")
}

func TestCompile_EntryNodes(t *testing.T) {
	wf := WorkflowConfig{
		ID: "multi-entry",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass"},
			"c": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "c"},
			{From: "b", To: "c"},
		},
	}

	g, err := Compile(wf, nil)
	require.NoError(t, err)

	assert.Len(t, g.EntryNodes, 2)
	assert.Contains(t, g.EntryNodes, "a")
	assert.Contains(t, g.EntryNodes, "b")
}

func TestCompile_TerminalNodes(t *testing.T) {
	wf := WorkflowConfig{
		ID: "terminals",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass"},
			"c": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"},
			{From: "a", To: "c"},
		},
	}

	g, err := Compile(wf, nil)
	require.NoError(t, err)

	assert.Len(t, g.TerminalNodes, 2)
	assert.Contains(t, g.TerminalNodes, "b")
	assert.Contains(t, g.TerminalNodes, "c")
}

func TestCompile_RetryOnlyOnErrorEdges(t *testing.T) {
	wf := WorkflowConfig{
		ID: "bad-retry",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", Output: "success", To: "b", Retry: &RetryConfig{Attempts: 3}},
		},
	}

	_, err := Compile(wf, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry")
}

func TestCompile_DefaultOutputIsSuccess(t *testing.T) {
	wf := WorkflowConfig{
		ID: "default-output",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"}, // no output specified
		},
	}

	g, err := Compile(wf, nil)
	require.NoError(t, err)

	targets := g.Adjacency["a"]["success"]
	assert.Contains(t, targets, "b")
}
