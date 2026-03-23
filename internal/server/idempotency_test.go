package server

import (
	"io"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdempotencyMiddleware_NoKey_PassesThrough(t *testing.T) {
	h, err := newIdempotencyMiddleware(nil, nil)
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Post("/test", h, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	// First call without key
	req := httptest.NewRequest("POST", "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, callCount)

	// Second call without key — should execute again
	req = httptest.NewRequest("POST", "/test", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 2, callCount)
}

func TestIdempotencyMiddleware_WithKey_ReturnsCached(t *testing.T) {
	h, err := newIdempotencyMiddleware(map[string]any{
		"lifetime": "1h",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Post("/test", h, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	// Fiber's built-in uses X-Idempotency-Key and expects a UUID (36 chars)
	key := "550e8400-e29b-41d4-a716-446655440000"

	// First call with key
	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Idempotency-Key", key)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, callCount)

	body1, _ := io.ReadAll(resp.Body)

	// Second call with same key — should return cached response
	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Idempotency-Key", key)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, callCount) // handler NOT called again

	body2, _ := io.ReadAll(resp.Body)
	assert.JSONEq(t, string(body1), string(body2))
}

func TestIdempotencyMiddleware_DifferentKeys_ExecutesSeparately(t *testing.T) {
	h, err := newIdempotencyMiddleware(nil, nil)
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Post("/test", h, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Idempotency-Key", "550e8400-e29b-41d4-a716-446655440001")
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Idempotency-Key", "550e8400-e29b-41d4-a716-446655440002")
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount) // different key, new execution
}

func TestIdempotencyMiddleware_CustomKeyHeader(t *testing.T) {
	h, err := newIdempotencyMiddleware(map[string]any{
		"key_header": "X-Request-Token",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Post("/test", h, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	key := "550e8400-e29b-41d4-a716-446655440003"

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Request-Token", key)
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Request-Token", key)
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount) // cached
}

func TestIdempotencyMiddleware_SafeMethodSkipped(t *testing.T) {
	h, err := newIdempotencyMiddleware(nil, nil)
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Get("/test", h, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	key := "550e8400-e29b-41d4-a716-446655440004"

	// GET is a safe method — idempotency middleware should skip it
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Idempotency-Key", key)
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Idempotency-Key", key)
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount) // NOT cached — safe methods are skipped
}

func TestIdempotencyMiddleware_CachesStatusCode(t *testing.T) {
	h, err := newIdempotencyMiddleware(nil, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Post("/test", h, func(c fiber.Ctx) error {
		return c.Status(201).JSON(map[string]any{"created": true})
	})

	key := "550e8400-e29b-41d4-a716-446655440005"

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Idempotency-Key", key)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	// Second call returns same status
	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Idempotency-Key", key)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
}

func TestNewIdempotencyMiddleware_InvalidLifetime(t *testing.T) {
	_, err := newIdempotencyMiddleware(map[string]any{
		"lifetime": "not-a-duration",
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid lifetime")
}

func TestNewIdempotencyMiddleware_RedisStorage(t *testing.T) {
	mr := miniredis.RunT(t)

	h, err := newIdempotencyMiddleware(map[string]any{
		"storage":   "redis",
		"redis_url": "redis://" + mr.Addr(),
	}, nil)
	require.NoError(t, err)
	assert.NotNil(t, h)
}

func TestNewIdempotencyMiddleware_RedisStorage_MissingURL(t *testing.T) {
	_, err := newIdempotencyMiddleware(map[string]any{
		"storage": "redis",
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis_url is required")
}
