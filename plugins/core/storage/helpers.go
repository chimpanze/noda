package storage

import (
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

var storageDeps = map[string]api.ServiceDep{
	"storage": {Prefix: "storage", Required: true},
}

func getStorageService(services map[string]any) (api.StorageService, error) {
	svc, ok := services["storage"]
	if !ok {
		return nil, fmt.Errorf("storage service not configured")
	}
	ss, ok := svc.(api.StorageService)
	if !ok {
		return nil, fmt.Errorf("storage service does not implement StorageService")
	}
	return ss, nil
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
