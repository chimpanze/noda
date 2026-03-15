package registry

import (
	"fmt"
	"log/slog"
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

// Register adds a plugin to the registry. If a prefix collision occurs between
// a node-only plugin and a service-only plugin, they are merged into a composite
// so that both node types and service creation work under the same prefix.
func (r *PluginRegistry) Register(plugin api.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := plugin.Prefix()
	existing, conflict := r.plugins[prefix]
	if !conflict {
		r.plugins[prefix] = plugin
		slog.Debug("plugin registered", "prefix", prefix, "name", plugin.Name(), "has_services", plugin.HasServices(), "nodes", len(plugin.Nodes()))
		return nil
	}

	// Allow merging when one provides nodes and the other provides services
	existingNodes := existing.Nodes()
	newNodes := plugin.Nodes()
	existingHasServices := existing.HasServices()
	newHasServices := plugin.HasServices()

	if len(existingNodes) > 0 && len(newNodes) == 0 && !existingHasServices && newHasServices {
		// Existing has nodes, new has services → merge
		r.plugins[prefix] = &compositePlugin{name: mergedName(existing, plugin), nodes: existing, services: plugin}
		return nil
	}
	if len(existingNodes) == 0 && len(newNodes) > 0 && existingHasServices && !newHasServices {
		// Existing has services, new has nodes → merge
		r.plugins[prefix] = &compositePlugin{name: mergedName(plugin, existing), nodes: plugin, services: existing}
		return nil
	}

	return fmt.Errorf("duplicate plugin prefix %q: %q and %q", prefix, existing.Name(), plugin.Name())
}

// mergedName returns a name for a composite plugin. If both plugins share the
// same name it is returned as-is; otherwise the names are joined with "+".
func mergedName(nodes, services api.Plugin) string {
	nn, sn := nodes.Name(), services.Name()
	if nn == sn {
		return nn
	}
	return nn + "+" + sn
}

// compositePlugin merges a node-providing plugin with a service-providing plugin
// that share the same prefix.
type compositePlugin struct {
	name     string     // explicit name preserved at merge time
	nodes    api.Plugin // provides Nodes()
	services api.Plugin // provides HasServices(), CreateService(), etc.
}

func (c *compositePlugin) Name() string   { return c.name }
func (c *compositePlugin) Prefix() string { return c.services.Prefix() }

func (c *compositePlugin) Nodes() []api.NodeRegistration {
	return c.nodes.Nodes()
}

func (c *compositePlugin) HasServices() bool { return true }
func (c *compositePlugin) CreateService(config map[string]any) (any, error) {
	return c.services.CreateService(config)
}
func (c *compositePlugin) HealthCheck(service any) error { return c.services.HealthCheck(service) }
func (c *compositePlugin) Shutdown(service any) error    { return c.services.Shutdown(service) }

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
