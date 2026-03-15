package cache

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type getDescriptor struct{}

func (d *getDescriptor) Name() string        { return "get" }
func (d *getDescriptor) Description() string { return "Retrieves a value from the cache" }
func (d *getDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"cache": {Prefix: "cache", Required: true},
	}
}
func (d *getDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"key": map[string]any{"type": "string", "description": "Cache key to retrieve"},
		},
		"required": []any{"key"},
	}
}
func (d *getDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "The cached value (string or deserialized object)",
		"error":   "Cache miss or connection error",
	}
}

type getExecutor struct{}

func newGetExecutor(_ map[string]any) api.NodeExecutor { return &getExecutor{} }

func (e *getExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *getExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[api.CacheService](services, "cache")
	if err != nil {
		return "", nil, fmt.Errorf("cache.get: %w", err)
	}

	key, err := plugin.ResolveString(nCtx, config, "key")
	if err != nil {
		return "", nil, fmt.Errorf("cache.get: %w", err)
	}

	value, err := svc.Get(ctx, key)
	if err != nil {
		return "", nil, err
	}

	return api.OutputSuccess, map[string]any{"value": value}, nil
}
