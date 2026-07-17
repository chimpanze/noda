package cache

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
		{"cache.del", (&delDescriptor{}).ConfigSchema(),
			map[string]any{"key": "{{ input.key }}"}, false,
			map[string]any{"key": true}},
		{"cache.exists", (&existsDescriptor{}).ConfigSchema(),
			map[string]any{"key": "{{ input.key }}"}, false,
			map[string]any{"key": true}},
		{"cache.get", (&getDescriptor{}).ConfigSchema(),
			map[string]any{"key": "{{ input.key }}"}, false,
			map[string]any{"key": true}},
		{"cache.set", (&setDescriptor{}).ConfigSchema(),
			map[string]any{"key": "{{ input.key }}", "value": "{{ input.value }}"}, false,
			map[string]any{"key": true, "value": "v"}},
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
