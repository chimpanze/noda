package storage

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type listDescriptor struct{}

func (d *listDescriptor) Name() string { return "list" }
func (d *listDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return storageDeps
}
func (d *listDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prefix": map[string]any{"type": "string"},
		},
		"required": []any{"prefix"},
	}
}

type listExecutor struct{}

func newListExecutor(_ map[string]any) api.NodeExecutor { return &listExecutor{} }

func (e *listExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *listExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := getStorageService(services)
	if err != nil {
		return "", nil, err
	}

	prefix, err := resolveString(nCtx, config, "prefix")
	if err != nil {
		return "", nil, fmt.Errorf("storage.list: %w", err)
	}

	paths, err := svc.List(ctx, prefix)
	if err != nil {
		return "", nil, err
	}

	if paths == nil {
		paths = []string{}
	}

	return "success", map[string]any{"paths": paths}, nil
}
