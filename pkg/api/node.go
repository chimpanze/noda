package api

import "context"

// ServiceDep describes a service dependency for a node.
type ServiceDep struct {
	Prefix   string
	Required bool
}

// NodeDescriptor describes a node's metadata and requirements.
type NodeDescriptor interface {
	Name() string
	ServiceDeps() map[string]ServiceDep
	ConfigSchema() map[string]any // JSON Schema as a Go map
}

// NodeRegistration pairs a node descriptor with its factory function.
type NodeRegistration struct {
	Descriptor NodeDescriptor
	Factory    func(config map[string]any) NodeExecutor
}

// NodeExecutor implements the logic for a single node type.
type NodeExecutor interface {
	Outputs() []string
	Execute(ctx context.Context, nCtx ExecutionContext, config map[string]any, services map[string]any) (outputName string, data any, err error)
}
