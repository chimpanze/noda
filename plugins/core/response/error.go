package response

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type errorDescriptor struct{}

func (d *errorDescriptor) Name() string                           { return "error" }
func (d *errorDescriptor) Description() string                    { return "Builds a standardized error response" }
func (d *errorDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *errorDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status":  map[string]any{"type": "string", "description": "HTTP status code (default: 500)"},
			"code":    map[string]any{"type": "string", "description": "Error code identifier"},
			"message": map[string]any{"type": "string", "description": "Human-readable error message"},
			"details": map[string]any{"type": "string", "description": "Additional error details"},
		},
		"required": []any{"status", "code", "message"},
	}
}

type errorExecutor struct{}

func newErrorExecutor(_ map[string]any) api.NodeExecutor {
	return &errorExecutor{}
}

func (e *errorExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *errorExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	// Resolve status (default 500 if absent)
	status := 500
	switch sv := config["status"].(type) {
	case string:
		if sv != "" {
			statusVal, err := nCtx.Resolve(sv)
			if err != nil {
				return "", nil, fmt.Errorf("response.error: status: %w", err)
			}
			if n, ok := plugin.ToInt(statusVal); ok {
				status = n
			}
		}
	default:
		if n, ok := plugin.ToInt(sv); ok {
			status = n
		}
	}

	// Resolve code
	codeExpr, _ := config["code"].(string)
	codeVal, err := nCtx.Resolve(codeExpr)
	if err != nil {
		return "", nil, fmt.Errorf("response.error: code: %w", err)
	}

	// Resolve message
	msgExpr, _ := config["message"].(string)
	msgVal, err := nCtx.Resolve(msgExpr)
	if err != nil {
		return "", nil, fmt.Errorf("response.error: message: %w", err)
	}

	// Resolve details (optional)
	var details any
	if detailsExpr, ok := config["details"].(string); ok && detailsExpr != "" {
		details, err = nCtx.Resolve(detailsExpr)
		if err != nil {
			return "", nil, fmt.Errorf("response.error: details: %w", err)
		}
	}

	// Build standardized error body with trace_id
	errorBody := map[string]any{
		"code":     fmt.Sprintf("%v", codeVal),
		"message":  fmt.Sprintf("%v", msgVal),
		"trace_id": nCtx.Trigger().TraceID,
	}
	if details != nil {
		errorBody["details"] = details
	}

	resp := &api.HTTPResponse{
		Status: status,
		Body: map[string]any{
			"error": errorBody,
		},
	}

	return api.OutputSuccess, resp, nil
}
