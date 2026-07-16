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
