package image

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/h2non/bimg"
)

type thumbnailDescriptor struct{}

func (d *thumbnailDescriptor) Name() string                           { return "thumbnail" }
func (d *thumbnailDescriptor) ServiceDeps() map[string]api.ServiceDep { return imageServiceDeps }
func (d *thumbnailDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input":  map[string]any{"type": "string"},
			"output": map[string]any{"type": "string"},
			"width":  map[string]any{"type": "number"},
			"height": map[string]any{"type": "number"},
		},
		"required": []any{"input", "output", "width", "height"},
	}
}

type thumbnailExecutor struct{}

func newThumbnailExecutor(_ map[string]any) api.NodeExecutor { return &thumbnailExecutor{} }

func (e *thumbnailExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *thumbnailExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	data, err := readSourceImage(ctx, services, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("image.thumbnail: %w", err)
	}

	width, _, err := plugin.ResolveInt(nCtx, config, "width")
	if err != nil {
		return "", nil, fmt.Errorf("image.thumbnail: %w", err)
	}
	height, _, err := plugin.ResolveInt(nCtx, config, "height")
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

	return "success", meta, nil
}
