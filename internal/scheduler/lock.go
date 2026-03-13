package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// releaseLockScript atomically deletes a key only if it holds the expected token.
var releaseLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

// tryAcquireLock attempts a Redis SET NX on the given key with a unique token and TTL.
// Returns the lock token (non-empty) if the lock was acquired, empty if another instance holds it.
func tryAcquireLock(ctx context.Context, svc any, key string, ttl time.Duration) (string, error) {
	provider, ok := svc.(plugin.RedisClientProvider)
	if !ok {
		return "", fmt.Errorf("lock: service does not implement RedisClientProvider")
	}

	if ttl == 0 {
		ttl = 5 * time.Minute
	}

	token := uuid.New().String()
	result, err := provider.Client().SetArgs(ctx, key, token, redis.SetArgs{Mode: "NX", TTL: ttl}).Result()
	if err == redis.Nil {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("lock acquire %q: %w", key, err)
	}
	if result == "OK" {
		return token, nil
	}
	return "", nil
}

// releaseLockKey releases a lock only if the token matches (compare-and-delete via Lua).
func releaseLockKey(ctx context.Context, svc any, key string, token string) error {
	provider, ok := svc.(plugin.RedisClientProvider)
	if !ok {
		return fmt.Errorf("lock: service does not implement RedisClientProvider")
	}
	_, err := releaseLockScript.Run(ctx, provider.Client(), []string{key}, token).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("lock release %q: %w", key, err)
	}
	return nil
}
