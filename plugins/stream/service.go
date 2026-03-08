package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/redis/go-redis/v9"
)

// Service wraps a Redis client for Streams operations.
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

// Subscribe reads messages from a Redis Stream consumer group.
// It blocks until messages are available or the context is cancelled.
// The handler receives the message ID and deserialized payload.
// Auto-creates the consumer group if it doesn't exist.
func (s *Service) Subscribe(ctx context.Context, topic, group, consumer string, handler func(messageID string, payload any) error) error {
	// Auto-create consumer group (MKSTREAM creates the stream too)
	err := s.client.XGroupCreateMkStream(ctx, topic, group, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return fmt.Errorf("stream subscribe: create group %q: %w", group, err)
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		streams, err := s.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: consumer,
			Streams:  []string{topic, ">"},
			Count:    1,
			Block:    2 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				continue // timeout, no messages
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("stream subscribe %q: %w", topic, err)
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				payloadStr, _ := msg.Values["payload"].(string)
				var payload any
				if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
					payload = payloadStr
				}

				if err := handler(msg.ID, payload); err != nil {
					return err
				}
			}
		}
	}
}

// Ack acknowledges a message in a consumer group.
func (s *Service) Ack(ctx context.Context, topic, group, messageID string) error {
	_, err := s.client.XAck(ctx, topic, group, messageID).Result()
	if err != nil {
		return fmt.Errorf("stream ack %q/%q: %w", topic, messageID, err)
	}
	return nil
}

// PendingCount returns the number of pending (unacknowledged) messages for a group.
func (s *Service) PendingCount(ctx context.Context, topic, group string) (int64, error) {
	info, err := s.client.XPending(ctx, topic, group).Result()
	if err != nil {
		return 0, fmt.Errorf("stream pending %q/%q: %w", topic, group, err)
	}
	return info.Count, nil
}

// Client returns the underlying Redis client.
func (s *Service) Client() *redis.Client { return s.client }
