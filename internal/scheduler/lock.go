package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/redis/go-redis/v9"
)

// tryAcquireLock attempts a Redis SET NX on the given key with TTL.
// Returns true if the lock was acquired, false if another instance holds it.
func tryAcquireLock(ctx context.Context, svc any, key string, ttl time.Duration) (bool, error) {
	provider, ok := svc.(plugin.RedisClientProvider)
	if !ok {
		return false, fmt.Errorf("lock: service does not implement RedisClientProvider")
	}

	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	result, err := provider.Client().SetArgs(ctx, key, "1", redis.SetArgs{Mode: "NX", TTL: ttl}).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock acquire %q: %w", key, err)
	}
	return result == "OK", nil
}

// releaseLockKey deletes a lock key.
func releaseLockKey(ctx context.Context, svc any, key string) error {
	provider, ok := svc.(plugin.RedisClientProvider)
	if !ok {
		return fmt.Errorf("lock: service does not implement RedisClientProvider")
	}
	return provider.Client().Del(ctx, key).Err()
}
