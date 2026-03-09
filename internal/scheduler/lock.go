package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
)

// tryAcquireLock attempts a Redis SET NX on the given key with TTL.
// Returns true if the lock was acquired, false if another instance holds it.
func tryAcquireLock(ctx context.Context, svc any, key string, ttl time.Duration) (bool, error) {
	client, ok := plugin.ExtractRedisClient(svc)
	if !ok {
		return false, fmt.Errorf("lock: service does not provide a Redis client")
	}

	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	result, err := client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, fmt.Errorf("lock acquire %q: %w", key, err)
	}
	return result, nil
}

// releaseLockKey deletes a lock key.
func releaseLockKey(ctx context.Context, svc any, key string) error {
	client, ok := plugin.ExtractRedisClient(svc)
	if !ok {
		return fmt.Errorf("lock: service does not provide a Redis client")
	}
	return client.Del(ctx, key).Err()
}
