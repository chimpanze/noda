package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test: compile expressions from a sample workflow config, evaluate against a mock context, verify all results
func TestIntegration_WorkflowConfig(t *testing.T) {
	compiler := NewCompilerWithFunctions()

	// Simulate a workflow node config with expressions
	config := map[string]any{
		"table":  "users",
		"action": "insert",
		"data": map[string]any{
			"name":  "{{ input.name }}",
			"email": "{{ input.email }}",
			"role":  "member",
		},
		"returning": []any{"id", "{{ input.extra_field }}"},
	}

	// Pre-compile all expressions at startup
	expressions := collectExpressions(config)
	for _, expr := range expressions {
		_, err := compiler.Compile(expr)
		require.NoError(t, err, "failed to compile: %s", expr)
	}

	// Evaluate at runtime
	ctx := map[string]any{
		"input": map[string]any{
			"name":        "Alice",
			"email":       "alice@example.com",
			"extra_field": "created_at",
		},
	}
	resolver := NewResolver(compiler, ctx)
	result, err := resolver.ResolveMap(config)
	require.NoError(t, err)

	assert.Equal(t, "users", result["table"])
	assert.Equal(t, "insert", result["action"])

	data := result["data"].(map[string]any)
	assert.Equal(t, "Alice", data["name"])
	assert.Equal(t, "alice@example.com", data["email"])
	assert.Equal(t, "member", data["role"])

	returning := result["returning"].([]any)
	assert.Equal(t, "id", returning[0])
	assert.Equal(t, "created_at", returning[1])
}

// Test: string interpolation with multiple expressions in route trigger mapping
func TestIntegration_RouteInterpolation(t *testing.T) {
	compiler := NewCompilerWithFunctions()
	ctx := map[string]any{
		"input": map[string]any{
			"first": "John",
			"last":  "Doe",
		},
		"request": map[string]any{
			"method": "POST",
		},
	}
	resolver := NewResolver(compiler, ctx)

	result, err := resolver.Resolve("{{ input.first }} {{ input.last }} via {{ request.method }}")
	require.NoError(t, err)
	assert.Equal(t, "John Doe via POST", result)
}

// Test: custom functions in expressions
func TestIntegration_CustomFunctions(t *testing.T) {
	reg := NewFunctionRegistry()
	reg.Register("greet", func(params ...any) (any, error) {
		return "Hello, " + params[0].(string) + "!", nil
	}, new(func(string) string))

	compiler := NewCompiler(WithExprOptions(reg.ExprOptions()...))
	ctx := map[string]any{
		"input": map[string]any{"name": "Alice"},
	}
	resolver := NewResolver(compiler, ctx)

	result, err := resolver.Resolve(`{{ greet(input.name) }}`)
	require.NoError(t, err)
	assert.Equal(t, "Hello, Alice!", result)
}

// Test: static field validation catches expressions in mode and cases fields
func TestIntegration_StaticFieldValidation(t *testing.T) {
	config := map[string]any{
		"mode":  "{{ input.mode }}",
		"cases": "{{ input.cases }}",
		"input": "{{ steps.prev.output }}",
	}

	errs := ValidateStaticFields(config, []string{"mode", "cases"})
	assert.Len(t, errs, 2)

	// "input" is not in static fields list, so no error for it
	errStrings := make([]string, len(errs))
	for i, e := range errs {
		errStrings[i] = e.Error()
	}
	assert.Contains(t, errStrings[0]+errStrings[1], "mode")
	assert.Contains(t, errStrings[0]+errStrings[1], "cases")
}

// Test: compile-time errors are collected when loading a workflow with invalid expressions
func TestIntegration_CompileTimeErrors(t *testing.T) {
	compiler := NewCompiler()

	invalidExpressions := []string{
		"{{ + }}",
		"{{ 1 + }}",
		"{{ [unclosed }}",
	}

	var compileErrors []error
	for _, expr := range invalidExpressions {
		_, err := compiler.Compile(expr)
		if err != nil {
			compileErrors = append(compileErrors, err)
		}
	}

	assert.NotEmpty(t, compileErrors, "should have compile errors for invalid expressions")
}

// Test: runtime evaluation error includes the original expression text
func TestIntegration_RuntimeErrorContext(t *testing.T) {
	compiler := NewCompiler()
	ctx := map[string]any{} // empty context

	resolver := NewResolver(compiler, ctx)

	config := map[string]any{
		"name": "{{ missing.deep.path }}",
	}

	_, err := resolver.ResolveMap(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name") // field name in error
}

// Test: full pipeline — parse → compile → cache → resolve
func TestIntegration_FullPipeline(t *testing.T) {
	compiler := NewCompilerWithFunctions()

	// Step 1: Parse and compile at startup
	expressions := []string{
		"{{ input.x + input.y }}",
		"{{ upper(input.label) }}",
		"Result: {{ input.x * 2 }}",
	}
	for _, e := range expressions {
		_, err := compiler.Compile(e)
		require.NoError(t, err)
	}

	// Step 2: Verify cache hit (compile again, should use cache)
	for _, e := range expressions {
		_, err := compiler.Compile(e)
		require.NoError(t, err)
	}

	// Step 3: Evaluate at runtime
	ctx := map[string]any{
		"input": map[string]any{
			"x":     10,
			"y":     20,
			"label": "hello",
		},
	}
	resolver := NewResolver(compiler, ctx)

	r1, err := resolver.Resolve(expressions[0])
	require.NoError(t, err)
	assert.Equal(t, 30, r1)

	r2, err := resolver.Resolve(expressions[1])
	require.NoError(t, err)
	assert.Equal(t, "HELLO", r2)

	r3, err := resolver.Resolve(expressions[2])
	require.NoError(t, err)
	assert.Equal(t, "Result: 20", r3)
}

// collectExpressions recursively collects all string values from a config map.
func collectExpressions(m map[string]any) []string {
	var result []string
	for _, v := range m {
		switch val := v.(type) {
		case string:
			result = append(result, val)
		case map[string]any:
			result = append(result, collectExpressions(val)...)
		case []any:
			for _, item := range val {
				if s, ok := item.(string); ok {
					result = append(result, s)
				}
			}
		}
	}
	return result
}
