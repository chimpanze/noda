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
func (d *resizeDescriptor) ServiceDeps() map[string]api.ServiceDep { return imageServiceDeps }
func (d *resizeDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input":   map[string]any{"type": "string"},
			"output":  map[string]any{"type": "string"},
			"width":   map[string]any{"type": "number"},
			"height":  map[string]any{"type": "number"},
			"quality": map[string]any{"type": "number"},
			"format":  map[string]any{"type": "string"},
		},
		"required": []any{"input", "output", "width", "height"},
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

	width, _, err := plugin.ResolveInt(nCtx, config, "width")
	if err != nil {
		return "", nil, fmt.Errorf("image.resize: %w", err)
	}
	height, _, err := plugin.ResolveInt(nCtx, config, "height")
	if err != nil {
		return "", nil, fmt.Errorf("image.resize: %w", err)
	}

	opts := bimg.Options{
		Width:  width,
		Height: height,
	}

	if quality, ok, _ := plugin.ResolveInt(nCtx, config, "quality"); ok {
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
