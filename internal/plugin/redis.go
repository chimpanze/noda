package plugin

import "github.com/redis/go-redis/v9"

// ExtractRedisClient extracts a *redis.Client from a service that exposes one
// via a Client() method (e.g., stream.Service, cache.Service).
func ExtractRedisClient(svc any) (*redis.Client, bool) {
	type clientProvider interface {
		Client() *redis.Client
	}
	if cp, ok := svc.(clientProvider); ok {
		return cp.Client(), true
	}
	return nil, false
}
