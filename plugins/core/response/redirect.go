package response

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type redirectDescriptor struct{}

func (d *redirectDescriptor) Name() string                           { return "redirect" }
func (d *redirectDescriptor) Description() string                    { return "Builds an HTTP redirect response" }
func (d *redirectDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *redirectDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":    map[string]any{"type": "string", "description": "Redirect target URL"},
			"status": map[string]any{"type": "integer", "description": "HTTP status code (default: 302)"},
		},
		"required": []any{"url"},
	}
}

type redirectExecutor struct{}

func newRedirectExecutor(_ map[string]any) api.NodeExecutor {
	return &redirectExecutor{}
}

func (e *redirectExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *redirectExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	// Resolve URL
	urlExpr, _ := config["url"].(string)
	if urlExpr == "" {
		return "", nil, fmt.Errorf("response.redirect: url is required")
	}
	urlVal, err := nCtx.Resolve(urlExpr)
	if err != nil {
		return "", nil, fmt.Errorf("response.redirect: url: %w", err)
	}
	urlStr := fmt.Sprintf("%v", urlVal)

	// Status: static integer, default 302
	status := 302
	if s, ok := config["status"].(float64); ok {
		status = int(s)
	}

	resp := &api.HTTPResponse{
		Status: status,
		Headers: map[string]string{
			"Location": urlStr,
		},
		Body: nil,
	}

	return api.OutputSuccess, resp, nil
}
