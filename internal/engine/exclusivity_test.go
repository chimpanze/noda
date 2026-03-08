package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileForExclusivity(t *testing.T, wf WorkflowConfig) *CompiledGraph {
	t.Helper()
	resolver := &mapResolver{
		types: map[string][]string{
			"control.if":      {"then", "else", "error"},
			"control.switch":  {"admin", "user", "default", "error"},
			"workflow.output": {},
		},
		fallback: []string{"success", "error"},
	}
	g, err := Compile(wf, resolver)
	require.NoError(t, err)
	return g
}

func TestExclusivity_IfThenElse_Valid(t *testing.T) {
	wf := WorkflowConfig{
		ID: "excl-if",
		Nodes: map[string]NodeConfig{
			"check": {Type: "control.if"},
			"out_a": {Type: "workflow.output", Config: map[string]any{"name": "approved"}},
			"out_b": {Type: "workflow.output", Config: map[string]any{"name": "rejected"}},
		},
		Edges: []EdgeConfig{
			{From: "check", Output: "then", To: "out_a"},
			{From: "check", Output: "else", To: "out_b"},
		},
	}

	graph := compileForExclusivity(t, wf)
	err := ValidateOutputExclusivity(graph)
	assert.NoError(t, err)
}

func TestExclusivity_SwitchCases_Valid(t *testing.T) {
	wf := WorkflowConfig{
		ID: "excl-switch",
		Nodes: map[string]NodeConfig{
			"check": {Type: "control.switch"},
			"out_a": {Type: "workflow.output", Config: map[string]any{"name": "admin_out"}},
			"out_b": {Type: "workflow.output", Config: map[string]any{"name": "user_out"}},
		},
		Edges: []EdgeConfig{
			{From: "check", Output: "admin", To: "out_a"},
			{From: "check", Output: "user", To: "out_b"},
		},
	}

	graph := compileForExclusivity(t, wf)
	err := ValidateOutputExclusivity(graph)
	assert.NoError(t, err)
}

func TestExclusivity_ParallelBranches_Invalid(t *testing.T) {
	wf := WorkflowConfig{
		ID: "excl-parallel",
		Nodes: map[string]NodeConfig{
			"a":     {Type: "mock.pass"},
			"b":     {Type: "mock.pass"},
			"out_a": {Type: "workflow.output", Config: map[string]any{"name": "output1"}},
			"out_b": {Type: "workflow.output", Config: map[string]any{"name": "output2"}},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "out_a"},
			{From: "b", To: "out_b"},
		},
	}

	graph := compileForExclusivity(t, wf)
	err := ValidateOutputExclusivity(graph)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not mutually exclusive")
}

func TestExclusivity_DuplicateNames_Invalid(t *testing.T) {
	wf := WorkflowConfig{
		ID: "excl-dup",
		Nodes: map[string]NodeConfig{
			"check": {Type: "control.if"},
			"out_a": {Type: "workflow.output", Config: map[string]any{"name": "same_name"}},
			"out_b": {Type: "workflow.output", Config: map[string]any{"name": "same_name"}},
		},
		Edges: []EdgeConfig{
			{From: "check", Output: "then", To: "out_a"},
			{From: "check", Output: "else", To: "out_b"},
		},
	}

	graph := compileForExclusivity(t, wf)
	err := ValidateOutputExclusivity(graph)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
}

func TestExclusivity_SingleOutput_Valid(t *testing.T) {
	wf := WorkflowConfig{
		ID: "excl-single",
		Nodes: map[string]NodeConfig{
			"step": {Type: "mock.pass"},
			"out":  {Type: "workflow.output", Config: map[string]any{"name": "done"}},
		},
		Edges: []EdgeConfig{
			{From: "step", To: "out"},
		},
	}

	graph := compileForExclusivity(t, wf)
	err := ValidateOutputExclusivity(graph)
	assert.NoError(t, err)
}

func TestExclusivity_DeepNested_Valid(t *testing.T) {
	wf := WorkflowConfig{
		ID: "excl-deep",
		Nodes: map[string]NodeConfig{
			"check":  {Type: "control.if"},
			"step_a": {Type: "mock.pass"},
			"step_b": {Type: "mock.pass"},
			"out_a":  {Type: "workflow.output", Config: map[string]any{"name": "path_a"}},
			"out_b":  {Type: "workflow.output", Config: map[string]any{"name": "path_b"}},
		},
		Edges: []EdgeConfig{
			{From: "check", Output: "then", To: "step_a"},
			{From: "check", Output: "else", To: "step_b"},
			{From: "step_a", To: "out_a"},
			{From: "step_b", To: "out_b"},
		},
	}

	graph := compileForExclusivity(t, wf)
	err := ValidateOutputExclusivity(graph)
	assert.NoError(t, err)
}
