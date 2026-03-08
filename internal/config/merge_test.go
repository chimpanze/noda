package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMergeOverlay_ScalarOverride(t *testing.T) {
	base := map[string]any{"port": float64(3000), "host": "localhost"}
	overlay := map[string]any{"port": float64(8080)}

	result := MergeOverlay(base, overlay)
	assert.Equal(t, float64(8080), result["port"])
	assert.Equal(t, "localhost", result["host"])
}

func TestMergeOverlay_NestedObjectMerge(t *testing.T) {
	base := map[string]any{
		"services": map[string]any{
			"db": map[string]any{
				"host": "localhost",
				"port": float64(5432),
			},
			"cache": map[string]any{"host": "localhost"},
		},
	}
	overlay := map[string]any{
		"services": map[string]any{
			"db": map[string]any{
				"host": "prod-db.example.com",
			},
		},
	}

	result := MergeOverlay(base, overlay)
	services := result["services"].(map[string]any)
	db := services["db"].(map[string]any)

	assert.Equal(t, "prod-db.example.com", db["host"])
	assert.Equal(t, float64(5432), db["port"]) // preserved from base
	assert.NotNil(t, services["cache"])         // preserved from base
}

func TestMergeOverlay_ArrayReplacement(t *testing.T) {
	base := map[string]any{"tags": []any{"a", "b", "c"}}
	overlay := map[string]any{"tags": []any{"x", "y"}}

	result := MergeOverlay(base, overlay)
	assert.Equal(t, []any{"x", "y"}, result["tags"])
}

func TestMergeOverlay_NullRemovesKey(t *testing.T) {
	base := map[string]any{"debug": true, "port": float64(3000)}
	overlay := map[string]any{"debug": nil}

	result := MergeOverlay(base, overlay)
	_, exists := result["debug"]
	assert.False(t, exists)
	assert.Equal(t, float64(3000), result["port"])
}

func TestMergeOverlay_DeeplyNested(t *testing.T) {
	base := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": "original",
					"e": "keep",
				},
			},
		},
	}
	overlay := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": "replaced",
				},
			},
		},
	}

	result := MergeOverlay(base, overlay)
	d := result["a"].(map[string]any)["b"].(map[string]any)["c"].(map[string]any)
	assert.Equal(t, "replaced", d["d"])
	assert.Equal(t, "keep", d["e"])
}

func TestMergeOverlay_NilOverlay(t *testing.T) {
	base := map[string]any{"port": float64(3000)}

	result := MergeOverlay(base, nil)
	assert.Equal(t, float64(3000), result["port"])
}

func TestMergeOverlay_EmptyOverlay(t *testing.T) {
	base := map[string]any{"port": float64(3000)}

	result := MergeOverlay(base, map[string]any{})
	assert.Equal(t, float64(3000), result["port"])
}

func TestMergeOverlay_DoesNotMutateInputs(t *testing.T) {
	base := map[string]any{"port": float64(3000)}
	overlay := map[string]any{"port": float64(8080)}

	_ = MergeOverlay(base, overlay)
	assert.Equal(t, float64(3000), base["port"])
	assert.Equal(t, float64(8080), overlay["port"])
}
