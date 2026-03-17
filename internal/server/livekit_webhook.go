package server

import (
	"bytes"
	"fmt"
	"net/http"

	"github.com/gofiber/fiber/v3"
	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/webhook"
)

// newLiveKitWebhookMiddleware creates a middleware that verifies LiveKit webhook signatures.
// Credentials are resolved from the middleware config first, then fall back to the
// LiveKit service config in rootConfig["services"].
func newLiveKitWebhookMiddleware(cfg map[string]any, rootConfig map[string]any) (fiber.Handler, error) {
	apiKey, apiSecret := resolveWebhookCredentials(cfg, rootConfig)
	if apiKey == "" || apiSecret == "" {
		return nil, fmt.Errorf("livekit.webhook: api_key and api_secret are required (set in middleware config or lk service config)")
	}

	provider := auth.NewSimpleKeyProvider(apiKey, apiSecret)

	return func(c fiber.Ctx) error {
		// LiveKit's webhook package expects *http.Request.
		// Build a minimal adapter from Fiber's fasthttp context.
		body := bytes.NewReader(c.Body())
		r, err := http.NewRequest(c.Method(), c.OriginalURL(), body)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, "failed to build request adapter")
		}
		r.Header.Set("Authorization", c.Get("Authorization"))

		event, err := webhook.ReceiveWebhookEvent(r, provider)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid webhook signature")
		}

		c.Locals("livekit_event", event)
		return c.Next()
	}, nil
}

// resolveWebhookCredentials extracts api_key/api_secret from middleware config
// or falls back to the lk service config.
func resolveWebhookCredentials(cfg map[string]any, rootConfig map[string]any) (string, string) {
	// Try middleware-level config first
	if cfg != nil {
		apiKey, _ := cfg["api_key"].(string)
		apiSecret, _ := cfg["api_secret"].(string)
		if apiKey != "" && apiSecret != "" {
			return apiKey, apiSecret
		}
	}

	// Fall back to lk service config
	services, _ := rootConfig["services"].(map[string]any)
	if services == nil {
		return "", ""
	}

	// Look for a service with plugin "lk" or "livekit"
	for _, svcRaw := range services {
		svc, ok := svcRaw.(map[string]any)
		if !ok {
			continue
		}
		pluginName, _ := svc["plugin"].(string)
		if pluginName != "lk" && pluginName != "livekit" {
			continue
		}
		inner, _ := svc["config"].(map[string]any)
		if inner == nil {
			inner = svc
		}
		apiKey, _ := inner["api_key"].(string)
		apiSecret, _ := inner["api_secret"].(string)
		if apiKey != "" && apiSecret != "" {
			return apiKey, apiSecret
		}
	}

	return "", ""
}
