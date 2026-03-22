package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
)

// idempotencyEntry stores a cached response for an idempotent request.
type idempotencyEntry struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
}

// newIdempotencyMiddleware creates middleware that ensures request idempotency
// using a cache service resolved from the service registry.
func newIdempotencyMiddleware(cfg map[string]any, rootConfig map[string]any) (fiber.Handler, error) {
	serviceName, _ := cfg["cache_service"].(string)
	if serviceName == "" {
		return nil, fmt.Errorf("idempotency: 'cache_service' is required in middleware config")
	}

	svcReg, ok := rootConfig["_services"].(*registry.ServiceRegistry)
	if !ok {
		return nil, fmt.Errorf("idempotency: service registry not available")
	}

	svc, ok := svcReg.Get(serviceName)
	if !ok {
		return nil, fmt.Errorf("idempotency: cache service %q not found in service registry", serviceName)
	}
	cache, ok := svc.(api.CacheService)
	if !ok {
		return nil, fmt.Errorf("idempotency: service %q does not implement CacheService", serviceName)
	}

	return newIdempotencyHandler(cache, cfg), nil
}

// newIdempotencyHandler creates the idempotency Fiber handler using a cache service.
func newIdempotencyHandler(cache api.CacheService, cfg map[string]any) fiber.Handler {
	keyHeader := "Idempotency-Key"
	if v, _ := cfg["key_header"].(string); v != "" {
		keyHeader = v
	}

	ttl := 24 * time.Hour
	if v, _ := cfg["ttl"].(string); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			ttl = d
		}
	}
	ttlSeconds := int(ttl.Seconds())

	prefix := "noda:idempotency:"

	return func(c fiber.Ctx) error {
		key := c.Get(keyHeader)
		if key == "" {
			// No idempotency key — pass through
			return c.Next()
		}

		// Build a fingerprint: method + path + idempotency key
		h := sha256.New()
		h.Write([]byte(c.Method()))
		h.Write([]byte(c.Path()))
		h.Write([]byte(key))
		fingerprint := prefix + hex.EncodeToString(h.Sum(nil))

		ctx := context.Background()

		// Check for cached response
		cached, err := cache.Get(ctx, fingerprint)
		if err == nil {
			// cached is the stored JSON string
			var data string
			switch v := cached.(type) {
			case string:
				data = v
			default:
				// If the cache returns something else, try to marshal it back
				if b, err := json.Marshal(v); err == nil {
					data = string(b)
				}
			}
			if data != "" {
				var entry idempotencyEntry
				if json.Unmarshal([]byte(data), &entry) == nil {
					for k, v := range entry.Headers {
						c.Set(k, v)
					}
					return c.Status(entry.Status).Send(entry.Body)
				}
			}
		}

		// Execute the request
		if err := c.Next(); err != nil {
			return err
		}

		// Cache the response
		entry := idempotencyEntry{
			Status:  c.Response().StatusCode(),
			Headers: make(map[string]string),
			Body:    c.Response().Body(),
		}
		// Cache content-type header
		if ct := string(c.Response().Header.ContentType()); ct != "" {
			entry.Headers["Content-Type"] = ct
		}

		data, err := json.Marshal(entry)
		if err == nil {
			_ = cache.Set(ctx, fingerprint, string(data), ttlSeconds)
		}

		return nil
	}
}
