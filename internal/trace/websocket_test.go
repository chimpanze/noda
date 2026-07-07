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

// Hostnames are case-insensitive (RFC 4343); originAllowed must not reject
// same-origin dev traffic over casing (#281). It still fails closed for
// genuinely foreign origins.
func TestOriginAllowed_CaseInsensitive(t *testing.T) {
	cases := []struct {
		origin, host string
		want         bool
	}{
		{"http://Example.com", "example.com", true},
		{"http://EXAMPLE.COM:3000", "example.com", true},
		{"http://LocalHost:5173", "example.com", true},
		{"http://evil.com", "example.com", false},
		{"://bad-url", "example.com", false},
	}
	for _, tc := range cases {
		if got := originAllowed(tc.origin, tc.host); got != tc.want {
			t.Errorf("originAllowed(%q, %q) = %v, want %v", tc.origin, tc.host, got, tc.want)
		}
	}
}
