package storage

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
		{"storage.delete", (&deleteDescriptor{}).ConfigSchema(),
			map[string]any{"path": "{{ input.path }}"}, false,
			map[string]any{"path": true}},
		{"storage.list", (&listDescriptor{}).ConfigSchema(),
			map[string]any{"prefix": "{{ input.prefix }}"}, false,
			map[string]any{"prefix": true}},
		{"storage.read", (&readDescriptor{}).ConfigSchema(),
			map[string]any{"path": "{{ input.path }}"}, false,
			map[string]any{"path": true}},
		{"storage.write", (&writeDescriptor{}).ConfigSchema(),
			map[string]any{"path": "{{ input.path }}", "data": "{{ input.data }}"}, false,
			map[string]any{"path": true, "data": "x"}},
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
