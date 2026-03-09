package storage

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type deleteDescriptor struct{}

func (d *deleteDescriptor) Name() string { return "delete" }
func (d *deleteDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return storageDeps
}
func (d *deleteDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []any{"path"},
	}
}

type deleteExecutor struct{}

func newDeleteExecutor(_ map[string]any) api.NodeExecutor { return &deleteExecutor{} }

func (e *deleteExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *deleteExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[api.StorageService](services, "storage")
	if err != nil {
		return "", nil, err
	}

	path, err := plugin.ResolveString(nCtx, config, "path")
	if err != nil {
		return "", nil, fmt.Errorf("storage.delete: %w", err)
	}

	if err := svc.Delete(ctx, path); err != nil {
		return "", nil, err
	}

	return api.OutputSuccess, map[string]any{}, nil
}
