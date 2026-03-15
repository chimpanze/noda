package cache

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type existsDescriptor struct{}

func (d *existsDescriptor) Name() string        { return "exists" }
func (d *existsDescriptor) Description() string { return "Checks if a key exists in the cache" }
func (d *existsDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"cache": {Prefix: "cache", Required: true},
	}
}
func (d *existsDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"key": map[string]any{"type": "string", "description": "Cache key to check"},
		},
		"required": []any{"key"},
	}
}
func (d *existsDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Boolean indicating if the key exists",
		"error":   "Connection error",
	}
}

type existsExecutor struct{}

func newExistsExecutor(_ map[string]any) api.NodeExecutor { return &existsExecutor{} }

func (e *existsExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *existsExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[api.CacheService](services, "cache")
	if err != nil {
		return "", nil, fmt.Errorf("cache.exists: %w", err)
	}

	key, err := plugin.ResolveString(nCtx, config, "key")
	if err != nil {
		return "", nil, fmt.Errorf("cache.exists: %w", err)
	}

	exists, err := svc.Exists(ctx, key)
	if err != nil {
		return "", nil, fmt.Errorf("cache.exists: %w", err)
	}

	return api.OutputSuccess, map[string]any{"exists": exists}, nil
}
