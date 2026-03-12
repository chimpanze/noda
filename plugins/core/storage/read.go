package storage

import (
	"context"
	"fmt"
	"net/http"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type readDescriptor struct{}

func (d *readDescriptor) Name() string        { return "read" }
func (d *readDescriptor) Description() string { return "Reads a file from storage" }
func (d *readDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return storageDeps
}
func (d *readDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "File path to read"},
		},
		"required": []any{"path"},
	}
}

type readExecutor struct{}

func newReadExecutor(_ map[string]any) api.NodeExecutor { return &readExecutor{} }

func (e *readExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *readExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[api.StorageService](services, "storage")
	if err != nil {
		return "", nil, err
	}

	path, err := plugin.ResolveString(nCtx, config, "path")
	if err != nil {
		return "", nil, fmt.Errorf("storage.read: %w", err)
	}

	data, err := svc.Read(ctx, path)
	if err != nil {
		return "", nil, err
	}

	contentType := http.DetectContentType(data)
	return api.OutputSuccess, map[string]any{
		"data":         data,
		"size":         len(data),
		"content_type": contentType,
	}, nil
}
