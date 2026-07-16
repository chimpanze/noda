package registry

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func schemaFor(props map[string]any, required ...any) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func TestValidateNodeConfig(t *testing.T) {
	intProp := map[string]any{"type": "integer"}
	strProp := map[string]any{"type": "string"}

	tests := []struct {
		name    string
		schema  map[string]any
		config  map[string]any
		wantErr string // substring; "" = valid
	}{
		{"missing required", schemaFor(map[string]any{"status": intProp}, "status"),
			map[string]any{}, `missing required config field "status"`},
		{"required satisfied by expression", schemaFor(map[string]any{"status": intProp}, "status"),
			map[string]any{"status": "{{ input.code }}"}, ""},
		{"wrong type", schemaFor(map[string]any{"status": intProp}, "status"),
			map[string]any{"status": "created"}, `"status"`},
		{"integer accepts integral float64", schemaFor(map[string]any{"status": intProp}),
			map[string]any{"status": float64(200)}, ""},
		{"integer rejects fractional", schemaFor(map[string]any{"status": intProp}),
			map[string]any{"status": 2.5}, `"status"`},
		{"union type", schemaFor(map[string]any{"v": map[string]any{"type": []any{"integer", "string"}}}),
			map[string]any{"v": "abc"}, ""},
		{"unknown top-level key", schemaFor(map[string]any{"name": strProp}),
			map[string]any{"nmae": "x"}, `unknown config field "nmae"`},
		{"unknown key allowed with additionalProperties", map[string]any{
			"type": "object", "properties": map[string]any{}, "additionalProperties": true},
			map[string]any{"anything": 1}, ""},
		{"enum violation", schemaFor(map[string]any{"mode": map[string]any{"type": "string", "enum": []any{"a", "b"}}}),
			map[string]any{"mode": "c"}, `"mode"`},
		{"enum satisfied by expression", schemaFor(map[string]any{"mode": map[string]any{"type": "string", "enum": []any{"a", "b"}}}),
			map[string]any{"mode": "{{ input.m }}"}, ""},
		{"items validated", schemaFor(map[string]any{"tags": map[string]any{"type": "array", "items": strProp}}),
			map[string]any{"tags": []any{"ok", 5}}, `tags[1]`},
		{"nested object properties", schemaFor(map[string]any{"opts": map[string]any{
			"type": "object", "properties": map[string]any{"n": intProp}, "required": []any{"n"}}}),
			map[string]any{"opts": map[string]any{}}, `opts`},
		{"nested unknown key tolerated without additionalProperties:false", schemaFor(map[string]any{"opts": map[string]any{
			"type": "object", "properties": map[string]any{"n": intProp}}}),
			map[string]any{"opts": map[string]any{"extra": 1}}, ""},
		{"oneOf any branch", map[string]any{"oneOf": []any{
			schemaFor(map[string]any{"a": strProp}, "a"),
			schemaFor(map[string]any{"b": strProp}, "b"),
		}}, map[string]any{"b": "x"}, ""},
		{"oneOf no branch", map[string]any{"oneOf": []any{
			schemaFor(map[string]any{"a": strProp}, "a"),
			schemaFor(map[string]any{"b": strProp}, "b"),
		}}, map[string]any{"c": "x"}, "does not match"},
		{"empty schema accepts anything", map[string]any{}, map[string]any{"x": 1}, ""},
		{"root oneOf branch match but extra unknown top-level key", map[string]any{"oneOf": []any{
			schemaFor(map[string]any{"a": strProp}, "a"),
			schemaFor(map[string]any{"b": strProp}, "b"),
		}}, map[string]any{"b": "x", "extra": 1}, "does not match"},
		{"nested object additionalProperties false rejects unknown key", schemaFor(map[string]any{"opts": map[string]any{
			"type": "object", "properties": map[string]any{"n": intProp}, "additionalProperties": false}}),
			map[string]any{"opts": map[string]any{"n": 1, "extra": 2}}, `unknown config field "extra"`},
		{"mid-text expression satisfies integer type", schemaFor(map[string]any{"auth": intProp}),
			map[string]any{"auth": "Bearer {{ input.token }}"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateNodeConfig(tt.schema, tt.config)
			if tt.wantErr == "" {
				assert.Empty(t, errs)
				return
			}
			if assert.NotEmpty(t, errs) {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Error(), tt.wantErr) {
						found = true
					}
				}
				assert.True(t, found, "want error containing %q, got %v", tt.wantErr, errs)
			}
		})
	}
}

