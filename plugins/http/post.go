package http

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type postDescriptor struct{}

func (d *postDescriptor) Name() string                           { return "post" }
func (d *postDescriptor) Description() string                    { return "Shorthand for POST requests" }
func (d *postDescriptor) ServiceDeps() map[string]api.ServiceDep { return httpServiceDeps }
func (d *postDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":     map[string]any{"type": "string", "description": "Request URL"},
			"headers": map[string]any{"type": "object", "description": "Request headers"},
			"body":    map[string]any{"description": "Request body"},
			"timeout": map[string]any{"type": "string", "description": "Request timeout"},
		},
		"required": []any{"url", "body"},
	}
}
func (d *postDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Object with status, headers, and body from the HTTP response",
		"error":   "HTTP request error",
	}
}

type postExecutor struct{}

func newPostExecutor(_ map[string]any) api.NodeExecutor { return &postExecutor{} }

func (e *postExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *postExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "client")
	if err != nil {
		return "", nil, fmt.Errorf("http.post: %w", err)
	}
	return doRequest(ctx, nCtx, config, svc, "POST")
}
