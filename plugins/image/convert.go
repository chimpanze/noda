package image

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/h2non/bimg"
)

type convertDescriptor struct{}

func (d *convertDescriptor) Name() string                           { return "convert" }
func (d *convertDescriptor) Description() string                    { return "Converts an image to a different format" }
func (d *convertDescriptor) ServiceDeps() map[string]api.ServiceDep { return imageServiceDeps }
func (d *convertDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input":   map[string]any{"type": "string", "description": "Source image path"},
			"output":  map[string]any{"type": "string", "description": "Output image path"},
			"format":  map[string]any{"type": "string", "description": "Target format: jpeg, png, webp, gif"},
			"quality": map[string]any{"type": "number", "description": "JPEG quality (1-100)"},
		},
		"required": []any{"input", "output", "format"},
	}
}
func (d *convertDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Converted image bytes",
		"error":   "Image processing error",
	}
}

type convertExecutor struct{}

func newConvertExecutor(_ map[string]any) api.NodeExecutor { return &convertExecutor{} }

func (e *convertExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *convertExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	data, err := readSourceImage(ctx, services, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("image.convert: %w", err)
	}

	format, err := plugin.ResolveString(nCtx, config, "format")
	if err != nil {
		return "", nil, fmt.Errorf("image.convert: %w", err)
	}

	imgType := parseFormat(format)
	if imgType == bimg.UNKNOWN {
		return "", nil, fmt.Errorf("image.convert: unsupported format %q", format)
	}

	opts := bimg.Options{
		Type: imgType,
	}

	if quality, ok, _ := plugin.ResolveOptionalInt(nCtx, config, "quality"); ok {
		opts.Quality = quality
	}

	result, err := bimg.NewImage(data).Process(opts)
	if err != nil {
		return "", nil, fmt.Errorf("image.convert: process: %w", err)
	}

	meta, err := writeTargetImage(ctx, services, nCtx, config, result)
	if err != nil {
		return "", nil, fmt.Errorf("image.convert: %w", err)
	}

	return api.OutputSuccess, meta, nil
}
