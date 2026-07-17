package http

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
		{"http.get", (&getDescriptor{}).ConfigSchema(),
			map[string]any{"url": "{{ input.url }}", "body": "{{ input.payload }}"}, false,
			map[string]any{"url": "/x", "headers": "not-an-object"}},
		{"http.post", (&postDescriptor{}).ConfigSchema(),
			map[string]any{"url": "{{ input.url }}"}, false,
			map[string]any{"headers": map[string]any{"X": "y"}}},
		{"http.request", (&requestDescriptor{}).ConfigSchema(),
			map[string]any{"method": "GET", "url": "{{ input.url }}"}, false,
			map[string]any{"url": "/x"}},
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
