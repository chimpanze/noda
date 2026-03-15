package storage

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type writeDescriptor struct{}

func (d *writeDescriptor) Name() string        { return "write" }
func (d *writeDescriptor) Description() string { return "Writes data to storage" }
func (d *writeDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return storageDeps
}
func (d *writeDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":         map[string]any{"type": "string", "description": "File path to write"},
			"data":         map[string]any{"description": "Data to write"},
			"content_type": map[string]any{"type": "string", "description": "MIME type of the data"},
		},
		"required": []any{"path", "data"},
	}
}
func (d *writeDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Object with path of the written file",
		"error":   "Write error",
	}
}

type writeExecutor struct{}

func newWriteExecutor(_ map[string]any) api.NodeExecutor { return &writeExecutor{} }

func (e *writeExecutor) Outputs() []string { return api.DefaultOutputs() }

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

	return api.OutputSuccess, map[string]any{}, nil
}
