package plugin

import "github.com/redis/go-redis/v9"

// RedisClientProvider is implemented by services that expose a raw Redis client
// (e.g., cache.Service, stream.Service, pubsub.Service). Used by internal
// components like the scheduler (distributed locking) and worker (stream
// consumption) that need Redis operations not covered by service interfaces.
type RedisClientProvider interface {
	Client() *redis.Client
}
