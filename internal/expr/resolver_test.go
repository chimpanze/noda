package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolver_Resolve_SimpleExpression(t *testing.T) {
	c := NewCompilerWithFunctions()
	ctx := map[string]any{
		"input": map[string]any{
			"name": "Alice",
		},
	}
	r := NewResolver(c, ctx)

	result, err := r.Resolve("{{ input.name }}")
	require.NoError(t, err)
	assert.Equal(t, "Alice", result)
}

func TestResolver_Resolve_Interpolation(t *testing.T) {
	c := NewCompilerWithFunctions()
	ctx := map[string]any{
		"input": map[string]any{
			"name": "Bob",
		},
	}
	r := NewResolver(c, ctx)

	result, err := r.Resolve("Hello {{ input.name }}!")
	require.NoError(t, err)
	assert.Equal(t, "Hello Bob!", result)
}

func TestResolver_Resolve_Literal(t *testing.T) {
	c := NewCompilerWithFunctions()
	r := NewResolver(c, map[string]any{})

	result, err := r.Resolve("plain text")
	require.NoError(t, err)
	assert.Equal(t, "plain text", result)
}

func TestResolver_Resolve_WithFunction(t *testing.T) {
	c := NewCompilerWithFunctions()
	r := NewResolver(c, map[string]any{})

	result, err := r.Resolve(`{{ upper("hello") }}`)
	require.NoError(t, err)
	assert.Equal(t, "HELLO", result)
}

func TestResolver_ResolveMap_Simple(t *testing.T) {
	c := NewCompilerWithFunctions()
	ctx := map[string]any{
		"input": map[string]any{
			"name":  "Alice",
			"email": "alice@example.com",
		},
	}
	r := NewResolver(c, ctx)

	config := map[string]any{
		"to":      "{{ input.email }}",
		"subject": "Hello {{ input.name }}",
	}

	result, err := r.ResolveMap(config)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", result["to"])
	assert.Equal(t, "Hello Alice", result["subject"])
}

func TestResolver_ResolveMap_NestedMap(t *testing.T) {
	c := NewCompilerWithFunctions()
	ctx := map[string]any{
		"input": map[string]any{
			"name": "Alice",
		},
	}
	r := NewResolver(c, ctx)

	config := map[string]any{
		"body": map[string]any{
			"greeting": "Hello {{ input.name }}",
		},
	}

	result, err := r.ResolveMap(config)
	require.NoError(t, err)
	body := result["body"].(map[string]any)
	assert.Equal(t, "Hello Alice", body["greeting"])
}

func TestResolver_ResolveMap_Array(t *testing.T) {
	c := NewCompilerWithFunctions()
	ctx := map[string]any{
		"input": map[string]any{
			"a": "X",
			"b": "Y",
		},
	}
	r := NewResolver(c, ctx)

	config := map[string]any{
		"items": []any{"{{ input.a }}", "{{ input.b }}"},
	}

	result, err := r.ResolveMap(config)
	require.NoError(t, err)
	items := result["items"].([]any)
	assert.Equal(t, "X", items[0])
	assert.Equal(t, "Y", items[1])
}

func TestResolver_ResolveMap_NonStringPassthrough(t *testing.T) {
	c := NewCompilerWithFunctions()
	r := NewResolver(c, map[string]any{})

	config := map[string]any{
		"count":   42,
		"enabled": true,
		"ratio":   3.14,
	}

	result, err := r.ResolveMap(config)
	require.NoError(t, err)
	assert.Equal(t, 42, result["count"])
	assert.Equal(t, true, result["enabled"])
	assert.Equal(t, 3.14, result["ratio"])
}

func TestResolver_ResolveMap_ErrorPropagation(t *testing.T) {
	c := NewCompiler() // no functions registered
	r := NewResolver(c, map[string]any{})

	config := map[string]any{
		"value": "{{ nonexistent() }}",
	}

	_, err := r.ResolveMap(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value")
}
