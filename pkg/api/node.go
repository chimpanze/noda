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

// NodeOutputSchemaProvider is optionally implemented by NodeDescriptor to declare
// a static JSON Schema for the node's output data.
type NodeOutputSchemaProvider interface {
	OutputSchema() map[string]any
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

// SubWorkflowRunner executes a sub-workflow and returns the output name and data.
// Used by control.loop and workflow.run nodes. Injected by the engine at dispatch time.
type SubWorkflowRunner interface {
	RunSubWorkflow(ctx context.Context, workflowID string, input any, parentCtx ExecutionContext) (outputName string, data any, err error)
}

// TransactionalSubWorkflowRunner extends SubWorkflowRunner with service override support
// for database transaction wrapping.
type TransactionalSubWorkflowRunner interface {
	SubWorkflowRunner
	RunSubWorkflowWithServices(ctx context.Context, workflowID string, input any, parentCtx ExecutionContext, serviceOverrides map[string]any) (outputName string, data any, err error)
}

// SubWorkflowInjectable is implemented by node executors that need a SubWorkflowRunner.
// The engine calls InjectSubWorkflowRunner after creating the executor.
type SubWorkflowInjectable interface {
	InjectSubWorkflowRunner(runner SubWorkflowRunner)
}

// Standard output names used by most nodes.
const (
	OutputSuccess = "success"
	OutputError   = "error"
)

// DefaultOutputs returns the standard outputs used by most nodes: ["success", "error"].
func DefaultOutputs() []string {
	return []string{OutputSuccess, OutputError}
}
