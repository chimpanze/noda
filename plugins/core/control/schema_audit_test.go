package control

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
			"control.if", (&ifDescriptor{}).ConfigSchema(),
			map[string]any{"condition": "{{ len(nodes.fetch) > 0 }}"}, false,
			map[string]any{"condition": true},
		},
		{
			"control.loop", (&loopDescriptor{}).ConfigSchema(),
			map[string]any{"collection": "{{ nodes.fetch }}", "workflow": "process-item", "max_items": float64(500)}, false,
			map[string]any{"collection": 5, "workflow": "process-item", "max_items": "not-a-number"},
		},
		{
			"control.switch", (&switchDescriptor{}).ConfigSchema(),
			map[string]any{"expression": "{{ input.action }}", "cases": []any{"create", "update", "delete"}}, false,
			map[string]any{"expression": "{{ input.action }}", "cases": "not-an-array"},
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
