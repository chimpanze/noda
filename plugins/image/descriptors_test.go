package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResizeDescriptor(t *testing.T) {
	d := &resizeDescriptor{}
	assert.Equal(t, "resize", d.Name())
	assert.NotNil(t, d.ServiceDeps())
	assert.Contains(t, d.ServiceDeps(), "source")
	assert.Contains(t, d.ServiceDeps(), "target")

	schema := d.ConfigSchema()
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "input")
	assert.Contains(t, props, "output")
	assert.Contains(t, props, "width")
	assert.Contains(t, props, "height")
	assert.Contains(t, props, "quality")
	assert.Contains(t, props, "format")
}

func TestCropDescriptor(t *testing.T) {
	d := &cropDescriptor{}
	assert.Equal(t, "crop", d.Name())
	assert.NotNil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "gravity")
}

func TestWatermarkDescriptor(t *testing.T) {
	d := &watermarkDescriptor{}
	assert.Equal(t, "watermark", d.Name())
	assert.NotNil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "watermark")
	assert.Contains(t, props, "opacity")
	assert.Contains(t, props, "position")

	required := schema["required"].([]any)
	assert.Contains(t, required, "input")
	assert.Contains(t, required, "output")
	assert.Contains(t, required, "watermark")
}

func TestConvertDescriptor(t *testing.T) {
	d := &convertDescriptor{}
	assert.Equal(t, "convert", d.Name())
	assert.NotNil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "format")
	assert.Contains(t, props, "quality")
}

func TestThumbnailDescriptor(t *testing.T) {
	d := &thumbnailDescriptor{}
	assert.Equal(t, "thumbnail", d.Name())
	assert.NotNil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "width")
	assert.Contains(t, props, "height")
}
