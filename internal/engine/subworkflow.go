package engine

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
)

// SubWorkflowRunnerImpl executes sub-workflows for control.loop and workflow.run nodes.
// It implements api.SubWorkflowRunner and api.TransactionalSubWorkflowRunner.
type SubWorkflowRunnerImpl struct {
	Cache    *WorkflowCache
	Services *registry.ServiceRegistry
	Nodes    *registry.NodeRegistry
}

// RunSubWorkflow executes a sub-workflow with the given input,
// inheriting context from the parent execution.
func (r *SubWorkflowRunnerImpl) RunSubWorkflow(
	ctx context.Context,
	workflowID string,
	input any,
	parentCtx api.ExecutionContext,
) (string, any, error) {
	return r.runSubWorkflow(ctx, workflowID, input, parentCtx, nil)
}

// RunSubWorkflowWithServices executes a sub-workflow with service overrides
// (used for database transaction wrapping).
func (r *SubWorkflowRunnerImpl) RunSubWorkflowWithServices(
	ctx context.Context,
	workflowID string,
	input any,
	parentCtx api.ExecutionContext,
	serviceOverrides map[string]any,
) (string, any, error) {
	return r.runSubWorkflow(ctx, workflowID, input, parentCtx, serviceOverrides)
}

func (r *SubWorkflowRunnerImpl) runSubWorkflow(
	ctx context.Context,
	workflowID string,
	input any,
	parentCtx api.ExecutionContext,
	serviceOverrides map[string]any,
) (string, any, error) {
	graph, ok := r.Cache.Get(workflowID)
	if !ok {
		return "", nil, fmt.Errorf("sub-workflow %q not found", workflowID)
	}

	// Build child execution context inheriting parent state.
	parent, ok := parentCtx.(*ExecutionContextImpl)
	if !ok {
		return "", nil, fmt.Errorf("sub-workflow %q: unsupported parent context type", workflowID)
	}

	childOpts := []ExecutionContextOption{
		WithInput(input),
		WithWorkflowID(workflowID),
		WithCompiler(parent.compiler),
		WithLogger(parent.logger),
		WithSecrets(parent.secretsContext),
		WithTracer(parent.tracer),
	}
	if parent.traceCallback != nil {
		childOpts = append(childOpts, WithTraceCallback(parent.traceCallback))
	}
	if parent.metrics != nil {
		childOpts = append(childOpts, WithMetricsInst(parent.metrics))
	}
	// Inherit auth and trigger from parent
	childOpts = append(childOpts, WithAuth(parent.auth))
	childOpts = append(childOpts, WithTrigger(parent.trigger))
	// Propagate the sub-workflow runner so nested sub-workflows work
	childOpts = append(childOpts, WithSubWorkflowRunner(r))

	childCtx := NewExecutionContext(childOpts...)

	// Inherit depth from parent
	childCtx.depth = parent.depth
	childCtx.maxDepth = parent.maxDepth

	// Apply service overrides if provided (for transactions)
	services := r.Services
	if len(serviceOverrides) > 0 {
		services = r.Services.WithOverrides(serviceOverrides)
	}

	err := ExecuteGraph(ctx, graph, childCtx, services, r.Nodes)
	if err != nil {
		return "", nil, err
	}

	// Collect the output from terminal nodes.
	// The sub-workflow's output is the data from the last completed node
	// that produced a workflow.output result.
	outputName, data := childCtx.WorkflowOutput()
	return outputName, data, nil
}
