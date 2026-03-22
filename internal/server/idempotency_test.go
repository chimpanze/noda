package server

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdempotencyMiddleware_NoKey_PassesThrough(t *testing.T) {
	mr := miniredis.RunT(t)

	handler, err := newIdempotencyMiddleware(map[string]any{
		"redis_url": "redis://" + mr.Addr(),
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Post("/test", handler, func(c fiber.Ctx) error {
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
	mr := miniredis.RunT(t)

	handler, err := newIdempotencyMiddleware(map[string]any{
		"redis_url": "redis://" + mr.Addr(),
		"ttl":       "1h",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Post("/test", handler, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	// First call with key
	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "abc123")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, callCount)

	body1, _ := io.ReadAll(resp.Body)

	// Second call with same key — should return cached response
	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "abc123")
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, callCount) // handler NOT called again

	body2, _ := io.ReadAll(resp.Body)
	assert.JSONEq(t, string(body1), string(body2))
}

func TestIdempotencyMiddleware_DifferentKeys_ExecutesSeparately(t *testing.T) {
	mr := miniredis.RunT(t)

	handler, err := newIdempotencyMiddleware(map[string]any{
		"redis_url": "redis://" + mr.Addr(),
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Post("/test", handler, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "key-1")
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "key-2")
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount) // different key, new execution
}

func TestIdempotencyMiddleware_CustomKeyHeader(t *testing.T) {
	mr := miniredis.RunT(t)

	handler, err := newIdempotencyMiddleware(map[string]any{
		"redis_url":  "redis://" + mr.Addr(),
		"key_header": "X-Request-Token",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Post("/test", handler, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Request-Token", "token-1")
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Request-Token", "token-1")
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount) // cached
}

func TestIdempotencyMiddleware_TTLExpiry(t *testing.T) {
	mr := miniredis.RunT(t)

	handler, err := newIdempotencyMiddleware(map[string]any{
		"redis_url": "redis://" + mr.Addr(),
		"ttl":       "100ms",
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	callCount := 0
	app.Post("/test", handler, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "expire-test")
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	// Fast-forward time in miniredis
	mr.FastForward(200 * time.Millisecond)

	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "expire-test")
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount) // TTL expired, new execution
}

func TestIdempotencyMiddleware_MissingRedisURL(t *testing.T) {
	_, err := newIdempotencyMiddleware(map[string]any{}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "redis_url")
}

func TestIdempotencyMiddleware_CachesStatusCode(t *testing.T) {
	mr := miniredis.RunT(t)

	handler, err := newIdempotencyMiddleware(map[string]any{
		"redis_url": "redis://" + mr.Addr(),
	}, nil)
	require.NoError(t, err)

	app := fiber.New()
	app.Post("/test", handler, func(c fiber.Ctx) error {
		return c.Status(201).JSON(map[string]any{"created": true})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "create-1")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, true, result["created"])

	// Second call returns same status
	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "create-1")
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
}
