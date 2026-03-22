package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/metric/noop"
)

func TestNewMetrics(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	m, err := NewMetrics(meter)
	require.NoError(t, err)
	assert.NotNil(t, m.RequestDuration)
	assert.NotNil(t, m.RequestsTotal)
	assert.NotNil(t, m.ErrorsTotal)
	assert.NotNil(t, m.WorkflowDuration)
	assert.NotNil(t, m.WorkflowsTotal)
	assert.NotNil(t, m.WorkflowErrors)
	assert.NotNil(t, m.NodeDuration)
	assert.NotNil(t, m.NodeErrors)
	assert.NotNil(t, m.ActiveConns)
	assert.NotNil(t, m.PanicsRecovered)
}

func TestNewProvider(t *testing.T) {
	provider, handler, err := NewProvider()
	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.NotNil(t, handler)

	// Verify the handler serves Prometheus metrics
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestParseConfig_Enabled(t *testing.T) {
	root := map[string]any{
		"observability": map[string]any{
			"metrics": map[string]any{
				"enabled": true,
				"path":    "/custom-metrics",
			},
		},
	}

	cfg := ParseConfig(root)
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "/custom-metrics", cfg.Path)
}

func TestParseConfig_Disabled(t *testing.T) {
	root := map[string]any{
		"observability": map[string]any{
			"metrics": map[string]any{
				"enabled": false,
			},
		},
	}

	cfg := ParseConfig(root)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "/metrics", cfg.Path)
}

func TestParseConfig_Missing(t *testing.T) {
	cfg := ParseConfig(map[string]any{})
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "/metrics", cfg.Path)
}

func TestParseConfig_NoMetricsKey(t *testing.T) {
	root := map[string]any{
		"observability": map[string]any{},
	}

	cfg := ParseConfig(root)
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "/metrics", cfg.Path)
}

func TestHTTPMiddleware(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	m, err := NewMetrics(meter)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(NewHTTPMiddleware(m))
	app.Get("/test", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "ok", string(body))
}

func TestHTTPMiddleware_Error(t *testing.T) {
	meter := noop.NewMeterProvider().Meter("test")
	m, err := NewMetrics(meter)
	require.NoError(t, err)

	app := fiber.New()
	app.Use(NewHTTPMiddleware(m))
	app.Get("/fail", func(c fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "bad")
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
