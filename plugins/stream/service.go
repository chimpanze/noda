package stream

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/redis/go-redis/v9"
)

// Service wraps a Redis client for Streams operations.
//
// Stream consumption is the worker runtime's job (internal/worker), which
// reads via the raw Redis client from Client() and reclaims pending messages
// with a policy clamped to the handler timeout. This service intentionally
// exposes no consume-side API.
type Service struct {
	client *redis.Client
}

// Verify Service implements StreamService at compile time.
var _ api.StreamService = (*Service)(nil)

// Publish adds a message to a Redis Stream via XADD.
// Returns the message ID assigned by Redis.
func (s *Service) Publish(ctx context.Context, topic string, payload any) (string, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("stream publish %q: marshal: %w", topic, err)
	}

	id, err := s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: topic,
		Values: map[string]any{"payload": string(data)},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("stream publish %q: %w", topic, err)
	}
	return id, nil
}

// Client returns the underlying Redis client.
func (s *Service) Client() *redis.Client { return s.client }
