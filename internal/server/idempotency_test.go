package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCacheService is a simple in-memory cache for testing.
type mockCacheService struct {
	mu   sync.RWMutex
	data map[string]any
}

func newMockCacheService() *mockCacheService {
	return &mockCacheService{data: make(map[string]any)}
}

func (m *mockCacheService) Get(_ context.Context, key string) (any, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, &api.NotFoundError{Resource: "cache", ID: key}
	}
	return v, nil
}

func (m *mockCacheService) Set(_ context.Context, key string, value any, _ int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *mockCacheService) Del(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}

func (m *mockCacheService) Exists(_ context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[key]
	return ok, nil
}

func TestIdempotencyMiddleware_NoKey_PassesThrough(t *testing.T) {
	cache := newMockCacheService()
	handler := newIdempotencyHandler(cache, nil)

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
	cache := newMockCacheService()
	handler := newIdempotencyHandler(cache, map[string]any{
		"ttl": "1h",
	})

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
	cache := newMockCacheService()
	handler := newIdempotencyHandler(cache, nil)

	app := fiber.New()
	callCount := 0
	app.Post("/test", handler, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "key-1")
	_, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "key-2")
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount) // different key, new execution
}

func TestIdempotencyMiddleware_CustomKeyHeader(t *testing.T) {
	cache := newMockCacheService()
	handler := newIdempotencyHandler(cache, map[string]any{
		"key_header": "X-Request-Token",
	})

	app := fiber.New()
	callCount := 0
	app.Post("/test", handler, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Request-Token", "token-1")
	_, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)

	req = httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Request-Token", "token-1")
	_, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount) // cached
}

func TestIdempotencyMiddleware_CacheMiss_ExecutesHandler(t *testing.T) {
	// Cache that always returns NotFound
	cache := newMockCacheService()
	handler := newIdempotencyHandler(cache, nil)

	app := fiber.New()
	executed := false
	app.Post("/test", handler, func(c fiber.Ctx) error {
		executed = true
		return c.JSON(map[string]any{"ok": true})
	})

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "new-key")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, executed)
}

func TestIdempotencyMiddleware_CachesStatusCode(t *testing.T) {
	cache := newMockCacheService()
	handler := newIdempotencyHandler(cache, nil)

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

func TestNewIdempotencyMiddleware_MissingCacheServiceConfig(t *testing.T) {
	_, err := newIdempotencyMiddleware(
		map[string]any{},
		map[string]any{"_services": registry.NewServiceRegistry()},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache_service")
}

func TestNewIdempotencyMiddleware_ServiceNotFound(t *testing.T) {
	_, err := newIdempotencyMiddleware(
		map[string]any{"cache_service": "missing"},
		map[string]any{"_services": registry.NewServiceRegistry()},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in service registry")
}

func TestNewIdempotencyMiddleware_WrongServiceType(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	_ = svcReg.Register("not-cache", &struct{}{}, &mockPlugin{})

	_, err := newIdempotencyMiddleware(
		map[string]any{"cache_service": "not-cache"},
		map[string]any{"_services": svcReg},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not implement CacheService")
}

func TestNewIdempotencyMiddleware_NoServiceRegistry(t *testing.T) {
	_, err := newIdempotencyMiddleware(
		map[string]any{"cache_service": "cache"},
		map[string]any{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service registry not available")
}

// failingCacheService returns errors on Get to test cache miss handling.
type failingCacheService struct{}

func (f *failingCacheService) Get(_ context.Context, key string) (any, error) {
	return nil, fmt.Errorf("cache unavailable")
}
func (f *failingCacheService) Set(_ context.Context, _ string, _ any, _ int) error { return nil }
func (f *failingCacheService) Del(_ context.Context, _ string) error               { return nil }
func (f *failingCacheService) Exists(_ context.Context, _ string) (bool, error)    { return false, nil }

func TestIdempotencyMiddleware_CacheError_StillExecutes(t *testing.T) {
	handler := newIdempotencyHandler(&failingCacheService{}, nil)

	app := fiber.New()
	callCount := 0
	app.Post("/test", handler, func(c fiber.Ctx) error {
		callCount++
		return c.JSON(map[string]any{"n": callCount})
	})

	// Even with cache errors, the handler should execute
	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("Idempotency-Key", "test-key")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, callCount)
}
