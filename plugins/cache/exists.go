package cache

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type existsDescriptor struct{}

func (d *existsDescriptor) Name() string { return "exists" }
func (d *existsDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"cache": {Prefix: "cache", Required: true},
	}
}
func (d *existsDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"key": map[string]any{"type": "string"},
		},
		"required": []any{"key"},
	}
}

type existsExecutor struct{}

func newExistsExecutor(_ map[string]any) api.NodeExecutor { return &existsExecutor{} }

func (e *existsExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *existsExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := getCacheService(services)
	if err != nil {
		return "", nil, err
	}

	key, err := resolveString(nCtx, config, "key")
	if err != nil {
		return "", nil, fmt.Errorf("cache.exists: %w", err)
	}

	exists, err := svc.Exists(ctx, key)
	if err != nil {
		return "", nil, fmt.Errorf("cache.exists: %w", err)
	}

	return "success", map[string]any{"exists": exists}, nil
}
