package control

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func execIf(t *testing.T, condition string, input map[string]any) (string, any, error) {
	t.Helper()
	execCtx := engine.NewExecutionContext(engine.WithInput(input))
	executor := newIfExecutor(nil)
	config := map[string]any{"condition": condition}
	return executor.Execute(context.Background(), execCtx, config, nil)
}

func TestIf_TrueCondition(t *testing.T) {
	output, _, err := execIf(t, "{{ true }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "then", output)
}

func TestIf_FalseCondition(t *testing.T) {
	output, _, err := execIf(t, "{{ false }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "else", output)
}

func TestIf_ExpressionEvaluation(t *testing.T) {
	output, _, err := execIf(t, `{{ input.role == "admin" }}`, map[string]any{"role": "admin"})
	require.NoError(t, err)
	assert.Equal(t, "then", output)

	output, _, err = execIf(t, `{{ input.role == "admin" }}`, map[string]any{"role": "user"})
	require.NoError(t, err)
	assert.Equal(t, "else", output)
}

func TestIf_NilResult(t *testing.T) {
	output, _, err := execIf(t, "{{ input.missing }}", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "else", output)
}

func TestIf_NumericTruthiness(t *testing.T) {
	output, _, err := execIf(t, "{{ 0 }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "else", output)

	output, _, err = execIf(t, "{{ 1 }}", nil)
	require.NoError(t, err)
	assert.Equal(t, "then", output)
}

func TestIf_StringTruthiness(t *testing.T) {
	output, _, err := execIf(t, `{{ "" }}`, nil)
	require.NoError(t, err)
	assert.Equal(t, "else", output)

	output, _, err = execIf(t, `{{ "hello" }}`, nil)
	require.NoError(t, err)
	assert.Equal(t, "then", output)
}

func TestIf_ArrayTruthiness(t *testing.T) {
	output, _, err := execIf(t, "{{ input.items }}", map[string]any{"items": []any{}})
	require.NoError(t, err)
	assert.Equal(t, "else", output)

	output, _, err = execIf(t, "{{ input.items }}", map[string]any{"items": []any{"a"}})
	require.NoError(t, err)
	assert.Equal(t, "then", output)
}

func TestIf_InvalidExpression(t *testing.T) {
	_, _, err := execIf(t, "{{ + }}", nil)
	require.Error(t, err)
}

func TestIf_OutputDataIsResolvedValue(t *testing.T) {
	_, data, err := execIf(t, "{{ true }}", nil)
	require.NoError(t, err)
	assert.Equal(t, true, data)
}

func TestIf_Outputs(t *testing.T) {
	exec := newIfExecutor(nil)
	assert.Equal(t, []string{"then", "else", "error"}, exec.Outputs())
}
