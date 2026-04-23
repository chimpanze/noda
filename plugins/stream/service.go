package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/redis/go-redis/v9"
)

const (
	// reclaimInterval is how often (in read cycles) Subscribe checks
	// for pending messages from crashed consumer-group members.
	reclaimInterval = 10

	// reclaimMinIdle is the minimum time a message must be pending
	// before Subscribe will steal it from another consumer. Set
	// shorter than typical worker handler timeouts (default 5min) so
	// legitimate slow handlers don't get their messages stolen.
	reclaimMinIdle = 60 * time.Second
)

// dispatchMessage decodes a stream message's payload and invokes handler.
// Returns the handler's error verbatim.
func dispatchMessage(msg redis.XMessage, handler func(messageID string, payload any) error) error {
	payloadStr, _ := msg.Values["payload"].(string)
	var payload any
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		payload = payloadStr
	}
	return handler(msg.ID, payload)
}

// reclaimPending claims messages idle longer than reclaimMinIdle for the
// current consumer. Returns the claimed messages and the cursor to resume
// from on the next call.
func (s *Service) reclaimPending(
	ctx context.Context,
	topic, group, consumer, cursor string,
) ([]redis.XMessage, string, error) {
	if cursor == "" {
		cursor = "0-0"
	}
	msgs, nextCursor, err := s.client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   topic,
		Group:    group,
		Consumer: consumer,
		MinIdle:  reclaimMinIdle,
		Start:    cursor,
		Count:    100,
	}).Result()
	return msgs, nextCursor, err
}

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
//
// Handlers MUST call Ack on success. Unacked messages stay pending
// in the consumer group; after reclaimMinIdle (60s), other consumers
// reclaim them via XAUTOCLAIM and the handler is invoked again on a
// different consumer — at-least-once delivery semantics.
//
// Every reclaimInterval read cycles, Subscribe calls XAUTOCLAIM to
// reclaim messages from crashed consumers (idle > reclaimMinIdle).
// Reclaimed messages flow through the same handler callback.
//
// End-to-end latency from a consumer crashing to another consumer
// re-attempting the message is approximately reclaimMinIdle (60s)
// + reclaimInterval × Block (≈20s) = ~80s.
func (s *Service) Subscribe(ctx context.Context, topic, group, consumer string, handler func(messageID string, payload any) error) error {
	// Auto-create consumer group (MKSTREAM creates the stream too)
	err := s.client.XGroupCreateMkStream(ctx, topic, group, "0").Err()
	if err != nil && !redis.HasErrorPrefix(err, "BUSYGROUP") {
		return fmt.Errorf("stream subscribe: create group %q: %w", group, err)
	}

	var (
		cycle       int
		reclaimNext string // continuation cursor for XAUTOCLAIM
	)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Periodic reclaim of pending messages from dead consumers.
		cycle++
		if cycle >= reclaimInterval {
			cycle = 0
			claimed, next, rerr := s.reclaimPending(ctx, topic, group, consumer, reclaimNext)
			if rerr != nil {
				slog.Warn("stream reclaim failed",
					"topic", topic, "group", group, "consumer", consumer,
					"error", rerr.Error())
				// Keep cursor; retry next cycle.
			} else {
				reclaimNext = next
				for _, msg := range claimed {
					if herr := dispatchMessage(msg, handler); herr != nil {
						return herr
					}
				}
			}
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
				if err := dispatchMessage(msg, handler); err != nil {
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
