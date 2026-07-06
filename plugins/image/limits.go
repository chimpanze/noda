package image

import (
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

const (
	defaultMaxDimension = 10000
	defaultMaxPixels    = 40_000_000 // ~40 megapixels
)

// enforceDimensionLimit rejects output dimensions that exceed the per-side or
// total-pixel ceiling, preventing decompression/allocation bombs. Caps default
// to 10000 px/side and 40 MP, overridable via max_width/max_height/max_pixels.
func enforceDimensionLimit(nCtx api.ExecutionContext, config map[string]any, width, height int) error {
	maxW := defaultMaxDimension
	if v, ok, _ := plugin.ResolveOptionalInt(nCtx, config, "max_width"); ok {
		maxW = v
	}
	maxH := defaultMaxDimension
	if v, ok, _ := plugin.ResolveOptionalInt(nCtx, config, "max_height"); ok {
		maxH = v
	}
	maxPx := int64(defaultMaxPixels)
	if v, ok, _ := plugin.ResolveOptionalInt(nCtx, config, "max_pixels"); ok {
		maxPx = int64(v)
	}
	if width > maxW || height > maxH {
		return fmt.Errorf("output dimensions %dx%d exceed limit (%dx%d)", width, height, maxW, maxH)
	}
	if int64(width)*int64(height) > maxPx {
		return fmt.Errorf("output area %d px exceeds limit (%d px)", int64(width)*int64(height), maxPx)
	}
	return nil
}
