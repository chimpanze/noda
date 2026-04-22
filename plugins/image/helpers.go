package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"  // header decoder
	_ "image/jpeg" // header decoder
	_ "image/png"  // header decoder

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/h2non/bimg"
)

const (
	// maxImageBytes is the hard ceiling on input file size before any decode.
	maxImageBytes = 20 << 20 // 20 MiB

	// maxImagePixels is the hard ceiling on pixel count (width * height).
	// 50 megapixels accommodates 8K-class images with headroom.
	maxImagePixels = 50_000_000
)

// validateImageInput rejects inputs that exceed the byte-size or pixel-count
// ceilings, before the bytes reach libvips. Returns *api.ValidationError on
// rejection.
func validateImageInput(data []byte) error {
	if len(data) > maxImageBytes {
		return &api.ValidationError{
			Field:   "input",
			Message: fmt.Sprintf("image exceeds %d bytes (got %d)", maxImageBytes, len(data)),
		}
	}

	// Stdlib header parse for PNG/JPEG/GIF (no pixel decode).
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err == nil {
		if int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
			return &api.ValidationError{
				Field:   "input",
				Message: fmt.Sprintf("image dimensions %dx%d exceed %d pixels", cfg.Width, cfg.Height, maxImagePixels),
			}
		}
		return nil
	}

	// Fallback: WebP/AVIF/TIFF — let libvips read the header.
	// bimg.Size() invokes vips_image_new_from_buffer, which is metadata-only
	// for headers it understands.
	size, sizeErr := bimg.NewImage(data).Size()
	if sizeErr != nil {
		return &api.ValidationError{
			Field:   "input",
			Message: fmt.Sprintf("unrecognised image format: %v", sizeErr),
		}
	}
	if int64(size.Width)*int64(size.Height) > maxImagePixels {
		return &api.ValidationError{
			Field:   "input",
			Message: fmt.Sprintf("image dimensions %dx%d exceed %d pixels", size.Width, size.Height, maxImagePixels),
		}
	}
	return nil
}

// Common service deps: source and target storage.
var imageServiceDeps = map[string]api.ServiceDep{
	"source": {Prefix: "storage", Required: true},
	"target": {Prefix: "storage", Required: true},
}

func getStorageService(services map[string]any, slot string) (api.StorageService, error) {
	return plugin.GetService[api.StorageService](services, slot)
}

// readSourceImage reads an image from source storage and returns its bytes.
func readSourceImage(ctx context.Context, services map[string]any, nCtx api.ExecutionContext, config map[string]any) ([]byte, error) {
	source, err := getStorageService(services, "source")
	if err != nil {
		return nil, err
	}
	inputPath, err := plugin.ResolveString(nCtx, config, "input")
	if err != nil {
		return nil, err
	}
	data, err := source.Read(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("read source image %q: %w", inputPath, err)
	}
	if err := validateImageInput(data); err != nil {
		return nil, err
	}
	return data, nil
}

// writeTargetImage writes processed image data to target storage and returns metadata.
func writeTargetImage(ctx context.Context, services map[string]any, nCtx api.ExecutionContext, config map[string]any, data []byte) (map[string]any, error) {
	target, err := getStorageService(services, "target")
	if err != nil {
		return nil, err
	}
	outputPath, err := plugin.ResolveString(nCtx, config, "output")
	if err != nil {
		return nil, err
	}
	if err := target.Write(ctx, outputPath, data); err != nil {
		return nil, fmt.Errorf("write target image %q: %w", outputPath, err)
	}

	// Read dimensions from output image
	size := bimg.NewImage(data).Length()
	imgSize, err := bimg.NewImage(data).Size()
	if err != nil {
		// Non-fatal: return path and size without dimensions
		return map[string]any{
			"path": outputPath,
			"size": size,
		}, nil
	}

	return map[string]any{
		"path":   outputPath,
		"width":  imgSize.Width,
		"height": imgSize.Height,
		"size":   size,
	}, nil
}

// parseFormat converts a format string to a bimg.ImageType.
func parseFormat(format string) bimg.ImageType {
	switch format {
	case "jpeg", "jpg":
		return bimg.JPEG
	case "png":
		return bimg.PNG
	case "webp":
		return bimg.WEBP
	case "avif":
		return bimg.AVIF
	case "tiff":
		return bimg.TIFF
	case "gif":
		return bimg.GIF
	default:
		return bimg.UNKNOWN
	}
}

// parseGravity converts a gravity string to bimg.Gravity.
func parseGravity(gravity string) bimg.Gravity {
	switch gravity {
	case "center", "centre":
		return bimg.GravityCentre
	case "north":
		return bimg.GravityNorth
	case "south":
		return bimg.GravitySouth
	case "east":
		return bimg.GravityEast
	case "west":
		return bimg.GravityWest
	case "smart":
		return bimg.GravitySmart
	default:
		return bimg.GravityCentre
	}
}
