package secrets

import (
	"context"
	"fmt"
	"sort"
)

// Manager loads secrets from configured providers and exposes them
// for both config-time $env() resolution and runtime expression evaluation.
type Manager struct {
	secrets   map[string]string
	providers []Provider
}

// New creates a Manager with the given providers.
// If no providers are given, Get/Has/Keys return empty results.
func New(providers ...Provider) *Manager {
	return &Manager{
		secrets:   make(map[string]string),
		providers: providers,
	}
}

// Load calls all providers in order and merges their results.
// Later providers override earlier ones for the same key.
func (m *Manager) Load(ctx context.Context) error {
	merged := make(map[string]string)
	for _, p := range m.providers {
		vals, err := p.Load(ctx)
		if err != nil {
			return fmt.Errorf("secrets provider %q: %w", p.Name(), err)
		}
		for k, v := range vals {
			merged[k] = v
		}
	}
	m.secrets = merged
	return nil
}

// Get returns a secret value by key.
func (m *Manager) Get(key string) (string, bool) {
	v, ok := m.secrets[key]
	return v, ok
}

// Has returns true if the key exists in loaded secrets.
func (m *Manager) Has(key string) bool {
	_, ok := m.secrets[key]
	return ok
}

// Keys returns all secret keys in sorted order.
func (m *Manager) Keys() []string {
	keys := make([]string, 0, len(m.secrets))
	for k := range m.secrets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ExpressionContext returns the secrets as a map[string]any for use in
// expression evaluation (the "secrets" namespace).
func (m *Manager) ExpressionContext() map[string]any {
	result := make(map[string]any, len(m.secrets))
	for k, v := range m.secrets {
		result[k] = v
	}
	return result
}
