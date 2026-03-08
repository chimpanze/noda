package registry

import (
	"fmt"
	"strings"
	"sync"

	"github.com/chimpanze/noda/pkg/api"
)

// NodeRegistry holds node type descriptors and executor factories.
type NodeRegistry struct {
	mu          sync.RWMutex
	descriptors map[string]api.NodeDescriptor
	factories   map[string]func(map[string]any) api.NodeExecutor
}

// NewNodeRegistry creates a new empty node registry.
func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		descriptors: make(map[string]api.NodeDescriptor),
		factories:   make(map[string]func(map[string]any) api.NodeExecutor),
	}
}

// RegisterFromPlugin registers all nodes from a plugin under prefix.name format.
func (r *NodeRegistry) RegisterFromPlugin(plugin api.Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	prefix := plugin.Prefix()
	for _, reg := range plugin.Nodes() {
		fullType := prefix + "." + reg.Descriptor.Name()
		if _, exists := r.descriptors[fullType]; exists {
			return fmt.Errorf("duplicate node type %q", fullType)
		}
		r.descriptors[fullType] = reg.Descriptor
		r.factories[fullType] = reg.Factory
	}
	return nil
}

// RegisterFactory registers a single node type with a factory function.
// This is used by the testing framework to register mock node factories.
func (r *NodeRegistry) RegisterFactory(nodeType string, factory func(map[string]any) api.NodeExecutor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[nodeType] = factory
}

// GetDescriptor looks up a node descriptor by full type (e.g., "db.query").
func (r *NodeRegistry) GetDescriptor(nodeType string) (api.NodeDescriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.descriptors[nodeType]
	return d, ok
}

// GetFactory looks up an executor factory by full type.
func (r *NodeRegistry) GetFactory(nodeType string) (func(map[string]any) api.NodeExecutor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	f, ok := r.factories[nodeType]
	return f, ok
}

// AllTypes returns all registered node types.
func (r *NodeRegistry) AllTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]string, 0, len(r.descriptors))
	for t := range r.descriptors {
		result = append(result, t)
	}
	return result
}

// TypesByPrefix returns node types matching the given prefix.
func (r *NodeRegistry) TypesByPrefix(prefix string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []string
	pfx := prefix + "."
	for t := range r.descriptors {
		if strings.HasPrefix(t, pfx) {
			result = append(result, t)
		}
	}
	return result
}
