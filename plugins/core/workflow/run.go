package workflow

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

// depthTracker is implemented by execution contexts that support recursion depth tracking.
type depthTracker interface {
	CheckAndIncrementDepth() error
	DecrementDepth()
}

type runDescriptor struct{}

func (d *runDescriptor) Name() string { return "run" }
func (d *runDescriptor) Description() string {
	return "Executes a sub-workflow with optional transaction wrapping"
}
func (d *runDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: false},
	}
}
func (d *runDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow":    map[string]any{"type": "string", "description": "Sub-workflow ID to execute"},
			"input":       map[string]any{"type": "object", "description": "Input data mapping"},
			"transaction": map[string]any{"type": "boolean", "description": "Wrap execution in a database transaction"},
		},
		"required": []any{"workflow"},
	}
}
func (d *runDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Output from the sub-workflow",
		"error":   "Sub-workflow execution error",
	}
}

// RunExecutor executes a sub-workflow.
// SubWorkflowRunner is injected by the engine to avoid circular imports.
type RunExecutor struct {
	Runner  SubWorkflowRunner
	outputs []string
}

// SubWorkflowRunner executes a sub-workflow and returns the output name and data.
type SubWorkflowRunner interface {
	RunSubWorkflow(ctx context.Context, workflowID string, input any, parentCtx api.ExecutionContext) (outputName string, data any, err error)
}

// TransactionalRunner extends SubWorkflowRunner with service override support for transactions.
type TransactionalRunner interface {
	SubWorkflowRunner
	RunSubWorkflowWithServices(ctx context.Context, workflowID string, input any, parentCtx api.ExecutionContext, serviceOverrides map[string]any) (outputName string, data any, err error)
}

func newRunExecutor(config map[string]any) api.NodeExecutor {
	// Collect outputs from sub-workflow's workflow.output nodes
	// For now, use a default set. The engine will inject proper outputs.
	return &RunExecutor{
		outputs: []string{"success", "error"},
	}
}

func (e *RunExecutor) Outputs() []string { return e.outputs }

// setOutputs allows the engine to set the dynamic outputs after discovering
// the sub-workflow's workflow.output nodes (test helper).
func (e *RunExecutor) setOutputs(outputs []string) {
	e.outputs = outputs
}

func (e *RunExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	workflowID, _ := config["workflow"].(string)

	if e.Runner == nil {
		return "", nil, fmt.Errorf("workflow.run: sub-workflow runner not configured")
	}

	// Resolve input map
	var input any
	if inputMap, ok := config["input"].(map[string]any); ok {
		resolved := make(map[string]any, len(inputMap))
		for k, v := range inputMap {
			if expr, ok := v.(string); ok {
				val, err := nCtx.Resolve(expr)
				if err != nil {
					return "", nil, fmt.Errorf("workflow.run: resolve input %q: %w", k, err)
				}
				resolved[k] = val
			} else {
				resolved[k] = v
			}
		}
		input = resolved
	}

	// Check recursion depth
	if dt, ok := nCtx.(depthTracker); ok {
		if err := dt.CheckAndIncrementDepth(); err != nil {
			return "", nil, fmt.Errorf("workflow.run: %w", err)
		}
		defer dt.DecrementDepth()
	}

	// Check if transaction mode is enabled
	transaction, _ := config["transaction"].(bool)
	if transaction {
		return e.executeWithTransaction(ctx, workflowID, input, nCtx, services)
	}

	outputName, data, err := e.Runner.RunSubWorkflow(ctx, workflowID, input, nCtx)
	if err != nil {
		return "", nil, err
	}

	return outputName, data, nil
}

// executeWithTransaction wraps the sub-workflow in a database transaction.
func (e *RunExecutor) executeWithTransaction(ctx context.Context, workflowID string, input any, nCtx api.ExecutionContext, services map[string]any) (string, any, error) {
	txRunner, ok := e.Runner.(TransactionalRunner)
	if !ok {
		return "", nil, fmt.Errorf("workflow.run: runner does not support transactions")
	}

	// Get database service
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("workflow.run: transaction requires a database service: %w", err)
	}

	var (
		outputName string
		data       any
		runErr     error
	)

	txErr := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Create service overrides: replace the database service with the transaction
		overrides := map[string]any{
			"database": tx,
		}

		outputName, data, runErr = txRunner.RunSubWorkflowWithServices(ctx, workflowID, input, nCtx, overrides)
		if runErr != nil {
			return runErr // triggers rollback
		}
		return nil // triggers commit
	})

	if txErr != nil {
		return "", nil, txErr
	}

	return outputName, data, nil
}
