package image

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	"github.com/h2non/bimg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestPNG creates a 200x200 PNG image for testing.
func createTestPNG(t *testing.T) []byte {
	t.Helper()
	// Create a 200x200 black image using bimg
	buf, err := bimg.NewImage(makeSolidPNG()).Process(bimg.Options{
		Width:  200,
		Height: 200,
		Enlarge: true,
	})
	require.NoError(t, err)
	// Verify dimensions
	sz, err := bimg.NewImage(buf).Size()
	require.NoError(t, err)
	require.Equal(t, 200, sz.Width)
	require.Equal(t, 200, sz.Height)
	return buf
}

// makeSolidPNG returns a minimal 1x1 red PNG image.
func makeSolidPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41,
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc,
		0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
}

func newTestServices(t *testing.T) (map[string]any, *storageplugin.Service) {
	t.Helper()
	p := &storageplugin.Plugin{}
	rawSvc, err := p.CreateService(map[string]any{"backend": "memory"})
	require.NoError(t, err)
	svc := rawSvc.(*storageplugin.Service)
	// Use same service for both source and target
	return map[string]any{"source": svc, "target": svc}, svc
}

func TestResize(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newResizeExecutor(nil)
	output, result, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "resized.png",
		"width":  float64(100),
		"height": float64(100),
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	meta := result.(map[string]any)
	assert.Equal(t, "resized.png", meta["path"])
	assert.NotZero(t, meta["size"])

	// Verify actual dimensions
	resizedData, err := svc.Read(ctx, "resized.png")
	require.NoError(t, err)
	size, err := bimg.NewImage(resizedData).Size()
	require.NoError(t, err)
	assert.LessOrEqual(t, size.Width, 100)
	assert.LessOrEqual(t, size.Height, 100)
}

func TestCrop(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newCropExecutor(nil)
	output, result, err := e.Execute(ctx, execCtx, map[string]any{
		"input":   "input.png",
		"output":  "cropped.png",
		"width":   float64(50),
		"height":  float64(50),
		"gravity": "center",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	meta := result.(map[string]any)
	assert.Equal(t, "cropped.png", meta["path"])

	croppedData, err := svc.Read(ctx, "cropped.png")
	require.NoError(t, err)
	size, err := bimg.NewImage(croppedData).Size()
	require.NoError(t, err)
	assert.Equal(t, 50, size.Width)
	assert.Equal(t, 50, size.Height)
}

func TestConvert_PNGtoWEBP(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newConvertExecutor(nil)
	output, result, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "output.webp",
		"format": "webp",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	meta := result.(map[string]any)
	assert.Equal(t, "output.webp", meta["path"])

	webpData, err := svc.Read(ctx, "output.webp")
	require.NoError(t, err)
	imgType := bimg.DetermineImageType(webpData)
	assert.Equal(t, bimg.WEBP, imgType)
}

func TestThumbnail(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newThumbnailExecutor(nil)
	output, result, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "input.png",
		"output": "thumb.png",
		"width":  float64(64),
		"height": float64(64),
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	meta := result.(map[string]any)
	assert.Equal(t, "thumb.png", meta["path"])

	thumbData, err := svc.Read(ctx, "thumb.png")
	require.NoError(t, err)
	size, err := bimg.NewImage(thumbData).Size()
	require.NoError(t, err)
	assert.Equal(t, 64, size.Width)
	assert.Equal(t, 64, size.Height)
}

func TestResize_WithQuality(t *testing.T) {
	services, svc := newTestServices(t)
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svc.Write(ctx, "input.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	// High quality
	e := newResizeExecutor(nil)
	_, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":   "input.png",
		"output":  "high.jpeg",
		"width":   float64(100),
		"height":  float64(100),
		"quality": float64(95),
		"format":  "jpeg",
	}, services)
	require.NoError(t, err)

	// Low quality
	_, _, err = e.Execute(ctx, execCtx, map[string]any{
		"input":   "input.png",
		"output":  "low.jpeg",
		"width":   float64(100),
		"height":  float64(100),
		"quality": float64(10),
		"format":  "jpeg",
	}, services)
	require.NoError(t, err)

	highData, _ := svc.Read(ctx, "high.jpeg")
	lowData, _ := svc.Read(ctx, "low.jpeg")
	assert.Greater(t, len(highData), len(lowData), "higher quality should produce larger file")
}

func TestDifferentSourceTarget(t *testing.T) {
	p := &storageplugin.Plugin{}

	rawA, err := p.CreateService(map[string]any{"backend": "memory"})
	require.NoError(t, err)
	svcA := rawA.(*storageplugin.Service)

	rawB, err := p.CreateService(map[string]any{"backend": "memory"})
	require.NoError(t, err)
	svcB := rawB.(*storageplugin.Service)

	services := map[string]any{"source": svcA, "target": svcB}
	ctx := context.Background()

	img := createTestPNG(t)
	require.NoError(t, svcA.Write(ctx, "original.png", img))

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newResizeExecutor(nil)
	output, _, err := e.Execute(ctx, execCtx, map[string]any{
		"input":  "original.png",
		"output": "resized.png",
		"width":  float64(50),
		"height": float64(50),
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	// Source should still have original, target should have resized
	_, err = svcA.Read(ctx, "original.png")
	require.NoError(t, err)

	_, err = svcB.Read(ctx, "resized.png")
	require.NoError(t, err)

	// Source should NOT have the resized image
	_, err = svcA.Read(ctx, "resized.png")
	assert.Error(t, err)
}
