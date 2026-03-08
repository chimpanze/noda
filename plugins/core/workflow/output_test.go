package workflow

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutput_ResolvedData(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"name": "Alice"}))
	executor := newOutputExecutor(nil)
	config := map[string]any{
		"name": "success",
		"data": "{{ input.name }}",
	}
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", output)
	assert.Equal(t, "Alice", data)
}

func TestOutput_NoData(t *testing.T) {
	execCtx := engine.NewExecutionContext()
	executor := newOutputExecutor(nil)
	config := map[string]any{
		"name": "success",
	}
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "done", output)
	assert.Nil(t, data)
}

func TestOutput_NameAccessible(t *testing.T) {
	config := map[string]any{"name": "my-output"}
	assert.Equal(t, "my-output", OutputName(config))
}

func TestOutput_TerminalNode(t *testing.T) {
	executor := newOutputExecutor(nil)
	assert.Empty(t, executor.Outputs())
}