func TestCheckSchemaVocabulary(t *testing.T) {
	ok := map[string]any{"type": "object", "properties": map[string]any{
		// "pattern" here is a FIELD NAME (key of properties), not a keyword — must pass
		"pattern": map[string]any{"type": "string", "description": "d"},
	}, "required": []any{"pattern"}}
	assert.Empty(t, CheckSchemaVocabulary(ok))

	bad := map[string]any{"type": "object", "properties": map[string]any{
		"n": map[string]any{"type": "integer", "minimum": float64(1)},
	}}
	errs := CheckSchemaVocabulary(bad)
	if assert.NotEmpty(t, errs) {
		assert.Contains(t, errs[0].Error(), "minimum")
	}
}

func TestCheckSchemaVocabularyShapes(t *testing.T) {
	t.Run("required as []string is rejected", func(t *testing.T) {
		schema := map[string]any{"type": "object", "required": []string{"x"}}
		errs := CheckSchemaVocabulary(schema)
		if assert.NotEmpty(t, errs) {
			assert.Contains(t, errs[0].Error(), "required")
		}
	})

	t.Run("properties as []any is rejected", func(t *testing.T) {
		schema := map[string]any{"type": "object", "properties": []any{
			map[string]any{"type": "string"},
		}}
		errs := CheckSchemaVocabulary(schema)
		if assert.NotEmpty(t, errs) {
			assert.Contains(t, errs[0].Error(), "properties")
		}
	})

	t.Run("type as []any of mixed types is rejected", func(t *testing.T) {
		schema := map[string]any{"type": []any{"integer", 5}}
		errs := CheckSchemaVocabulary(schema)
		if assert.NotEmpty(t, errs) {
			assert.Contains(t, errs[0].Error(), "type")
		}
	})

	t.Run("constraint keyword as sibling of oneOf is rejected", func(t *testing.T) {
		schema := map[string]any{
			"oneOf": []any{
				map[string]any{"type": "string"},
				map[string]any{"type": "integer"},
			},
			"required": []any{"x"},
		}
		errs := CheckSchemaVocabulary(schema)
		if assert.NotEmpty(t, errs) {
			found := false
			for _, e := range errs {
				if strings.Contains(e.Error(), "oneOf") && strings.Contains(e.Error(), "required") {
					found = true
				}
			}
			assert.True(t, found, "want sibling-of-oneOf error mentioning both keywords, got %v", errs)
		}
	})

	t.Run("annotation keyword as sibling of oneOf is fine", func(t *testing.T) {
		schema := map[string]any{
			"oneOf": []any{
				map[string]any{"type": "string"},
			},
			"description": "d",
		}
		assert.Empty(t, CheckSchemaVocabulary(schema))
	})

	t.Run("additionalProperties as string is rejected", func(t *testing.T) {
		schema := map[string]any{"type": "object", "additionalProperties": "yes"}
		errs := CheckSchemaVocabulary(schema)
		if assert.NotEmpty(t, errs) {
			assert.Contains(t, errs[0].Error(), "additionalProperties")
		}
	})

	t.Run("enum as non-slice is rejected", func(t *testing.T) {
		schema := map[string]any{"type": "string", "enum": "a"}
		errs := CheckSchemaVocabulary(schema)
		if assert.NotEmpty(t, errs) {
			assert.Contains(t, errs[0].Error(), "enum")
		}
	})

	t.Run("oneOf branch as non-map is rejected", func(t *testing.T) {
		schema := map[string]any{"oneOf": []any{"not a map"}}
		errs := CheckSchemaVocabulary(schema)
		if assert.NotEmpty(t, errs) {
			assert.Contains(t, errs[0].Error(), "oneOf")
		}
	})
}

func TestOneOfNoMatchErrorHasFieldLevelHint(t *testing.T) {
	intProp := map[string]any{"type": "integer"}
	strProp := map[string]any{"type": "string"}
	schema := map[string]any{"oneOf": []any{
		schemaFor(map[string]any{"a": intProp}, "a"),
		schemaFor(map[string]any{"b": strProp}, "b"),
	}}
	errs := ValidateNodeConfig(schema, map[string]any{"c": "x"})
	if assert.NotEmpty(t, errs) {
		found := false
		for _, e := range errs {
			if strings.Contains(e.Error(), "does not match any allowed variant") &&
				(strings.Contains(e.Error(), "missing required config field") || strings.Contains(e.Error(), "closest variant")) {
				found = true
			}
		}
		assert.True(t, found, "want oneOf error to include field-level detail, got %v", errs)
	}
}
