package trace

import (
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

func TestTraceWebSocket_RejectsCrossOrigin(t *testing.T) {
	app := fiber.New()
	RegisterTraceWebSocket(app, NewEventHub(), slog.Default())

	// cross-origin upgrade attempt → 403 (never reaches the ws handler)
	req := httptest.NewRequest("GET", "/ws/trace", nil)
	req.Header.Set("Origin", "http://evil.com")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 403, resp.StatusCode)

	// same-host origin → not 403 (426/101/etc. — upgrade proceeds past the guard)
	req2 := httptest.NewRequest("GET", "/ws/trace", nil)
	req2.Header.Set("Origin", "http://example.com")
	req2.Host = "example.com"
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	require.NotEqual(t, 403, resp2.StatusCode)

	// empty Origin (non-browser client, e.g. CLI) → not 403
	req3 := httptest.NewRequest("GET", "/ws/trace", nil)
	resp3, err := app.Test(req3)
	require.NoError(t, err)
	require.NotEqual(t, 403, resp3.StatusCode)

	// localhost origin → not 403
	req4 := httptest.NewRequest("GET", "/ws/trace", nil)
	req4.Header.Set("Origin", "http://localhost:5173")
	resp4, err := app.Test(req4)
	require.NoError(t, err)
	require.NotEqual(t, 403, resp4.StatusCode)
}
