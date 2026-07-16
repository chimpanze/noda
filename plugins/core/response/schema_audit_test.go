package response

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
			"response.json", (&jsonDescriptor{}).ConfigSchema(),
			map[string]any{"status": 200, "body": map[string]any{"ok": true}}, true,
			map[string]any{"status": 200, "body": "x", "headers": "not-a-map"},
		},
		{
			"response.error", (&errorDescriptor{}).ConfigSchema(),
			map[string]any{"code": "NOT_FOUND", "message": "Task not found"}, false,
			map[string]any{"status": "500", "code": true, "message": "msg"},
		},
		{
			"response.file", (&fileDescriptor{}).ConfigSchema(),
			map[string]any{"data": "{{ nodes.read_report.data }}", "content_type": "application/pdf"}, false,
			map[string]any{"data": "x", "content_type": true},
		},
		{
			"response.redirect", (&redirectDescriptor{}).ConfigSchema(),
			map[string]any{"url": "/api/tasks/1"}, false,
			map[string]any{"url": true},
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
