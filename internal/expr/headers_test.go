package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resolveWith(t *testing.T, exprStr string, ctx map[string]any) any {
	t.Helper()
	c := NewCompilerWithFunctions()
	r := NewResolver(c, ctx)
	v, err := r.Resolve(exprStr)
	require.NoError(t, err)
	return v
}

func TestHeaderKeyPatcher(t *testing.T) {
	headers := map[string]any{"x-github-event": "issues", "content-type": "application/json", "authorization": "Bearer tok"}

	t.Run("bracket access with mixed-case literal", func(t *testing.T) {
		got := resolveWith(t, "{{ headers['X-GitHub-Event'] }}", map[string]any{"headers": headers})
		assert.Equal(t, "issues", got)
	})

	t.Run("request.headers base", func(t *testing.T) {
		got := resolveWith(t, "{{ request.headers['Content-Type'] }}",
			map[string]any{"request": map[string]any{"headers": headers}})
		assert.Equal(t, "application/json", got)
	})

	t.Run("input.headers base", func(t *testing.T) {
		got := resolveWith(t, "{{ input.headers['X-GITHUB-EVENT'] }}",
			map[string]any{"input": map[string]any{"headers": headers}})
		assert.Equal(t, "issues", got)
	})

	t.Run("nodes response headers base", func(t *testing.T) {
		got := resolveWith(t, "{{ nodes.fetch.headers['Content-Type'] }}",
			map[string]any{"nodes": map[string]any{"fetch": map[string]any{"headers": headers}}})
		assert.Equal(t, "application/json", got)
	})

	t.Run("dot access is lowercased", func(t *testing.T) {
		got := resolveWith(t, "{{ headers.Authorization }}", map[string]any{"headers": headers})
		assert.Equal(t, "Bearer tok", got)
	})

	t.Run("non-headers maps are not rewritten", func(t *testing.T) {
		got := resolveWith(t, "{{ body['X-Key'] }}",
			map[string]any{"body": map[string]any{"X-Key": "kept"}})
		assert.Equal(t, "kept", got)
	})

	t.Run("dynamic keys are not rewritten", func(t *testing.T) {
		got := resolveWith(t, "{{ headers[input.h] }}",
			map[string]any{"headers": headers, "input": map[string]any{"h": "x-github-event"}})
		assert.Equal(t, "issues", got)
	})
}
