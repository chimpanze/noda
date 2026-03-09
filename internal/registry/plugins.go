package registry

import (
	"fmt"
	"sync"

	"github.com/chimpanze/noda/pkg/api"
)

// PluginRegistry is the central registry where plugins register by prefix.
type PluginRegistry struct {
	mu      sync.RWMutex
	plugins map[string]api.Plugin // keyed by prefix
}

// NewPluginRegistry creates a new empty plugin registry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{
		plugins: make(map[string]api.Plugin),
	}
}

// Register adds a plugin to the registry. Returns an error if the prefix is already taken.
func (r *PluginRegistry) Register(plugin api.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := plugin.Prefix()
	if existing, ok := r.plugins[prefix]; ok {
		return fmt.Errorf("duplicate plugin prefix %q: %q and %q", prefix, existing.Name(), plugin.Name())
	}
	r.plugins[prefix] = plugin
	return nil
}

// Get looks up a plugin by prefix.
func (r *PluginRegistry) Get(prefix string) (api.Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.plugins[prefix]
	return p, ok
}

// GetByName looks up a plugin by its name (e.g. "postgres").
// Falls back to prefix lookup if no name match is found.
func (r *PluginRegistry) GetByName(name string) (api.Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.plugins {
		if p.Name() == name {
			return p, true
		}
	}
	// Fall back to prefix lookup for plugins where name == prefix
	p, ok := r.plugins[name]
	return p, ok
}

// All returns all registered plugins.
func (r *PluginRegistry) All() []api.Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]api.Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, p)
	}
	return result
}

// Prefixes returns all registered prefixes.
func (r *PluginRegistry) Prefixes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.plugins))
	for prefix := range r.plugins {
		result = append(result, prefix)
	}
	return result
}
