package pubsub

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/redis/go-redis/v9"
)

// Service wraps a Redis client for PubSub operations.
type Service struct {
	client *redis.Client
}

// Verify Service implements PubSubService at compile time.
var _ api.PubSubService = (*Service)(nil)

// Publish sends a message to a Redis PubSub channel.
func (s *Service) Publish(ctx context.Context, channel string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pubsub publish %q: marshal: %w", channel, err)
	}

	return s.client.Publish(ctx, channel, string(data)).Err()
}

// Subscribe listens on a Redis PubSub channel and invokes the handler for each message.
// Blocks until the context is cancelled. The handler receives the deserialized payload.
func (s *Service) Subscribe(ctx context.Context, channel string, handler func(payload any) error) error {
	sub := s.client.Subscribe(ctx, channel)
	defer func() { _ = sub.Close() }()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return nil
			}
			var payload any
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				payload = msg.Payload
			}
			if err := handler(payload); err != nil {
				return err
			}
		}
	}
}

// Client returns the underlying Redis client.
func (s *Service) Client() *redis.Client { return s.client }
