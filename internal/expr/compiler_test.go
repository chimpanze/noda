package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompile_ValidExpression(t *testing.T) {
	c := NewCompiler()
	compiled, err := c.Compile("{{ input.name }}")
	require.NoError(t, err)

	assert.NotNil(t, compiled)
	assert.True(t, compiled.Parsed.IsSimple)
	assert.NotNil(t, compiled.Programs[0])
}

func TestCompile_InvalidSyntax(t *testing.T) {
	c := NewCompiler()
	_, err := c.Compile("{{ input.name ++ }}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compile error")
}

func TestCompile_Literal(t *testing.T) {
	c := NewCompiler()
	compiled, err := c.Compile("plain text")
	require.NoError(t, err)

	assert.True(t, compiled.Parsed.IsLiteral)
	assert.Nil(t, compiled.Programs[0])
}

func TestCompile_InterpolatedString(t *testing.T) {
	c := NewCompiler()
	compiled, err := c.Compile("Hello {{ input.name }}, age {{ input.age }}")
	require.NoError(t, err)

	assert.Len(t, compiled.Programs, 4) // literal, expr, literal, expr
	assert.Nil(t, compiled.Programs[0])  // literal
	assert.NotNil(t, compiled.Programs[1]) // expression
	assert.Nil(t, compiled.Programs[2])  // literal
	assert.NotNil(t, compiled.Programs[3]) // expression
}

func TestCompile_CacheHit(t *testing.T) {
	c := NewCompiler()

	compiled1, err := c.Compile("{{ input.x }}")
	require.NoError(t, err)

	compiled2, err := c.Compile("{{ input.x }}")
	require.NoError(t, err)

	assert.Same(t, compiled1, compiled2) // same pointer = cache hit
}

func TestCompileAll_CollectsErrors(t *testing.T) {
	c := NewCompiler()

	exprs := map[string]string{
		"valid":   "{{ input.name }}",
		"invalid": "{{ ++ }}",
		"also_ok": "{{ input.age }}",
	}

	result, errs := c.CompileAll(exprs)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "invalid")
	assert.Len(t, result, 2) // valid and also_ok
}

func TestCompile_RetainsOriginalText(t *testing.T) {
	c := NewCompiler()
	input := "{{ input.name }}"

	compiled, err := c.Compile(input)
	require.NoError(t, err)

	assert.Equal(t, input, compiled.Parsed.Raw)
}
