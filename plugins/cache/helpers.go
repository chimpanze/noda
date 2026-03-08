package cache

import (
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

func getCacheService(services map[string]any) (api.CacheService, error) {
	svc, ok := services["cache"]
	if !ok {
		return nil, fmt.Errorf("cache service not configured")
	}
	cs, ok := svc.(api.CacheService)
	if !ok {
		return nil, fmt.Errorf("cache service does not implement CacheService")
	}
	return cs, nil
}

func resolveString(nCtx api.ExecutionContext, config map[string]any, key string) (string, error) {
	raw, ok := config[key]
	if !ok {
		return "", fmt.Errorf("missing required field %q", key)
	}
	expr, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("field %q must be a string", key)
	}
	val, err := nCtx.Resolve(expr)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", key, err)
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("field %q resolved to %T, expected string", key, val)
	}
	return s, nil
}

func resolveAny(nCtx api.ExecutionContext, config map[string]any, key string) (any, error) {
	raw, ok := config[key]
	if !ok {
		return nil, fmt.Errorf("missing required field %q", key)
	}
	if expr, ok := raw.(string); ok {
		return nCtx.Resolve(expr)
	}
	return raw, nil
}

func resolveInt(nCtx api.ExecutionContext, raw any) (int, error) {
	switch v := raw.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case string:
		val, err := nCtx.Resolve(v)
		if err != nil {
			return 0, err
		}
		switch n := val.(type) {
		case float64:
			return int(n), nil
		case int:
			return n, nil
		}
		return 0, fmt.Errorf("resolved to %T, expected number", val)
	}
	return 0, fmt.Errorf("expected number, got %T", raw)
}
