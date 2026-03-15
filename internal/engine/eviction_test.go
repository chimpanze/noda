package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEviction_AfterLastDependent(t *testing.T) {
	// A → B → C: after C runs, B's output should be evicted
	wf := WorkflowConfig{
		ID: "evict",
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
	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	execCtx.SetOutput("a", "data-a")
	execCtx.SetOutput("b", "data-b")

	tracker := NewEvictionTracker(graph, execCtx)

	// After B completes, A has one consumer (B), so A should be evicted
	tracker.NodeCompleted("b")
	_, ok := execCtx.GetOutput("a")
	assert.False(t, ok, "a should be evicted after b completes")

	// B still needed by C
	_, ok = execCtx.GetOutput("b")
	assert.True(t, ok, "b should not be evicted yet")

	// After C completes, B should be evicted
	tracker.NodeCompleted("c")
	_, ok = execCtx.GetOutput("b")
	assert.False(t, ok, "b should be evicted after c completes")
}

func TestEviction_ParallelBranches(t *testing.T) {
	// A → B, A → C: A should only be evicted after both B and C complete
	wf := WorkflowConfig{
		ID: "parallel-evict",
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
	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	execCtx.SetOutput("a", "data-a")

	tracker := NewEvictionTracker(graph, execCtx)

	// After B completes, A still needed by C
	tracker.NodeCompleted("b")
	_, ok := execCtx.GetOutput("a")
	assert.True(t, ok, "a should not be evicted — c still needs it")

	// After C completes, A can be evicted
	tracker.NodeCompleted("c")
	_, ok = execCtx.GetOutput("a")
	assert.False(t, ok, "a should be evicted after both b and c complete")
}

func TestEviction_AsAlias(t *testing.T) {
	wf := WorkflowConfig{
		ID: "alias-evict",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass", As: "user"},
			"b": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"},
		},
	}
	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	execCtx.RegisterAlias("a", "user")
	execCtx.SetOutput("a", "user-data")

	tracker := NewEvictionTracker(graph, execCtx)

	tracker.NodeCompleted("b")
	// Should evict under the alias key
	_, ok := execCtx.GetOutput("a")
	assert.False(t, ok, "aliased output should be evicted after dependent completes")
}
