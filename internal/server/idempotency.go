package server

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/idempotency"
	redisStorage "github.com/gofiber/storage/redis/v3"
)

// parseIdempotencyConfig validates idempotency config without side effects;
// redisURL is non-empty when redis-backed storage was requested. Split from
// the factory so validate-time checks don't open connections.
func parseIdempotencyConfig(cfg map[string]any) (idempotency.Config, string, error) {
	idemCfg := idempotency.Config{}
	redisURL := ""

	if cfg != nil {
		if v, _ := cfg["key_header"].(string); v != "" {
			idemCfg.KeyHeader = v
		}

		if v, _ := cfg["lifetime"].(string); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return idemCfg, "", fmt.Errorf("idempotency: invalid lifetime %q: %w", v, err)
			}
			idemCfg.Lifetime = d
		}

		if storage, _ := cfg["storage"].(string); storage == "redis" {
			redisURL, _ = cfg["redis_url"].(string)
			if redisURL == "" {
				return idemCfg, "", fmt.Errorf("idempotency: redis_url is required when storage is \"redis\"")
			}
		}
	}

	return idemCfg, redisURL, nil
}

// newIdempotencyMiddleware creates middleware that ensures request idempotency
// using Fiber's built-in idempotency middleware.
//
// Config options:
//   - key_header: header name (default "X-Idempotency-Key")
//   - lifetime: cache TTL as duration string (default "30m")
//   - storage: "redis" for distributed storage (default in-memory)
//   - redis_url: required when storage is "redis"
func newIdempotencyMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	idemCfg, redisURL, err := parseIdempotencyConfig(cfg)
	if err != nil {
		return nil, err
	}
	if redisURL != "" {
		idemCfg.Storage = redisStorage.New(redisStorage.Config{
			URL: redisURL,
		})
	}
	return idempotency.New(idemCfg), nil
}
