package image

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/h2non/bimg"
)

// Common service deps: source and target storage.
var imageServiceDeps = map[string]api.ServiceDep{
	"source": {Prefix: "storage", Required: true},
	"target": {Prefix: "storage", Required: true},
}

func getStorageService(services map[string]any, slot string) (api.StorageService, error) {
	svc, ok := services[slot]
	if !ok {
		return nil, fmt.Errorf("storage service %q not configured", slot)
	}
	ss, ok := svc.(api.StorageService)
	if !ok {
		return nil, fmt.Errorf("service %q does not implement StorageService", slot)
	}
	return ss, nil
}

func resolveString(nCtx api.ExecutionContext, config map[string]any, key string) (string, error) {
	raw, ok := config[key]
	if !ok {
		return "", fmt.Errorf("missing required field %q", key)
	}
	expr, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("field %q must be a string", key)
	}
	val, err := nCtx.Resolve(expr)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", key, err)
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("field %q resolved to %T, expected string", key, val)
	}
	return s, nil
}

func resolveInt(nCtx api.ExecutionContext, config map[string]any, key string) (int, bool, error) {
	raw, ok := config[key]
	if !ok {
		return 0, false, nil
	}
	switch v := raw.(type) {
	case float64:
		return int(v), true, nil
	case int:
		return v, true, nil
	case string:
		val, err := nCtx.Resolve(v)
		if err != nil {
			return 0, false, fmt.Errorf("resolve %q: %w", key, err)
		}
		switch n := val.(type) {
		case float64:
			return int(n), true, nil
		case int:
			return n, true, nil
		}
		return 0, false, fmt.Errorf("field %q resolved to %T, expected int", key, val)
	}
	return 0, false, fmt.Errorf("field %q has invalid type %T", key, raw)
}

// readSourceImage reads an image from source storage and returns its bytes.
func readSourceImage(ctx context.Context, services map[string]any, nCtx api.ExecutionContext, config map[string]any) ([]byte, error) {
	source, err := getStorageService(services, "source")
	if err != nil {
		return nil, err
	}
	inputPath, err := resolveString(nCtx, config, "input")
	if err != nil {
		return nil, err
	}
	data, err := source.Read(ctx, inputPath)
	if err != nil {
		return nil, fmt.Errorf("read source image %q: %w", inputPath, err)
	}
	return data, nil
}

// writeTargetImage writes processed image data to target storage and returns metadata.
func writeTargetImage(ctx context.Context, services map[string]any, nCtx api.ExecutionContext, config map[string]any, data []byte) (map[string]any, error) {
	target, err := getStorageService(services, "target")
	if err != nil {
		return nil, err
	}
	outputPath, err := resolveString(nCtx, config, "output")
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
