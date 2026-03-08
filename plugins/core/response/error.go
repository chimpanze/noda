package response

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type errorDescriptor struct{}

func (d *errorDescriptor) Name() string                           { return "error" }
func (d *errorDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *errorDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"status":  map[string]any{"type": "string"},
			"code":    map[string]any{"type": "string"},
			"message": map[string]any{"type": "string"},
			"details": map[string]any{"type": "string"},
		},
		"required": []any{"status", "code", "message"},
	}
}

type errorExecutor struct{}

func newErrorExecutor(_ map[string]any) api.NodeExecutor {
	return &errorExecutor{}
}

func (e *errorExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *errorExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	// Resolve status
	statusExpr, _ := config["status"].(string)
	if statusExpr == "" {
		statusExpr = "500"
	}
	statusVal, err := nCtx.Resolve(statusExpr)
	if err != nil {
		return "", nil, fmt.Errorf("response.error: status: %w", err)
	}
	status := toInt(statusVal)
	if status == 0 {
		status = 500
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

	return "success", resp, nil
}
