package cache

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type delDescriptor struct{}

func (d *delDescriptor) Name() string { return "del" }
func (d *delDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"cache": {Prefix: "cache", Required: true},
	}
}
func (d *delDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"key": map[string]any{"type": "string"},
		},
		"required": []any{"key"},
	}
}

type delExecutor struct{}

func newDelExecutor(_ map[string]any) api.NodeExecutor { return &delExecutor{} }

func (e *delExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *delExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := getCacheService(services)
	if err != nil {
		return "", nil, err
	}

	key, err := resolveString(nCtx, config, "key")
	if err != nil {
		return "", nil, fmt.Errorf("cache.del: %w", err)
	}

	if err := svc.Del(ctx, key); err != nil {
		return "", nil, fmt.Errorf("cache.del: %w", err)
	}

	return "success", map[string]any{"ok": true}, nil
}
