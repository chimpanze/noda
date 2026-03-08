package expr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileAndEvalWithFunctions(t *testing.T, input string, ctx map[string]any) any {
	t.Helper()
	c := NewCompilerWithFunctions()
	compiled, err := c.Compile(input)
	require.NoError(t, err)
	result, err := Evaluate(compiled, ctx)
	require.NoError(t, err)
	return result
}

func TestFunction_UUID(t *testing.T) {
	result := compileAndEvalWithFunctions(t, "{{ $uuid() }}", map[string]any{})
	s, ok := result.(string)
	require.True(t, ok)
	assert.Len(t, s, 36) // UUID v4 format: 8-4-4-4-12
	assert.Contains(t, s, "-")
}

func TestFunction_Lower(t *testing.T) {
	result := compileAndEvalWithFunctions(t, `{{ lower("HELLO") }}`, map[string]any{})
	assert.Equal(t, "hello", result)
}

func TestFunction_Upper(t *testing.T) {
	result := compileAndEvalWithFunctions(t, `{{ upper("hello") }}`, map[string]any{})
	assert.Equal(t, "HELLO", result)
}

func TestFunction_Now(t *testing.T) {
	before := time.Now()
	result := compileAndEvalWithFunctions(t, "{{ now() }}", map[string]any{})
	after := time.Now()

	ts, ok := result.(time.Time)
	require.True(t, ok)
	assert.True(t, !ts.Before(before) && !ts.After(after))
}

func TestFunction_LenWithArray(t *testing.T) {
	ctx := map[string]any{
		"items": []any{"a", "b", "c"},
	}
	result := compileAndEvalWithFunctions(t, "{{ len(items) }}", ctx)
	assert.Equal(t, 3, result)
}

func TestFunction_UnknownFunction(t *testing.T) {
	c := NewCompilerWithFunctions()
	compiled, err := c.Compile("{{ nonexistent() }}")
	require.NoError(t, err) // compiles with AllowUndefinedVariables

	// Fails at runtime
	_, err = Evaluate(compiled, map[string]any{})
	require.Error(t, err)
}

func TestFunctionRegistry_CustomFunction(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("double", func(params ...any) (any, error) {
		return params[0].(int) * 2, nil
	}, new(func(int) int))

	c := NewCompiler(WithExprOptions(reg.ExprOptions()...))
	compiled, err := c.Compile("{{ double(21) }}")
	require.NoError(t, err)

	result, err := Evaluate(compiled, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}
