package server

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildMiddleware_StatusRemap_MissingMap(t *testing.T) {
	_, err := BuildMiddleware("response.status_remap", map[string]any{
		"middleware": map[string]any{
			"response.status_remap": map[string]any{},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "map")
}

func TestBuildMiddleware_StatusRemap_EmptyMap(t *testing.T) {
	_, err := BuildMiddleware("response.status_remap", map[string]any{
		"middleware": map[string]any{
			"response.status_remap": map[string]any{
				"map": map[string]any{},
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestBuildMiddleware_StatusRemap_NonNumericKey(t *testing.T) {
	_, err := BuildMiddleware("response.status_remap", map[string]any{
		"middleware": map[string]any{
			"response.status_remap": map[string]any{
				"map": map[string]any{
					"forbidden": float64(401),
				},
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")
}

func TestBuildMiddleware_StatusRemap_KeyOutOfRange(t *testing.T) {
	for _, badKey := range []string{"99", "600", "0", "-403"} {
		_, err := BuildMiddleware("response.status_remap", map[string]any{
			"middleware": map[string]any{
				"response.status_remap": map[string]any{
					"map": map[string]any{badKey: float64(401)},
				},
			},
		})
		require.Error(t, err, "key %q should be rejected", badKey)
		assert.Contains(t, err.Error(), badKey)
	}
}

func TestBuildMiddleware_StatusRemap_ValueOutOfRange(t *testing.T) {
	_, err := BuildMiddleware("response.status_remap", map[string]any{
		"middleware": map[string]any{
			"response.status_remap": map[string]any{
				"map": map[string]any{"403": float64(700)},
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "700")
}

func TestBuildMiddleware_StatusRemap_SelfMap(t *testing.T) {
	_, err := BuildMiddleware("response.status_remap", map[string]any{
		"middleware": map[string]any{
			"response.status_remap": map[string]any{
				"map": map[string]any{"403": float64(403)},
			},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestBuildMiddleware_StatusRemap_HappyPath(t *testing.T) {
	h, err := BuildMiddleware("response.status_remap", map[string]any{
		"middleware": map[string]any{
			"response.status_remap": map[string]any{
				"map": map[string]any{
					"403": float64(401),
					"502": float64(503),
				},
			},
		},
	})
	require.NoError(t, err)

	app := fiber.New()
	app.Use(h)
	app.Get("/forbidden", func(c fiber.Ctx) error {
		return c.Status(403).JSON(map[string]string{"error": "forbidden"})
	})
	app.Get("/badgateway", func(c fiber.Ctx) error {
		return c.Status(502).SendString("upstream down")
	})
	app.Get("/ok", func(c fiber.Ctx) error {
		return c.Status(200).SendString("ok")
	})
	app.Get("/teapot", func(c fiber.Ctx) error {
		return c.Status(418).SendString("teapot")
	})

	// 403 → 401
	req := httptest.NewRequest("GET", "/forbidden", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)

	// 502 → 503
	req = httptest.NewRequest("GET", "/badgateway", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 503, resp.StatusCode)

	// 200 passes through
	req = httptest.NewRequest("GET", "/ok", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Unmapped status (418) passes through
	req = httptest.NewRequest("GET", "/teapot", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 418, resp.StatusCode)
}

func TestBuildMiddleware_StatusRemap_PreservesBodyAndHeaders(t *testing.T) {
	h, err := BuildMiddleware("response.status_remap", map[string]any{
		"middleware": map[string]any{
			"response.status_remap": map[string]any{
				"map": map[string]any{"403": float64(401)},
			},
		},
	})
	require.NoError(t, err)

	app := fiber.New()
	app.Use(h)
	app.Get("/forbidden", func(c fiber.Ctx) error {
		c.Set("X-Custom-Header", "preserved")
		c.Set("Content-Type", "application/problem+json")
		return c.Status(403).SendString(`{"error":"forbidden","detail":"no session"}`)
	})

	req := httptest.NewRequest("GET", "/forbidden", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
	assert.Equal(t, "preserved", resp.Header.Get("X-Custom-Header"))
	assert.Equal(t, "application/problem+json", resp.Header.Get("Content-Type"))

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, `{"error":"forbidden","detail":"no session"}`, string(body))
}
