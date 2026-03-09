package storage

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type writeDescriptor struct{}

func (d *writeDescriptor) Name() string { return "write" }
func (d *writeDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return storageDeps
}
func (d *writeDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":         map[string]any{"type": "string"},
			"data":         map[string]any{},
			"content_type": map[string]any{"type": "string"},
		},
		"required": []any{"path", "data"},
	}
}

type writeExecutor struct{}

func newWriteExecutor(_ map[string]any) api.NodeExecutor { return &writeExecutor{} }

func (e *writeExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *writeExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[api.StorageService](services, "storage")
	if err != nil {
		return "", nil, err
	}

	path, err := plugin.ResolveString(nCtx, config, "path")
	if err != nil {
		return "", nil, fmt.Errorf("storage.write: %w", err)
	}

	rawData, err := plugin.ResolveAny(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("storage.write: %w", err)
	}

	var data []byte
	switch v := rawData.(type) {
	case []byte:
		data = v
	case string:
		data = []byte(v)
	default:
		return "", nil, fmt.Errorf("storage.write: data must be string or bytes, got %T", rawData)
	}

	if err := svc.Write(ctx, path, data); err != nil {
		return "", nil, err
	}

	return "success", map[string]any{}, nil
}
