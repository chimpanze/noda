package image

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/h2non/bimg"
)

type resizeDescriptor struct{}

func (d *resizeDescriptor) Name() string                           { return "resize" }
func (d *resizeDescriptor) Description() string                    { return "Resizes an image to specified dimensions" }
func (d *resizeDescriptor) ServiceDeps() map[string]api.ServiceDep { return imageServiceDeps }
func (d *resizeDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input":   map[string]any{"type": "string", "description": "Source image path"},
			"output":  map[string]any{"type": "string", "description": "Output image path"},
			"width":   map[string]any{"type": "number", "description": "Target width in pixels"},
			"height":  map[string]any{"type": "number", "description": "Target height in pixels"},
			"quality": map[string]any{"type": "number", "description": "JPEG quality (1-100)"},
			"format":  map[string]any{"type": "string", "description": "Output format: jpeg, png, webp"},
		},
		"required": []any{"input", "output", "width", "height"},
	}
}
func (d *resizeDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Resized image bytes",
		"error":   "Image processing error",
	}
}

type resizeExecutor struct{}

func newResizeExecutor(_ map[string]any) api.NodeExecutor { return &resizeExecutor{} }

func (e *resizeExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *resizeExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	data, err := readSourceImage(ctx, services, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("image.resize: %w", err)
	}

	width, _, err := plugin.ResolveOptionalInt(nCtx, config, "width")
	if err != nil {
		return "", nil, fmt.Errorf("image.resize: %w", err)
	}
	height, _, err := plugin.ResolveOptionalInt(nCtx, config, "height")
	if err != nil {
		return "", nil, fmt.Errorf("image.resize: %w", err)
	}

	opts := bimg.Options{
		Width:  width,
		Height: height,
	}

	if quality, ok, _ := plugin.ResolveOptionalInt(nCtx, config, "quality"); ok {
		opts.Quality = quality
	}
	if format, ok := config["format"].(string); ok {
		opts.Type = parseFormat(format)
	}

	result, err := bimg.NewImage(data).Process(opts)
	if err != nil {
		return "", nil, fmt.Errorf("image.resize: process: %w", err)
	}

	meta, err := writeTargetImage(ctx, services, nCtx, config, result)
	if err != nil {
		return "", nil, fmt.Errorf("image.resize: %w", err)
	}

	return api.OutputSuccess, meta, nil
}
