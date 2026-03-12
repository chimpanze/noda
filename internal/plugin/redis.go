package plugin

import (
	"fmt"

	"github.com/redis/go-redis/v9"
)

// RedisClientProvider is implemented by services that expose a raw Redis client
// (e.g., cache.Service, stream.Service, pubsub.Service). Used by internal
// components like the scheduler (distributed locking) and worker (stream
// consumption) that need Redis operations not covered by service interfaces.
type RedisClientProvider interface {
	Client() *redis.Client
}

// NewRedisClient creates a Redis client from a plugin config map.
// It expects a "url" key and optionally "pool_size" and "min_idle".
func NewRedisClient(config map[string]any, prefix string) (*redis.Client, error) {
	url, _ := config["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("%s: missing 'url'", prefix)
	}

	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("%s: parse url: %w", prefix, err)
	}

	if v, ok := ToInt(config["pool_size"]); ok {
		opts.PoolSize = v
	}
	if v, ok := ToInt(config["min_idle"]); ok {
		opts.MinIdleConns = v
	}

	return redis.NewClient(opts), nil
}
