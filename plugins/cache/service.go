package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/redis/go-redis/v9"
)

// Service wraps a Redis client and implements api.CacheService.
type Service struct {
	client *redis.Client
}

// Client returns the underlying Redis client (for direct access if needed).
func (s *Service) Client() *redis.Client { return s.client }

// Get retrieves a value by key, deserializing from JSON.
func (s *Service) Get(ctx context.Context, key string) (any, error) {
	val, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, &api.NotFoundError{Resource: "cache", ID: key}
	}
	if err != nil {
		return nil, fmt.Errorf("cache get %q: %w", key, err)
	}

	var result any
	if err := json.Unmarshal([]byte(val), &result); err != nil {
		// If it's not valid JSON, return as string
		return val, nil
	}
	return result, nil
}

// Set stores a value with optional TTL (in seconds, 0 = no expiry).
func (s *Service) Set(ctx context.Context, key string, value any, ttl int) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache set %q: marshal: %w", key, err)
	}

	var expiration time.Duration
	if ttl > 0 {
		expiration = time.Duration(ttl) * time.Second
	}

	return s.client.Set(ctx, key, string(data), expiration).Err()
}

// Del removes a key.
func (s *Service) Del(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

// Exists checks if a key exists.
func (s *Service) Exists(ctx context.Context, key string) (bool, error) {
	n, err := s.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("cache exists %q: %w", key, err)
	}
	return n > 0, nil
}

// Verify Service implements CacheService at compile time.
var _ api.CacheService = (*Service)(nil)
