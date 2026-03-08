package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func compileAndEval(t *testing.T, input string, ctx map[string]any) any {
	t.Helper()
	c := NewCompiler()
	compiled, err := c.Compile(input)
	require.NoError(t, err)
	result, err := Evaluate(compiled, ctx)
	require.NoError(t, err)
	return result
}

func TestEvaluate_SimplePathAccess(t *testing.T) {
	ctx := map[string]any{"input": map[string]any{"name": "Alice"}}
	result := compileAndEval(t, "{{ input.name }}", ctx)
	assert.Equal(t, "Alice", result)
}

func TestEvaluate_NestedPath(t *testing.T) {
	ctx := map[string]any{
		"input": map[string]any{
			"user": map[string]any{
				"address": map[string]any{
					"city": "Berlin",
				},
			},
		},
	}
	result := compileAndEval(t, "{{ input.user.address.city }}", ctx)
	assert.Equal(t, "Berlin", result)
}

func TestEvaluate_Arithmetic(t *testing.T) {
	ctx := map[string]any{
		"input": map[string]any{
			"page":  2,
			"limit": 20,
		},
	}
	result := compileAndEval(t, "{{ input.page * input.limit }}", ctx)
	assert.Equal(t, 40, result)
}

func TestEvaluate_Comparison(t *testing.T) {
	ctx := map[string]any{
		"input": map[string]any{"role": "admin"},
	}
	result := compileAndEval(t, `{{ input.role == "admin" }}`, ctx)
	assert.Equal(t, true, result)
}

func TestEvaluate_Ternary(t *testing.T) {
	ctx := map[string]any{
		"input": map[string]any{"role": "user"},
	}
	result := compileAndEval(t, `{{ input.role == "admin" ? "full" : "limited" }}`, ctx)
	assert.Equal(t, "limited", result)
}

func TestEvaluate_StringInterpolation(t *testing.T) {
	ctx := map[string]any{
		"input": map[string]any{"name": "Alice"},
	}
	result := compileAndEval(t, "Hello {{ input.name }}", ctx)
	assert.Equal(t, "Hello Alice", result)
}

func TestEvaluate_MultipleInterpolations(t *testing.T) {
	ctx := map[string]any{
		"input": map[string]any{"name": "Alice", "age": 30},
	}
	result := compileAndEval(t, "{{ input.name }} is {{ input.age }} years old", ctx)
	assert.Equal(t, "Alice is 30 years old", result)
}

func TestEvaluate_ArrayAccess(t *testing.T) {
	ctx := map[string]any{
		"items": []any{
			map[string]any{"name": "first"},
			map[string]any{"name": "second"},
		},
	}
	result := compileAndEval(t, "{{ items[0].name }}", ctx)
	assert.Equal(t, "first", result)
}

func TestEvaluate_LenFunction(t *testing.T) {
	ctx := map[string]any{
		"items": []any{"a", "b", "c"},
	}
	result := compileAndEval(t, "{{ len(items) }}", ctx)
	assert.Equal(t, 3, result)
}

func TestEvaluate_BoolPreserved(t *testing.T) {
	ctx := map[string]any{
		"input": map[string]any{"active": true},
	}
	result := compileAndEval(t, "{{ input.active }}", ctx)
	assert.IsType(t, true, result)
	assert.Equal(t, true, result)
}

func TestEvaluate_IntPreserved(t *testing.T) {
	ctx := map[string]any{
		"input": map[string]any{"count": 42},
	}
	result := compileAndEval(t, "{{ input.count }}", ctx)
	assert.Equal(t, 42, result)
}

func TestEvaluate_Literal(t *testing.T) {
	ctx := map[string]any{}
	result := compileAndEval(t, "plain text", ctx)
	assert.Equal(t, "plain text", result)
}

func TestEvaluate_NilNestedAccess(t *testing.T) {
	c := NewCompiler()
	compiled, err := c.Compile("{{ missing.nested.value }}")
	require.NoError(t, err) // compiles fine with AllowUndefinedVariables

	_, err = Evaluate(compiled, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evaluation error")
}

func TestEvaluate_UndefinedTopLevel(t *testing.T) {
	c := NewCompiler()
	compiled, err := c.Compile("{{ missing }}")
	require.NoError(t, err)

	result, err := Evaluate(compiled, map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, result) // undefined top-level returns nil
}
