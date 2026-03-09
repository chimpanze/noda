package storage

import (
	"context"
	"fmt"
	"net/http"

	"github.com/chimpanze/noda/pkg/api"
)

type readDescriptor struct{}

func (d *readDescriptor) Name() string { return "read" }
func (d *readDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return storageDeps
}
func (d *readDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
		},
		"required": []any{"path"},
	}
}

type readExecutor struct{}

func newReadExecutor(_ map[string]any) api.NodeExecutor { return &readExecutor{} }

func (e *readExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *readExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := getStorageService(services)
	if err != nil {
		return "", nil, err
	}

	path, err := resolveString(nCtx, config, "path")
	if err != nil {
		return "", nil, fmt.Errorf("storage.read: %w", err)
	}

	data, err := svc.Read(ctx, path)
	if err != nil {
		return "", nil, err
	}

	contentType := http.DetectContentType(data)
	return "success", map[string]any{
		"data":         data,
		"size":         len(data),
		"content_type": contentType,
	}, nil
}
