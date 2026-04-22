package image

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/h2non/bimg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input    string
		expected bimg.ImageType
	}{
		{"jpeg", bimg.JPEG},
		{"jpg", bimg.JPEG},
		{"png", bimg.PNG},
		{"webp", bimg.WEBP},
		{"avif", bimg.AVIF},
		{"tiff", bimg.TIFF},
		{"gif", bimg.GIF},
		{"unknown", bimg.UNKNOWN},
		{"bmp", bimg.UNKNOWN},
		{"", bimg.UNKNOWN},
		{"JPEG", bimg.UNKNOWN}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseFormat(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseGravity(t *testing.T) {
	tests := []struct {
		input    string
		expected bimg.Gravity
	}{
		{"center", bimg.GravityCentre},
		{"centre", bimg.GravityCentre},
		{"north", bimg.GravityNorth},
		{"south", bimg.GravitySouth},
		{"east", bimg.GravityEast},
		{"west", bimg.GravityWest},
		{"smart", bimg.GravitySmart},
		{"", bimg.GravityCentre},        // default
		{"invalid", bimg.GravityCentre}, // default
	}

	for _, tt := range tests {
		name := tt.input
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			result := parseGravity(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculatePosition(t *testing.T) {
	img := bimg.ImageSize{Width: 1000, Height: 800}
	wm := bimg.ImageSize{Width: 100, Height: 50}

	tests := []struct {
		position     string
		expectedLeft int
		expectedTop  int
	}{
		{"top-left", 0, 0},
		{"top-right", 900, 0},
		{"bottom-left", 0, 750},
		{"bottom-right", 900, 750},
		{"center", 450, 375},
		{"unknown", 450, 375},    // default is center
		{"", 450, 375},           // default is center
		{"top-center", 450, 375}, // not explicitly handled, falls to default
	}

	for _, tt := range tests {
		name := tt.position
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			left, top := calculatePosition(tt.position, img, wm)
			assert.Equal(t, tt.expectedLeft, left, "left offset")
			assert.Equal(t, tt.expectedTop, top, "top offset")
		})
	}
}

func TestCalculatePosition_SameSize(t *testing.T) {
	img := bimg.ImageSize{Width: 100, Height: 100}
	wm := bimg.ImageSize{Width: 100, Height: 100}

	left, top := calculatePosition("center", img, wm)
	assert.Equal(t, 0, left)
	assert.Equal(t, 0, top)

	left, top = calculatePosition("bottom-right", img, wm)
	assert.Equal(t, 0, left)
	assert.Equal(t, 0, top)
}

func TestCalculatePosition_WatermarkLargerThanImage(t *testing.T) {
	img := bimg.ImageSize{Width: 50, Height: 50}
	wm := bimg.ImageSize{Width: 100, Height: 100}

	// Negative offsets are clamped to 0 to prevent crashes
	left, top := calculatePosition("top-right", img, wm)
	assert.Equal(t, 0, left)
	assert.Equal(t, 0, top)

	left, top = calculatePosition("bottom-left", img, wm)
	assert.Equal(t, 0, left)
	assert.Equal(t, 0, top)
}

// makeTinyPNG returns the bytes of a 1×1 black PNG.
func makeTinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.Black)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

// makeLargeDimensionPNG returns the bytes of a real PNG with the given dimensions.
func makeLargeDimensionPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestValidateImageInput_TooManyBytes(t *testing.T) {
	huge := bytes.Repeat([]byte{0xff}, maxImageBytes+1)
	err := validateImageInput(huge)
	require.Error(t, err)
	var ve *api.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "input", ve.Field)
}

func TestValidateImageInput_AcceptsTinyPNG(t *testing.T) {
	require.NoError(t, validateImageInput(makeTinyPNG(t)))
}

func TestValidateImageInput_RejectsHighPixelCount(t *testing.T) {
	// 8000 × 8000 = 64 MP > maxImagePixels (50 MP)
	data := makeLargeDimensionPNG(t, 8000, 8000)
	if len(data) > maxImageBytes {
		// If encoding produced a >20MB file, the byte-size check fires first
		// (still a valid rejection, just not the test we wanted). Skip.
		t.Skipf("8000x8000 PNG encoded to %d bytes, exceeds size cap; pixel check not exercised here", len(data))
	}
	err := validateImageInput(data)
	require.Error(t, err)
	var ve *api.ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "input", ve.Field)
}

func TestValidateImageInput_AcceptsModerateImage(t *testing.T) {
	// 1000 × 1000 = 1 MP — well within the cap.
	data := makeLargeDimensionPNG(t, 1000, 1000)
	require.NoError(t, validateImageInput(data))
}

func TestValidateImageInput_BimgFallbackForUnknownFormat(t *testing.T) {
	// Random bytes are not a recognised image: image.DecodeConfig
	// returns ErrFormat, then the bimg fallback also fails. We don't
	// assert the error TYPE (depends on bimg's wrapping), only that
	// the validator surfaces an error rather than silently passing
	// the bytes through.
	random := bytes.Repeat([]byte{0xab, 0xcd}, 100)
	err := validateImageInput(random)
	require.Error(t, err)
}

// keep bimg import used elsewhere
var _ = bimg.JPEG
