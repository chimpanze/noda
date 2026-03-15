package http

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type getDescriptor struct{}

func (d *getDescriptor) Name() string                           { return "get" }
func (d *getDescriptor) Description() string                    { return "Shorthand for GET requests" }
func (d *getDescriptor) ServiceDeps() map[string]api.ServiceDep { return httpServiceDeps }
func (d *getDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":     map[string]any{"type": "string", "description": "Request URL"},
			"headers": map[string]any{"type": "object", "description": "Request headers"},
			"timeout": map[string]any{"type": "string", "description": "Request timeout"},
		},
		"required": []any{"url"},
	}
}
func (d *getDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Object with status, headers, and body from the HTTP response",
		"error":   "HTTP request error",
	}
}

type getExecutor struct{}

func newGetExecutor(_ map[string]any) api.NodeExecutor { return &getExecutor{} }

func (e *getExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *getExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "client")
	if err != nil {
		return "", nil, fmt.Errorf("http.get: %w", err)
	}
	return doRequest(ctx, nCtx, config, svc, "GET")
}
