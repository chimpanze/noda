package transform

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
)

func TestConfigSchemasMatchExecutors(t *testing.T) {
	tests := []struct {
		nodeType     string
		schema       map[string]any
		minimalValid map[string]any // smallest config the executor accepts (from docs example)
		emptyValid   bool           // does the executor run with config {}?
		invalid      map[string]any // one config the executor would reject/misuse
	}{
		{
			"transform.delete", (&deleteDescriptor{}).ConfigSchema(),
			// fields is optional: a missing "fields" is a no-op (deletes nothing),
			// so the minimal valid config omits it to exercise that.
			map[string]any{"data": "{{ nodes.x }}"},
			false,
			map[string]any{"data": 123, "fields": []any{"a"}},
		},
		{
			"transform.filter", (&filterDescriptor{}).ConfigSchema(),
			map[string]any{"collection": "{{ nodes.x }}", "expression": "{{ $item.ok }}"},
			false,
			map[string]any{"collection": 5, "expression": "{{ $item.ok }}"},
		},
		{
			"transform.map", (&mapDescriptor{}).ConfigSchema(),
			map[string]any{"collection": "{{ nodes.x }}", "expression": "{{ $item }}"},
			false,
			map[string]any{"collection": true, "expression": "{{ $item }}"},
		},
		{
			"transform.merge", (&mergeDescriptor{}).ConfigSchema(),
			// exercises the added match.fields.left/right properties.
			map[string]any{
				"mode":   "match",
				"inputs": []any{"{{ nodes.a }}", "{{ nodes.b }}"},
				"match": map[string]any{
					"type":   "inner",
					"fields": map[string]any{"left": "id", "right": "id"},
				},
			},
			false,
			map[string]any{"mode": "bogus", "inputs": []any{"{{ nodes.a }}"}},
		},
		{
			"transform.set", (&setDescriptor{}).ConfigSchema(),
			// exercises additionalProperties:true (arbitrary user-defined keys).
			map[string]any{"fields": map[string]any{"name": "{{ input.name }}", "anything_here": "literal"}},
			false,
			map[string]any{"fields": "not-an-object"},
		},
		{
			"transform.validate", (&validateDescriptor{}).ConfigSchema(),
			// exercises additionalProperties:true on the arbitrary JSON Schema doc.
			map[string]any{"schema": map[string]any{"type": "object", "properties": map[string]any{"foo": map[string]any{"type": "string"}}}},
			false,
			map[string]any{"schema": "not-an-object"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			assert.Empty(t, registry.CheckSchemaVocabulary(tt.schema))
			assert.Empty(t, registry.ValidateNodeConfig(tt.schema, tt.minimalValid), "minimal valid config must pass")
			emptyErrs := registry.ValidateNodeConfig(tt.schema, map[string]any{})
			if tt.emptyValid {
				assert.Empty(t, emptyErrs, "executor accepts {}, schema must too")
			} else {
				assert.NotEmpty(t, emptyErrs, "executor rejects {}, schema must too")
			}
			assert.NotEmpty(t, registry.ValidateNodeConfig(tt.schema, tt.invalid))
		})
	}
}
