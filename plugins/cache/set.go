package cache

import (
	"context"
	"fmt"

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

func (e *setExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *setExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := getCacheService(services)
	if err != nil {
		return "", nil, err
	}

	key, err := resolveString(nCtx, config, "key")
	if err != nil {
		return "", nil, fmt.Errorf("cache.set: %w", err)
	}

	value, err := resolveAny(nCtx, config, "value")
	if err != nil {
		return "", nil, fmt.Errorf("cache.set: %w", err)
	}

	ttl := 0
	if ttlRaw, ok := config["ttl"]; ok {
		ttl, err = resolveInt(nCtx, ttlRaw)
		if err != nil {
			return "", nil, fmt.Errorf("cache.set ttl: %w", err)
		}
	}

	if err := svc.Set(ctx, key, value, ttl); err != nil {
		return "", nil, fmt.Errorf("cache.set: %w", err)
	}

	return "success", map[string]any{"ok": true}, nil
}
