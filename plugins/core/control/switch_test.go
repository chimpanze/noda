package control

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSwitch_MatchesCase(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"status": "active"}))
	config := map[string]any{
		"expression": `{{ input.status }}`,
		"cases":      []any{"active", "inactive", "pending"},
	}
	executor := newSwitchExecutor(config)
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "active", output)
	assert.Equal(t, "active", data)
}

func TestSwitch_NoMatch_Default(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"status": "unknown"}))
	config := map[string]any{
		"expression": `{{ input.status }}`,
		"cases":      []any{"active", "inactive"},
	}
	executor := newSwitchExecutor(config)
	output, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "default", output)
}

func TestSwitch_ExpressionError(t *testing.T) {
	execCtx := engine.NewExecutionContext()
	config := map[string]any{
		"expression": "{{ + }}",
		"cases":      []any{"a"},
	}
	executor := newSwitchExecutor(config)
	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
}

func TestSwitch_IntegerConversion(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"code": 42}))
	config := map[string]any{
		"expression": "{{ input.code }}",
		"cases":      []any{"42", "200"},
	}
	executor := newSwitchExecutor(config)
	output, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "42", output)
}

func TestSwitch_EmptyCases(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"x": "val"}))
	config := map[string]any{
		"expression": "{{ input.x }}",
		"cases":      []any{},
	}
	executor := newSwitchExecutor(config)
	output, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "default", output)
}

func TestSwitch_DynamicOutputs(t *testing.T) {
	config := map[string]any{
		"cases": []any{"admin", "user", "guest"},
	}
	executor := newSwitchExecutor(config)
	outputs := executor.Outputs()
	assert.Contains(t, outputs, "admin")
	assert.Contains(t, outputs, "user")
	assert.Contains(t, outputs, "guest")
	assert.Contains(t, outputs, "default")
	assert.Contains(t, outputs, "error")
}
