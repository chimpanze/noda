package api

import "context"

// StorageService provides file storage operations.
type StorageService interface {
	Read(ctx context.Context, path string) ([]byte, error)
	Write(ctx context.Context, path string, data []byte) error
	Delete(ctx context.Context, path string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// CacheService provides key-value caching operations.
type CacheService interface {
	Get(ctx context.Context, key string) (any, error)
	Set(ctx context.Context, key string, value any, ttl int) error
	Del(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
}

// StreamService provides durable message streaming (Redis Streams).
type StreamService interface {
	Publish(ctx context.Context, topic string, payload any) (string, error) // returns message ID
	Ack(ctx context.Context, topic string, group string, messageID string) error
}

// PubSubService provides real-time fan-out messaging (Redis PubSub).
type PubSubService interface {
	Publish(ctx context.Context, channel string, payload any) error
}

// ConnectionService provides WebSocket and SSE messaging.
type ConnectionService interface {
	Send(ctx context.Context, channel string, data any) error
	SendSSE(ctx context.Context, channel string, event string, data any, id string) error
}
