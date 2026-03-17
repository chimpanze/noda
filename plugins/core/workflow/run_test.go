package workflow

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
	outputName string
	outputData any
	err        error
	lastInput  any
}

func (r *mockRunner) RunSubWorkflow(_ context.Context, workflowID string, input any, parentCtx api.ExecutionContext) (string, any, error) {
	r.lastInput = input
	if r.err != nil {
		return "", nil, r.err
	}
	return r.outputName, r.outputData, nil
}

func TestRun_ExecutesSubWorkflow(t *testing.T) {
	runner := &mockRunner{outputName: "success", outputData: map[string]any{"id": 1}}
	executor := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"name": "Alice"}))
	config := map[string]any{
		"workflow": "sub-workflow",
		"input": map[string]any{
			"name": "{{ input.name }}",
		},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, map[string]any{"id": 1}, data)

	// Verify input was resolved
	inputMap := runner.lastInput.(map[string]any)
	assert.Equal(t, "Alice", inputMap["name"])
}

func TestRun_SubWorkflowFailure(t *testing.T) {
	runner := &mockRunner{err: fmt.Errorf("sub-workflow failed")}
	executor := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}

	execCtx := engine.NewExecutionContext()
	config := map[string]any{"workflow": "bad-wf"}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sub-workflow failed")
}

func TestRun_NoRunner(t *testing.T) {
	executor := &RunExecutor{outputs: []string{"success", "error"}}

	execCtx := engine.NewExecutionContext()
	config := map[string]any{"workflow": "test"}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestRun_DynamicOutputs(t *testing.T) {
	executor := &RunExecutor{outputs: []string{"success", "error"}}
	assert.Equal(t, []string{"success", "error"}, executor.Outputs())

	executor.SetOutputs([]string{"approved", "rejected", "error"})
	assert.Equal(t, []string{"approved", "rejected", "error"}, executor.Outputs())
}

func TestRun_InputResolveError(t *testing.T) {
	runner := &mockRunner{outputName: "success"}
	executor := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}

	execCtx := engine.NewExecutionContext()
	config := map[string]any{
		"workflow": "sub-wf",
		"input": map[string]any{
			"bad": "{{ invalid..expr }}",
		},
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve input")
}

func TestRun_InputNonStringValues(t *testing.T) {
	runner := &mockRunner{outputName: "success", outputData: "ok"}
	executor := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}

	execCtx := engine.NewExecutionContext()
	config := map[string]any{
		"workflow": "sub-wf",
		"input": map[string]any{
			"count":  42,
			"active": true,
		},
	}

	output, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	inputMap := runner.lastInput.(map[string]any)
	assert.Equal(t, 42, inputMap["count"])
	assert.Equal(t, true, inputMap["active"])
}

func TestRun_DepthTrackingExceeded(t *testing.T) {
	runner := &mockRunner{outputName: "success"}
	executor := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}

	execCtx := engine.NewExecutionContext()
	// Exhaust recursion depth by calling CheckAndIncrementDepth until it fails
	for i := 0; i < 64; i++ {
		err := execCtx.CheckAndIncrementDepth()
		require.NoError(t, err)
	}

	config := map[string]any{"workflow": "sub-wf"}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "depth")
}

func TestRun_DepthTrackingSuccess(t *testing.T) {
	runner := &mockRunner{outputName: "success", outputData: "ok"}
	executor := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}

	execCtx := engine.NewExecutionContext()
	config := map[string]any{"workflow": "sub-wf"}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "ok", data)
}

func TestRun_NoInputMap(t *testing.T) {
	runner := &mockRunner{outputName: "success", outputData: "done"}
	executor := &RunExecutor{Runner: runner, outputs: []string{"success", "error"}}

	execCtx := engine.NewExecutionContext()
	config := map[string]any{"workflow": "sub-wf"}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "done", data)
	assert.Nil(t, runner.lastInput)
}

func TestRunDescriptor_Metadata(t *testing.T) {
	desc := &runDescriptor{}
	assert.Equal(t, "run", desc.Name())
	assert.Contains(t, desc.Description(), "sub-workflow")

	outputs := desc.OutputDescriptions()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

func TestNewRunExecutor_Factory(t *testing.T) {
	executor := newRunExecutor(map[string]any{"workflow": "test"})
	require.NotNil(t, executor)
	assert.Equal(t, []string{"success", "error"}, executor.Outputs())
}
