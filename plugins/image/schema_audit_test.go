package image

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
		{"image.convert", (&convertDescriptor{}).ConfigSchema(),
			map[string]any{"input": "{{ input.path }}", "output": "out.jpg", "format": "jpeg", "quality": 85}, false,
			map[string]any{"input": "in.png", "output": "out.jpg"}},
		{"image.crop", (&cropDescriptor{}).ConfigSchema(),
			map[string]any{
				"input": "{{ input.path }}", "output": "out.jpg", "width": 100, "height": 100,
				"max_width": 5000, "max_height": 5000, "max_pixels": 10_000_000,
			}, false,
			map[string]any{"input": "in.png", "output": "out.jpg"}},
		{"image.resize", (&resizeDescriptor{}).ConfigSchema(),
			map[string]any{
				"input": "{{ input.path }}", "output": "out.jpg", "width": 100, "height": 100,
				"max_width": 5000, "max_height": 5000, "max_pixels": 10_000_000,
			}, false,
			map[string]any{"input": "in.png", "output": "out.jpg"}},
		{"image.thumbnail", (&thumbnailDescriptor{}).ConfigSchema(),
			map[string]any{
				"input": "{{ input.path }}", "output": "out.jpg", "width": 100, "height": 100,
				"max_width": 5000, "max_height": 5000, "max_pixels": 10_000_000,
			}, false,
			map[string]any{"input": "in.png", "output": "out.jpg"}},
		{"image.watermark", (&watermarkDescriptor{}).ConfigSchema(),
			map[string]any{"input": "{{ input.path }}", "output": "out.jpg", "watermark": "wm.png"}, false,
			map[string]any{"input": "in.png", "output": "out.jpg"}},
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
