package image

import (
	"testing"

	"github.com/h2non/bimg"
	"github.com/stretchr/testify/assert"
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
