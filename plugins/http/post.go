package http

import (
	"context"

	"github.com/chimpanze/noda/pkg/api"
)

type postDescriptor struct{}

func (d *postDescriptor) Name() string                          { return "post" }
func (d *postDescriptor) ServiceDeps() map[string]api.ServiceDep { return httpServiceDeps }
func (d *postDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":     map[string]any{"type": "string"},
			"headers": map[string]any{"type": "object"},
			"body":    map[string]any{},
			"timeout": map[string]any{"type": "string"},
		},
		"required": []any{"url", "body"},
	}
}

type postExecutor struct{}

func newPostExecutor(_ map[string]any) api.NodeExecutor { return &postExecutor{} }

func (e *postExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *postExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := getHTTPService(services)
	if err != nil {
		return "", nil, err
	}
	return doRequest(ctx, nCtx, config, svc, "POST")
}
