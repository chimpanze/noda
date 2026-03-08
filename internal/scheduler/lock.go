package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// tryAcquireLock attempts a Redis SET NX on the given key with TTL.
// Returns true if the lock was acquired, false if another instance holds it.
func tryAcquireLock(svc any, key string, ttl time.Duration) (bool, error) {
	client, ok := extractRedisClient(svc)
	if !ok {
		return false, fmt.Errorf("lock: service does not provide a Redis client")
	}

	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	ctx := context.Background()
	result, err := client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("lock acquire %q: %w", key, err)
	}
	return result, nil
}

// releaseLockKey deletes a lock key.
func releaseLockKey(svc any, key string) error {
	client, ok := extractRedisClient(svc)
	if !ok {
		return fmt.Errorf("lock: service does not provide a Redis client")
	}
	return client.Del(context.Background(), key).Err()
}

// extractRedisClient extracts a *redis.Client from a service that provides one.
func extractRedisClient(svc any) (*redis.Client, bool) {
	type clientProvider interface {
		Client() *redis.Client
	}
	if cp, ok := svc.(clientProvider); ok {
		return cp.Client(), true
	}
	return nil, false
}
