package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A nested object value (e.g. a JSONB column) must have its templated leaves
// resolved, not copied through verbatim (#438).
func TestResolveMap_NestedMapIsResolved(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved_" + expr, nil
	}}
	config := map[string]any{
		"data": map[string]any{
			"metadata": map[string]any{
				"username": "expr_username",
				"bio":      "expr_bio",
			},
		},
	}

	result, err := ResolveMap(nCtx, config, "data")
	require.NoError(t, err)

	meta, ok := result["metadata"].(map[string]any)
	require.True(t, ok, "metadata should stay a map")
	assert.Equal(t, "resolved_expr_username", meta["username"])
	assert.Equal(t, "resolved_expr_bio", meta["bio"])
}

// Arbitrary depth, and slices along the way.
func TestResolveMap_DeeplyNestedAndSlices(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved_" + expr, nil
	}}
	config := map[string]any{
		"data": map[string]any{
			"a": map[string]any{
				"b": map[string]any{
					"c": "deep",
				},
				"list": []any{"item1", map[string]any{"d": "item2"}},
			},
		},
	}

	result, err := ResolveMap(nCtx, config, "data")
	require.NoError(t, err)

	a := result["a"].(map[string]any)
	assert.Equal(t, "resolved_deep", a["b"].(map[string]any)["c"])

	list := a["list"].([]any)
	assert.Equal(t, "resolved_item1", list[0])
	assert.Equal(t, "resolved_item2", list[1].(map[string]any)["d"])
}

// Non-string leaves keep their type and value.
func TestResolveMap_NestedNonStringsPassThrough(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"data": map[string]any{
			"nested": map[string]any{"n": 42, "b": true, "nil": nil},
		},
	}

	result, err := ResolveMap(nCtx, config, "data")
	require.NoError(t, err)

	nested := result["nested"].(map[string]any)
	assert.Equal(t, 42, nested["n"])
	assert.Equal(t, true, nested["b"])
	assert.Nil(t, nested["nil"])
}

// A resolution failure deep in the structure must name the path, not vanish.
func TestResolveMap_NestedResolveErrorIsReported(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		if expr == "boom" {
			return nil, assert.AnError
		}
		return expr, nil
	}}
	config := map[string]any{
		"data": map[string]any{"outer": map[string]any{"inner": "boom"}},
	}

	_, err := ResolveMap(nCtx, config, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data")
	assert.Contains(t, err.Error(), "outer")
	assert.Contains(t, err.Error(), "inner")
}
