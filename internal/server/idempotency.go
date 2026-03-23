package server

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/idempotency"
	redisStorage "github.com/gofiber/storage/redis/v3"
)

// newIdempotencyMiddleware creates middleware that ensures request idempotency
// using Fiber's built-in idempotency middleware.
//
// Config options:
//   - key_header: header name (default "X-Idempotency-Key")
//   - lifetime: cache TTL as duration string (default "30m")
//   - storage: "redis" for distributed storage (default in-memory)
//   - redis_url: required when storage is "redis"
func newIdempotencyMiddleware(cfg map[string]any, _ map[string]any) (fiber.Handler, error) {
	idemCfg := idempotency.Config{}

	if cfg != nil {
		if v, _ := cfg["key_header"].(string); v != "" {
			idemCfg.KeyHeader = v
		}

		if v, _ := cfg["lifetime"].(string); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil {
				return nil, fmt.Errorf("idempotency: invalid lifetime %q: %w", v, err)
			}
			idemCfg.Lifetime = d
		}

		if storage, _ := cfg["storage"].(string); storage == "redis" {
			redisURL, _ := cfg["redis_url"].(string)
			if redisURL == "" {
				return nil, fmt.Errorf("idempotency: redis_url is required when storage is \"redis\"")
			}
			idemCfg.Storage = redisStorage.New(redisStorage.Config{
				URL: redisURL,
			})
		}
	}

	return idempotency.New(idemCfg), nil
}
