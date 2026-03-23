package registry

import (
	"fmt"
	"sync"

	"github.com/chimpanze/noda/pkg/api"
)

type serviceEntry struct {
	instance any
	plugin   api.Plugin
}

// ServiceRegistry holds all initialized service instances.
type ServiceRegistry struct {
	mu       sync.RWMutex
	services map[string]serviceEntry
	order    []string // initialization order for reverse shutdown
}

// NewServiceRegistry creates a new empty service registry.
func NewServiceRegistry() *ServiceRegistry {
	return &ServiceRegistry{
		services: make(map[string]serviceEntry),
	}
}

// Register stores a service instance with its owning plugin.
func (r *ServiceRegistry) Register(name string, instance any, plugin api.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.services[name]; exists {
		return fmt.Errorf("duplicate service name %q", name)
	}
	r.services[name] = serviceEntry{instance: instance, plugin: plugin}
	r.order = append(r.order, name)
	return nil
}

// Get looks up a service instance by name.
func (r *ServiceRegistry) Get(name string) (any, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.services[name]
	if !ok {
		return nil, false
	}
	return entry.instance, true
}

// getWithPlugin looks up a service instance and its owning plugin (test helper).
func (r *ServiceRegistry) getWithPlugin(name string) (any, api.Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.services[name]
	if !ok {
		return nil, nil, false
	}
	return entry.instance, entry.plugin, true
}

// GetPrefix returns the prefix of the plugin that owns the named service.
func (r *ServiceRegistry) GetPrefix(name string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, ok := r.services[name]
	if !ok {
		return "", false
	}
	if entry.plugin == nil {
		return "", false
	}
	return entry.plugin.Prefix(), true
}

// All returns all service instances keyed by name.
func (r *ServiceRegistry) All() map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]any, len(r.services))
	for name, entry := range r.services {
		result[name] = entry.instance
	}
	return result
}

// byPrefix returns service instances belonging to the given plugin prefix (test helper).
func (r *ServiceRegistry) byPrefix(prefix string) map[string]any {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]any)
	for name, entry := range r.services {
		if entry.plugin == nil {
			continue
		}
		if entry.plugin.Prefix() == prefix {
			result[name] = entry.instance
		}
	}
	return result
}

// Count returns the number of registered services.
func (r *ServiceRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.order)
}

// initOrder returns the service names in initialization order (test helper).
func (r *ServiceRegistry) initOrder() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, len(r.order))
	copy(result, r.order)
	return result
}
