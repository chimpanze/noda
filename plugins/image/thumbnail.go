package image

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/h2non/bimg"
)

type thumbnailDescriptor struct{}

func (d *thumbnailDescriptor) Name() string { return "thumbnail" }
func (d *thumbnailDescriptor) Description() string {
	return "Smart crop and resize to exact dimensions"
}
func (d *thumbnailDescriptor) ServiceDeps() map[string]api.ServiceDep { return imageServiceDeps }
func (d *thumbnailDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input":  map[string]any{"type": "string", "description": "Source image path"},
			"output": map[string]any{"type": "string", "description": "Output image path"},
			"width":  map[string]any{"type": "number", "description": "Thumbnail width in pixels"},
			"height": map[string]any{"type": "number", "description": "Thumbnail height in pixels"},
		},
		"required": []any{"input", "output", "width", "height"},
	}
}
func (d *thumbnailDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Thumbnail image bytes",
		"error":   "Image processing error",
	}
}

type thumbnailExecutor struct{}

func newThumbnailExecutor(_ map[string]any) api.NodeExecutor { return &thumbnailExecutor{} }

func (e *thumbnailExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *thumbnailExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	data, err := readSourceImage(ctx, services, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("image.thumbnail: %w", err)
	}

	width, _, err := plugin.ResolveOptionalInt(nCtx, config, "width")
	if err != nil {
		return "", nil, fmt.Errorf("image.thumbnail: %w", err)
	}
	height, _, err := plugin.ResolveOptionalInt(nCtx, config, "height")
	if err != nil {
		return "", nil, fmt.Errorf("image.thumbnail: %w", err)
	}

	// Thumbnail: crop to exact dimensions with smart gravity
	opts := bimg.Options{
		Width:   width,
		Height:  height,
		Crop:    true,
		Gravity: bimg.GravitySmart,
	}

	result, err := bimg.NewImage(data).Process(opts)
	if err != nil {
		return "", nil, fmt.Errorf("image.thumbnail: process: %w", err)
	}

	meta, err := writeTargetImage(ctx, services, nCtx, config, result)
	if err != nil {
		return "", nil, fmt.Errorf("image.thumbnail: %w", err)
	}

	return api.OutputSuccess, meta, nil
}
