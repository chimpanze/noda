package image

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/h2non/bimg"
)

type cropDescriptor struct{}

func (d *cropDescriptor) Name() string                          { return "crop" }
func (d *cropDescriptor) ServiceDeps() map[string]api.ServiceDep { return imageServiceDeps }
func (d *cropDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input":   map[string]any{"type": "string"},
			"output":  map[string]any{"type": "string"},
			"width":   map[string]any{"type": "number"},
			"height":  map[string]any{"type": "number"},
			"gravity": map[string]any{"type": "string"},
		},
		"required": []any{"input", "output", "width", "height"},
	}
}

type cropExecutor struct{}

func newCropExecutor(_ map[string]any) api.NodeExecutor { return &cropExecutor{} }

func (e *cropExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *cropExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	data, err := readSourceImage(ctx, services, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("image.crop: %w", err)
	}

	width, _, err := resolveInt(nCtx, config, "width")
	if err != nil {
		return "", nil, fmt.Errorf("image.crop: %w", err)
	}
	height, _, err := resolveInt(nCtx, config, "height")
	if err != nil {
		return "", nil, fmt.Errorf("image.crop: %w", err)
	}

	gravity := bimg.GravityCentre
	if g, ok := config["gravity"].(string); ok {
		gravity = parseGravity(g)
	}

	opts := bimg.Options{
		Width:   width,
		Height:  height,
		Crop:    true,
		Gravity: gravity,
	}

	result, err := bimg.NewImage(data).Process(opts)
	if err != nil {
		return "", nil, fmt.Errorf("image.crop: process: %w", err)
	}

	meta, err := writeTargetImage(ctx, services, nCtx, config, result)
	if err != nil {
		return "", nil, fmt.Errorf("image.crop: %w", err)
	}

	return "success", meta, nil
}
