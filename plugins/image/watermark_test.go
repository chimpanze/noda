package image

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/h2non/bimg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWatermark_BasicOverlay(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "base.png", img))

	// Create a small watermark image
	wmData := createSmallPNG(t)
	require.NoError(t, svc.Write(ctx, "wm.png", wmData))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newWatermarkExecutor(nil)
	output, result, err := e.Execute(ctx, execCtx, map[string]any{
		"input":     "base.png",
		"output":    "watermarked.png",
		"watermark": "wm.png",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	meta := result.(map[string]any)
	assert.Equal(t, "watermarked.png", meta["path"])
	assert.NotZero(t, meta["size"])
}

func TestWatermark_WithOpacityFloat(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "base.png", img))
	wmData := createSmallPNG(t)
	require.NoError(t, svc.Write(ctx, "wm.png", wmData))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newWatermarkExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":     "base.png",
		"output":    "wm_opacity.png",
		"watermark": "wm.png",
		"opacity":   float64(0.5),
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestWatermark_WithOpacityInt(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "base.png", img))
	wmData := createSmallPNG(t)
	require.NoError(t, svc.Write(ctx, "wm.png", wmData))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newWatermarkExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":     "base.png",
		"output":    "wm_int.png",
		"watermark": "wm.png",
		"opacity":   int(1),
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestWatermark_WithOpacityString(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "base.png", img))
	wmData := createSmallPNG(t)
	require.NoError(t, svc.Write(ctx, "wm.png", wmData))

	// Use a string opacity that won't resolve (expression), so it stays at default 1.0
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"myOpacity": float64(0.8)}))
	e := newWatermarkExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":     "base.png",
		"output":    "wm_str.png",
		"watermark": "wm.png",
		"opacity":   "not_a_valid_expr",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestWatermark_WithPosition_TopLeft(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "base.png", img))
	wmData := createSmallPNG(t)
	require.NoError(t, svc.Write(ctx, "wm.png", wmData))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newWatermarkExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":     "base.png",
		"output":    "wm_tl.png",
		"watermark": "wm.png",
		"position":  "top-left",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestWatermark_WithPosition_BottomRight(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "base.png", img))
	wmData := createSmallPNG(t)
	require.NoError(t, svc.Write(ctx, "wm.png", wmData))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newWatermarkExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":     "base.png",
		"output":    "wm_br.png",
		"watermark": "wm.png",
		"position":  "bottom-right",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestWatermark_WithPosition_Center(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "base.png", img))
	wmData := createSmallPNG(t)
	require.NoError(t, svc.Write(ctx, "wm.png", wmData))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newWatermarkExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":     "base.png",
		"output":    "wm_center.png",
		"watermark": "wm.png",
		"position":  "center",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestWatermark_Outputs(t *testing.T) {
	e := newWatermarkExecutor(nil)
	outputs := e.Outputs()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

func TestWatermark_MissingSourceService(t *testing.T) {
	ctx := context.Background()
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newWatermarkExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":     "base.png",
		"output":    "out.png",
		"watermark": "wm.png",
	}, map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.watermark")
}

func TestWatermark_MissingWatermarkFile(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "base.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newWatermarkExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":     "base.png",
		"output":    "out.png",
		"watermark": "nonexistent.png",
	}, services)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.watermark")
}

func TestWatermark_MissingInputFile(t *testing.T) {
	services, _ := newTestServices(t)
	ctx := context.Background()

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newWatermarkExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":     "nonexistent.png",
		"output":    "out.png",
		"watermark": "wm.png",
	}, services)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "image.watermark")
}

// createSmallPNG creates a small 20x20 PNG for use as watermark.
func createSmallPNG(t *testing.T) []byte {
	t.Helper()
	return createTestPNGWithSize(t, 20, 20)
}

// createTestPNGWithSize creates a PNG of specified dimensions.
func createTestPNGWithSize(t *testing.T, width, height int) []byte {
	t.Helper()
	buf, err := newTestImageProcessor().Process(bimg.Options{
		Width:   width,
		Height:  height,
		Enlarge: true,
	})
	require.NoError(t, err)
	return buf
}

// newTestImageProcessor returns a bimg.Image from a solid PNG seed.
func newTestImageProcessor() *bimg.Image {
	return bimg.NewImage(makeSolidPNG())
}
