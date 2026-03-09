package cache

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type setDescriptor struct{}

func (d *setDescriptor) Name() string { return "set" }
func (d *setDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"cache": {Prefix: "cache", Required: true},
	}
}
func (d *setDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"key":   map[string]any{"type": "string"},
			"value": map[string]any{},
			"ttl":   map[string]any{"type": "integer"},
		},
		"required": []any{"key", "value"},
	}
}

type setExecutor struct{}

func newSetExecutor(_ map[string]any) api.NodeExecutor { return &setExecutor{} }

func (e *setExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *setExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[api.CacheService](services, "cache")
	if err != nil {
		return "", nil, err
	}

	key, err := plugin.ResolveString(nCtx, config, "key")
	if err != nil {
		return "", nil, fmt.Errorf("cache.set: %w", err)
	}

	value, err := plugin.ResolveAny(nCtx, config, "value")
	if err != nil {
		return "", nil, fmt.Errorf("cache.set: %w", err)
	}

	ttl := 0
	if ttlRaw, ok := config["ttl"]; ok {
		ttl, err = plugin.ResolveIntRaw(nCtx, ttlRaw)
		if err != nil {
			return "", nil, fmt.Errorf("cache.set ttl: %w", err)
		}
	}

	if err := svc.Set(ctx, key, value, ttl); err != nil {
		return "", nil, fmt.Errorf("cache.set: %w", err)
	}

	return api.OutputSuccess, map[string]any{"ok": true}, nil
}
