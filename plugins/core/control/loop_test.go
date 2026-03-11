package control

import (
	"context"
	"fmt"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRunner implements SubWorkflowRunner for testing.
type mockRunner struct {
	calls []mockRunnerCall
	err   error
}

type mockRunnerCall struct {
	WorkflowID string
	Input      any
}

func (r *mockRunner) RunSubWorkflow(_ context.Context, workflowID string, input any, _ api.ExecutionContext) (string, any, error) {
	r.calls = append(r.calls, mockRunnerCall{WorkflowID: workflowID, Input: input})
	if r.err != nil {
		return "", nil, r.err
	}
	return "done", input, nil
}

// --- Descriptor tests ---

func TestLoopDescriptor_Name(t *testing.T) {
	d := &loopDescriptor{}
	assert.Equal(t, "loop", d.Name())
}

func TestLoopDescriptor_ServiceDeps(t *testing.T) {
	d := &loopDescriptor{}
	assert.Nil(t, d.ServiceDeps())
}

func TestLoopDescriptor_ConfigSchema(t *testing.T) {
	d := &loopDescriptor{}
	schema := d.ConfigSchema()
	require.NotNil(t, schema)
	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "collection")
	assert.Contains(t, props, "workflow")
	assert.Contains(t, props, "input")

	required, ok := schema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "collection")
	assert.Contains(t, required, "workflow")
}

// --- Executor Outputs ---

func TestLoop_Outputs(t *testing.T) {
	exec := newLoopExecutor(nil)
	outputs := exec.Outputs()
	assert.Equal(t, []string{"done", "error"}, outputs)
}

// --- Execute tests ---

func TestLoop_BasicIteration(t *testing.T) {
	runner := &mockRunner{}
	exec := &LoopExecutor{Runner: runner}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{"a", "b", "c"},
	}))
	config := map[string]any{
		"collection": "{{ input.items }}",
		"workflow":   "process-item",
	}

	output, data, err := exec.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", output)

	results, ok := data.([]any)
	require.True(t, ok)
	assert.Len(t, results, 3)
	assert.Len(t, runner.calls, 3)

	for i, call := range runner.calls {
		assert.Equal(t, "process-item", call.WorkflowID)
		inputMap, ok := call.Input.(map[string]any)
		require.True(t, ok, "iteration %d input should be a map", i)
		assert.Equal(t, []any{"a", "b", "c"}[i], inputMap["item"])
		assert.Equal(t, i, inputMap["index"])
	}
}

func TestLoop_EmptyCollection(t *testing.T) {
	exec := &LoopExecutor{Runner: &mockRunner{}}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{},
	}))
	config := map[string]any{
		"collection": "{{ input.items }}",
		"workflow":   "process-item",
	}

	output, data, err := exec.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", output)
	assert.Equal(t, []any{}, data)
}

func TestLoop_NonArrayError(t *testing.T) {
	exec := &LoopExecutor{Runner: &mockRunner{}}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": "not-an-array",
	}))
	config := map[string]any{
		"collection": "{{ input.items }}",
		"workflow":   "process-item",
	}

	_, _, err := exec.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "collection must be an array")
}

func TestLoop_NilRunnerError(t *testing.T) {
	exec := &LoopExecutor{Runner: nil}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{"x"},
	}))
	config := map[string]any{
		"collection": "{{ input.items }}",
		"workflow":   "process-item",
	}

	_, _, err := exec.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sub-workflow runner not configured")
}

func TestLoop_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	runner := &mockRunner{}
	exec := &LoopExecutor{Runner: runner}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{"a", "b", "c"},
	}))
	config := map[string]any{
		"collection": "{{ input.items }}",
		"workflow":   "process-item",
	}

	_, _, err := exec.Execute(ctx, execCtx, config, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, runner.calls, "no sub-workflow calls should have been made")
}

func TestLoop_RunnerError(t *testing.T) {
	runner := &mockRunner{err: fmt.Errorf("workflow failed")}
	exec := &LoopExecutor{Runner: runner}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{"a"},
	}))
	config := map[string]any{
		"collection": "{{ input.items }}",
		"workflow":   "process-item",
	}

	_, _, err := exec.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "iteration 0")
	assert.Contains(t, err.Error(), "workflow failed")
}

func TestLoop_ResolveCollectionError(t *testing.T) {
	exec := &LoopExecutor{Runner: &mockRunner{}}
	execCtx := engine.NewExecutionContext()
	config := map[string]any{
		"collection": "{{ + }}",
		"workflow":   "process-item",
	}

	_, _, err := exec.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve collection")
}

func TestLoop_WithInputTemplate(t *testing.T) {
	runner := &mockRunner{}
	exec := &LoopExecutor{Runner: runner}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"items": []any{"alpha", "beta"},
	}))
	config := map[string]any{
		"collection": "{{ input.items }}",
		"workflow":   "process-item",
		"input": map[string]any{
			"value": "{{ $item }}",
			"pos":   "{{ $index }}",
			"fixed": 42,
		},
	}

	output, data, err := exec.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", output)

	results, ok := data.([]any)
	require.True(t, ok)
	assert.Len(t, results, 2)

	// Verify first iteration input
	first, ok := runner.calls[0].Input.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "alpha", first["value"])
	assert.Equal(t, 0, first["pos"])
	assert.Equal(t, 42, first["fixed"])

	// Verify second iteration input
	second, ok := runner.calls[1].Input.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "beta", second["value"])
	assert.Equal(t, 1, second["pos"])
	assert.Equal(t, 42, second["fixed"])
}

// --- buildIterInput tests ---

func TestBuildIterInput_NilTemplate(t *testing.T) {
	execCtx := engine.NewExecutionContext()
	result, err := buildIterInput(nil, "hello", 3, execCtx)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"item": "hello", "index": 3}, result)
}

func TestBuildIterInput_WithItemAndIndex(t *testing.T) {
	execCtx := engine.NewExecutionContext()
	template := map[string]any{
		"name": "{{ $item }}",
		"idx":  "{{ $index }}",
	}
	result, err := buildIterInput(template, "world", 5, execCtx)
	require.NoError(t, err)
	assert.Equal(t, "world", result["name"])
	assert.Equal(t, 5, result["idx"])
}

func TestBuildIterInput_NonStringValues(t *testing.T) {
	execCtx := engine.NewExecutionContext()
	template := map[string]any{
		"expr":  "{{ $item }}",
		"count": 99,
		"flag":  true,
	}
	result, err := buildIterInput(template, "val", 0, execCtx)
	require.NoError(t, err)
	assert.Equal(t, "val", result["expr"])
	assert.Equal(t, 99, result["count"])
	assert.Equal(t, true, result["flag"])
}

func TestBuildIterInput_ResolveError(t *testing.T) {
	execCtx := engine.NewExecutionContext()
	template := map[string]any{
		"bad": "{{ + }}",
	}
	_, err := buildIterInput(template, "x", 0, execCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `field "bad"`)
}

// --- newLoopExecutor factory ---

func TestNewLoopExecutor_ReturnsLoopExecutor(t *testing.T) {
	exec := newLoopExecutor(nil)
	_, ok := exec.(*LoopExecutor)
	assert.True(t, ok, "factory should return *LoopExecutor")
}

func TestNewLoopExecutor_RunnerIsNil(t *testing.T) {
	exec := newLoopExecutor(nil).(*LoopExecutor)
	assert.Nil(t, exec.Runner, "runner should be nil from factory (injected later)")
}
