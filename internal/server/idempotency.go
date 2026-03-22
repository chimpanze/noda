package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/redis/go-redis/v9"
)

// idempotencyEntry stores a cached response for an idempotent request.
type idempotencyEntry struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body"`
}

// newIdempotencyMiddleware creates middleware that ensures request idempotency via Redis.
// Requests with the same idempotency key return the cached response.
func newIdempotencyMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	redisURL, _ := cfg["redis_url"].(string)
	if redisURL == "" {
		return nil, fmt.Errorf("idempotency: 'redis_url' is required")
	}

	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("idempotency: parse redis_url: %w", err)
	}
	client := redis.NewClient(opts)

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
		cached, err := client.Get(ctx, fingerprint).Bytes()
		if err == nil {
			var entry idempotencyEntry
			if json.Unmarshal(cached, &entry) == nil {
				for k, v := range entry.Headers {
					c.Set(k, v)
				}
				return c.Status(entry.Status).Send(entry.Body)
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
			_ = client.Set(ctx, fingerprint, data, ttl).Err()
		}

		return nil
	}, nil
}
