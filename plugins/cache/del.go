package cache

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type delDescriptor struct{}

func (d *delDescriptor) Name() string        { return "del" }
func (d *delDescriptor) Description() string { return "Deletes a key from the cache" }
func (d *delDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"cache": {Prefix: "cache", Required: true},
	}
}
func (d *delDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"key": map[string]any{"type": "string", "description": "Cache key to delete"},
		},
		"required": []any{"key"},
	}
}
func (d *delDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "null (key was deleted)",
		"error":   "Connection error",
	}
}

func (d *delDescriptor) OutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ok": map[string]any{"type": "boolean"},
		},
		"required": []any{"ok"},
	}
}

type delExecutor struct{}

func newDelExecutor(_ map[string]any) api.NodeExecutor { return &delExecutor{} }

func (e *delExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *delExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[api.CacheService](services, "cache")
	if err != nil {
		return "", nil, fmt.Errorf("cache.del: %w", err)
	}

	key, err := plugin.ResolveString(nCtx, config, "key")
	if err != nil {
		return "", nil, fmt.Errorf("cache.del: %w", err)
	}

	if err := svc.Del(ctx, key); err != nil {
		return "", nil, fmt.Errorf("cache.del: %w", err)
	}

	return api.OutputSuccess, map[string]any{"ok": true}, nil
}
