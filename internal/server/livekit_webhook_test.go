package server

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testLKAPIKey    = "APItest123"
	testLKAPISecret = "secret-that-is-long-enough-for-hmac-signing"
)

// signWebhookBody creates a LiveKit-style webhook Authorization header value.
// LiveKit signs: base64(sha256(body)) as the token body, with api_key in claims.
func signWebhookBody(body []byte, apiKey, apiSecret string) string {
	sum := sha256.Sum256(body)
	hash := base64.StdEncoding.EncodeToString(sum[:])

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":    apiKey,
		"sha256": hash,
		"nbf":    time.Now().Add(-5 * time.Minute).Unix(),
		"exp":    time.Now().Add(5 * time.Minute).Unix(),
	})
	tokenStr, _ := token.SignedString([]byte(apiSecret))
	return tokenStr
}

func TestLiveKitWebhook_ValidSignature(t *testing.T) {
	body := `{"event":"room_started","room":{"sid":"RM_test","name":"test-room"}}`
	auth := signWebhookBody([]byte(body), testLKAPIKey, testLKAPISecret)

	app := fiber.New()
	h, err := newLiveKitWebhookMiddleware(
		map[string]any{"api_key": testLKAPIKey, "api_secret": testLKAPISecret},
		map[string]any{},
	)
	require.NoError(t, err)

	app.Use(h)
	app.Post("/webhook", func(c fiber.Ctx) error {
		event := c.Locals("livekit_event")
		if event == nil {
			return c.Status(500).SendString("no event in locals")
		}
		return c.SendString("ok")
	})

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/webhook+json")
	req.Header.Set("Authorization", auth)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestLiveKitWebhook_InvalidSignature(t *testing.T) {
	app := fiber.New()
	h, err := newLiveKitWebhookMiddleware(
		map[string]any{"api_key": testLKAPIKey, "api_secret": testLKAPISecret},
		map[string]any{},
	)
	require.NoError(t, err)

	app.Use(h)
	app.Post("/webhook", func(c fiber.Ctx) error {
		return c.SendString("should not reach")
	})

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(`{"event":"test"}`))
	req.Header.Set("Authorization", "invalid-token")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestLiveKitWebhook_MissingAuth(t *testing.T) {
	app := fiber.New()
	h, err := newLiveKitWebhookMiddleware(
		map[string]any{"api_key": testLKAPIKey, "api_secret": testLKAPISecret},
		map[string]any{},
	)
	require.NoError(t, err)

	app.Use(h)
	app.Post("/webhook", func(c fiber.Ctx) error {
		return c.SendString("should not reach")
	})

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(`{"event":"test"}`))
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 401, resp.StatusCode)
}

func TestLiveKitWebhook_MissingCredentials(t *testing.T) {
	_, err := newLiveKitWebhookMiddleware(nil, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_key and api_secret are required")
}

func TestLiveKitWebhook_FallbackToServiceConfig(t *testing.T) {
	h, err := newLiveKitWebhookMiddleware(nil, map[string]any{
		"services": map[string]any{
			"lk": map[string]any{
				"plugin": "lk",
				"config": map[string]any{
					"url":        "wss://example.livekit.cloud",
					"api_key":    testLKAPIKey,
					"api_secret": testLKAPISecret,
				},
			},
		},
	})
	require.NoError(t, err)
	assert.NotNil(t, h)
}

func TestResolveWebhookCredentials_DirectConfig(t *testing.T) {
	key, secret := resolveWebhookCredentials(
		map[string]any{"api_key": "k", "api_secret": "s"},
		map[string]any{},
	)
	assert.Equal(t, "k", key)
	assert.Equal(t, "s", secret)
}

func TestResolveWebhookCredentials_Empty(t *testing.T) {
	key, secret := resolveWebhookCredentials(nil, map[string]any{})
	assert.Empty(t, key)
	assert.Empty(t, secret)
}
