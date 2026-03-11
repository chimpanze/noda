package image

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResize_MissingSourceService(t *testing.T) {
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newResizeExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "out.png",
		"width":  float64(100),
		"height": float64(100),
	}, map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.resize")
}

func TestResize_MissingInputFile(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newResizeExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "nonexistent.png",
		"output": "out.png",
		"width":  float64(100),
		"height": float64(100),
	}, services)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.resize")
}

func TestResize_Outputs(t *testing.T) {
	e := newResizeExecutor(nil)
	outputs := e.Outputs()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

func TestCrop_MissingSourceService(t *testing.T) {
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newCropExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "out.png",
		"width":  float64(50),
		"height": float64(50),
	}, map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.crop")
}

func TestCrop_MissingInputFile(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newCropExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "nonexistent.png",
		"output": "out.png",
		"width":  float64(50),
		"height": float64(50),
	}, services)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.crop")
}

func TestCrop_Outputs(t *testing.T) {
	e := newCropExecutor(nil)
	outputs := e.Outputs()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

func TestCrop_WithAllGravities(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	gravities := []string{"center", "centre", "north", "south", "east", "west", "smart"}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newCropExecutor(nil)

	for _, g := range gravities {
		t.Run(g, func(t *testing.T) {
			output, _, err := e.Execute(ctx, execCtx, map[string]any{
				"input":   "input.png",
				"output":  "cropped_" + g + ".png",
				"width":   float64(50),
				"height":  float64(50),
				"gravity": g,
			}, services)
			require.NoError(t, err)
			assert.Equal(t, "success", output)
		})
	}
}

func TestCrop_WithoutGravity(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newCropExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "cropped_default.png",
		"width":  float64(50),
		"height": float64(50),
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestConvert_MissingSourceService(t *testing.T) {
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newConvertExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "out.webp",
		"format": "webp",
	}, map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.convert")
}

func TestConvert_MissingInputFile(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newConvertExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "nonexistent.png",
		"output": "out.webp",
		"format": "webp",
	}, services)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.convert")
}

func TestConvert_MissingFormat(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newConvertExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "out.png",
	}, services)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field 'format'")
}

func TestConvert_UnsupportedFormat(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newConvertExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "out.bmp",
		"format": "bmp",
	}, services)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestConvert_Outputs(t *testing.T) {
	e := newConvertExecutor(nil)
	outputs := e.Outputs()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

func TestConvert_WithQuality(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newConvertExecutor(nil)
	output, result, err := e.Execute(ctx, execCtx, map[string]any{
		"input":   "input.png",
		"output":  "out.jpeg",
		"format":  "jpeg",
		"quality": float64(50),
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	meta := result.(map[string]any)
	assert.Equal(t, "out.jpeg", meta["path"])
}

func TestConvert_PNGtoJPEG(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newConvertExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "out.jpg",
		"format": "jpg",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestConvert_PNGtoTIFF(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newConvertExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "out.tiff",
		"format": "tiff",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestThumbnail_MissingSourceService(t *testing.T) {
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newThumbnailExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "thumb.png",
		"width":  float64(64),
		"height": float64(64),
	}, map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.thumbnail")
}

func TestThumbnail_MissingInputFile(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newThumbnailExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "nonexistent.png",
		"output": "thumb.png",
		"width":  float64(64),
		"height": float64(64),
	}, services)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.thumbnail")
}

func TestThumbnail_Outputs(t *testing.T) {
	e := newThumbnailExecutor(nil)
	outputs := e.Outputs()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

func TestResize_WithFormat(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	formats := []string{"jpeg", "png", "webp", "tiff"}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newResizeExecutor(nil)

	for _, f := range formats {
		t.Run(f, func(t *testing.T) {
			output, _, err := e.Execute(ctx, execCtx, map[string]any{
				"input":  "input.png",
				"output": "resized_" + f + "." + f,
				"width":  float64(100),
				"height": float64(100),
				"format": f,
			}, services)
			require.NoError(t, err)
			assert.Equal(t, "success", output)
		})
	}
}

func TestResize_MetaContainsDimensions(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newResizeExecutor(nil)
	_, result, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "resized.png",
		"width":  float64(80),
		"height": float64(80),
	}, services)
	require.NoError(t, err)

	meta := result.(map[string]any)
	assert.Contains(t, meta, "width")
	assert.Contains(t, meta, "height")
	assert.Contains(t, meta, "path")
	assert.Contains(t, meta, "size")
}
