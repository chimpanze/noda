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

	assert.Len(t, compiled.Programs, 4)    // literal, expr, literal, expr
	assert.Nil(t, compiled.Programs[0])    // literal
	assert.NotNil(t, compiled.Programs[1]) // expression
	assert.Nil(t, compiled.Programs[2])    // literal
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

	result, errs := c.compileAll(exprs)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "invalid")
	assert.Len(t, result, 2) // valid and also_ok
}

func TestCompile_StrictMode_RejectsUndefinedTopLevelVariable(t *testing.T) {
	c := NewCompiler(WithStrictMode(true))
	// "auht" is a typo for "auth" — strict mode catches top-level typos
	_, err := c.Compile("{{ auht.is_admin }}")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compile error")
}

func TestCompile_StrictMode_AllowsKnownTopLevelVariables(t *testing.T) {
	c := NewCompiler(WithStrictMode(true))
	// All known top-level context variables should compile fine
	for _, expr := range []string{
		"{{ input.name }}",
		"{{ auth.sub }}",
		"{{ trigger.type }}",
		"{{ nodes.step1 }}",
		"{{ secrets.api_key }}",
		"{{ 1 + 2 }}",
	} {
		compiled, err := c.Compile(expr)
		require.NoError(t, err, "expression %q should compile in strict mode", expr)
		assert.NotNil(t, compiled)
	}
}

func TestCompile_NonStrict_AllowsUndefinedVariables(t *testing.T) {
	// Default (non-strict) allows undefined variables — backward compatible
	c := NewCompiler()
	compiled, err := c.Compile("{{ auht.is_admin }}")
	require.NoError(t, err)
	assert.NotNil(t, compiled)
}

func TestCompile_StrictModeFalse_AllowsUndefinedVariables(t *testing.T) {
	c := NewCompiler(WithStrictMode(false))
	compiled, err := c.Compile("{{ auht.is_admin }}")
	require.NoError(t, err)
	assert.NotNil(t, compiled)
}

func TestCompile_RetainsOriginalText(t *testing.T) {
	c := NewCompiler()
	input := "{{ input.name }}"

	compiled, err := c.Compile(input)
	require.NoError(t, err)

	assert.Equal(t, input, compiled.Parsed.Raw)
}
