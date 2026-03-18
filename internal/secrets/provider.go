package secrets

import "context"

// Provider loads secret key-value pairs from an external source.
type Provider interface {
	Name() string
	Load(ctx context.Context) (map[string]string, error)
}
