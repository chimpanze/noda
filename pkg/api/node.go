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
	Description() string
	ServiceDeps() map[string]ServiceDep
	ConfigSchema() map[string]any          // JSON Schema as a Go map
	OutputDescriptions() map[string]string // describes data shape per output port
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

// WorkflowRunner executes a workflow by ID with input data.
// Used by the connection manager and Wasm runtime to trigger workflow execution.
type WorkflowRunner func(ctx context.Context, workflowID string, input map[string]any) error

// Standard output names used by most nodes.
const (
	OutputSuccess = "success"
	OutputError   = "error"
)

// DefaultOutputs returns the standard outputs used by most nodes: ["success", "error"].
func DefaultOutputs() []string {
	return []string{OutputSuccess, OutputError}
}
