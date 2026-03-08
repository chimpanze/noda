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

// ConnectionService provides WebSocket and SSE messaging.
type ConnectionService interface {
	Send(ctx context.Context, channel string, data any) error
	SendSSE(ctx context.Context, channel string, event string, data any, id string) error
}
