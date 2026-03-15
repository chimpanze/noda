package image

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/h2non/bimg"
)

type watermarkDescriptor struct{}

func (d *watermarkDescriptor) Name() string                           { return "watermark" }
func (d *watermarkDescriptor) Description() string                    { return "Applies a watermark overlay to an image" }
func (d *watermarkDescriptor) ServiceDeps() map[string]api.ServiceDep { return imageServiceDeps }
func (d *watermarkDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input":     map[string]any{"type": "string", "description": "Source image path"},
			"output":    map[string]any{"type": "string", "description": "Output image path"},
			"watermark": map[string]any{"type": "string", "description": "Watermark image path"},
			"opacity":   map[string]any{"type": "number", "description": "Opacity from 0 to 1"},
			"position":  map[string]any{"type": "string", "description": "Position: center, top-left, top-right, bottom-left, bottom-right"},
		},
		"required": []any{"input", "output", "watermark"},
	}
}
func (d *watermarkDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Watermarked image bytes",
		"error":   "Image processing error",
	}
}

type watermarkExecutor struct{}

func newWatermarkExecutor(_ map[string]any) api.NodeExecutor { return &watermarkExecutor{} }

func (e *watermarkExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *watermarkExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	data, err := readSourceImage(ctx, services, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("image.watermark: %w", err)
	}

	// Read watermark image from source storage
	source, err := getStorageService(services, "source")
	if err != nil {
		return "", nil, fmt.Errorf("image.watermark: %w", err)
	}
	wmPath, err := plugin.ResolveString(nCtx, config, "watermark")
	if err != nil {
		return "", nil, fmt.Errorf("image.watermark: %w", err)
	}
	wmData, err := source.Read(ctx, wmPath)
	if err != nil {
		return "", nil, fmt.Errorf("image.watermark: read watermark %q: %w", wmPath, err)
	}

	// bimg's watermark support is text-based; for image overlay we use
	// a two-step approach: resize watermark, then overlay with vips.
	// bimg.WatermarkImage provides image-based watermarking.
	opacity := 1.0
	if raw, ok := config["opacity"]; ok {
		switch v := raw.(type) {
		case float64:
			opacity = v
		case int:
			opacity = float64(v)
		case string:
			val, err := nCtx.Resolve(v)
			if err == nil {
				if f, ok := val.(float64); ok {
					opacity = f
				}
			}
		}
	}

	wmImg := bimg.WatermarkImage{
		Buf:     wmData,
		Opacity: float32(opacity),
	}

	// Apply position via Left/Top offsets
	if pos, ok := config["position"].(string); ok {
		imgSize, sizeErr := bimg.NewImage(data).Size()
		wmSize, wmSizeErr := bimg.NewImage(wmData).Size()
		if sizeErr == nil && wmSizeErr == nil {
			left, top := calculatePosition(pos, imgSize, wmSize)
			wmImg.Left = left
			wmImg.Top = top
		}
	}

	result, err := bimg.NewImage(data).WatermarkImage(wmImg)
	if err != nil {
		return "", nil, fmt.Errorf("image.watermark: process: %w", err)
	}

	meta, err := writeTargetImage(ctx, services, nCtx, config, result)
	if err != nil {
		return "", nil, fmt.Errorf("image.watermark: %w", err)
	}

	return api.OutputSuccess, meta, nil
}

// calculatePosition returns Left, Top offsets for watermark placement.
func calculatePosition(position string, img, wm bimg.ImageSize) (int, int) {
	switch position {
	case "top-left":
		return 0, 0
	case "top-right":
		return img.Width - wm.Width, 0
	case "bottom-left":
		return 0, img.Height - wm.Height
	case "bottom-right":
		return img.Width - wm.Width, img.Height - wm.Height
	case "center":
		return (img.Width - wm.Width) / 2, (img.Height - wm.Height) / 2
	default:
		return (img.Width - wm.Width) / 2, (img.Height - wm.Height) / 2
	}
}
