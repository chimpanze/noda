package util

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
		{"util.delay", (&delayDescriptor{}).ConfigSchema(),
			map[string]any{"timeout": "2s"}, false,
			map[string]any{"timeout": 5}},
		{"util.jwt_sign", (&jwtSignDescriptor{}).ConfigSchema(),
			map[string]any{"claims": map[string]any{"sub": "{{ input.user_id }}"}, "secret": "{{ secrets.JWT_SECRET }}"}, false,
			map[string]any{"claims": "not-an-object", "secret": "s"}},
		{"util.log", (&logDescriptor{}).ConfigSchema(),
			map[string]any{"level": "info", "message": "Order created: {{ nodes.insert.id }}"}, false,
			map[string]any{"level": "info", "message": true}},
		{"util.timestamp", (&timestampDescriptor{}).ConfigSchema(),
			map[string]any{"format": "unix_ms"}, true,
			map[string]any{"format": "not-a-format"}},
		{"util.uuid", (&uuidDescriptor{}).ConfigSchema(),
			map[string]any{}, true,
			map[string]any{"extra": "unexpected"}},
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
