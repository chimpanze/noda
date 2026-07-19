package registry

import "testing"

// TestCompileServiceSchema_CacheKeyedByContent guards against a regression
// (I3, final-review): the compile cache used to be keyed by plugin name
// alone, so registering two different ServiceConfigSchemas under the same
// plugin name returned the first schema's compiled result for the second
// caller too — silently. This matters in practice because registry's own
// tests (and other packages') register throwaway plugins named "auth" with
// deliberately different/incomplete schemas to probe validation edge cases;
// whichever one compiles first would otherwise poison the cache slot for
// every later "auth" caller in the same test binary, including the real
// auth plugin's own audit test.
func TestCompileServiceSchema_CacheKeyedByContent(t *testing.T) {
	schemaA := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"foo": map[string]any{"type": "string"},
		},
		"required":             []any{"foo"},
		"additionalProperties": false,
	}
	schemaB := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bar": map[string]any{"type": "integer"},
		},
		"required":             []any{"bar"},
		"additionalProperties": false,
	}

	compiledA, err := compileServiceSchema("dupname", schemaA)
	if err != nil {
		t.Fatalf("compile schemaA: %v", err)
	}
	compiledB, err := compileServiceSchema("dupname", schemaB)
	if err != nil {
		t.Fatalf("compile schemaB: %v", err)
	}

	if compiledA == compiledB {
		t.Fatal("expected different compiled schemas for different content under the same plugin name, got the same *jsonschema.Schema (stale cache hit)")
	}

	// Each compiled schema must actually enforce its own shape, not the
	// other's (a stale-cache bug would make B silently validate against A's
	// required field "foo" instead of B's "bar").
	if err := validateAgainst(compiledA, map[string]any{"foo": "x"}); err != nil {
		t.Fatalf("schemaA should accept {foo: x}: %v", err)
	}
	if err := validateAgainst(compiledA, map[string]any{"bar": 1}); err == nil {
		t.Fatal("schemaA should reject {bar: 1} (missing required foo, unknown key bar)")
	}
	if err := validateAgainst(compiledB, map[string]any{"bar": 1}); err != nil {
		t.Fatalf("schemaB should accept {bar: 1}: %v", err)
	}
	if err := validateAgainst(compiledB, map[string]any{"foo": "x"}); err == nil {
		t.Fatal("schemaB should reject {foo: x} (missing required bar, unknown key foo)")
	}

	// Re-compiling the same schema content under the same name must hit the
	// cache and return the identical compiled schema.
	compiledAAgain, err := compileServiceSchema("dupname", schemaA)
	if err != nil {
		t.Fatalf("recompile schemaA: %v", err)
	}
	if compiledAAgain != compiledA {
		t.Fatal("expected a cache hit (identical *jsonschema.Schema) for identical name+content")
	}
}
