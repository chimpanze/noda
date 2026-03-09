package http

import (
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

var httpServiceDeps = map[string]api.ServiceDep{
	"client": {Prefix: "http", Required: true},
}

func getHTTPService(services map[string]any) (*Service, error) {
	svc, ok := services["client"]
	if !ok {
		return nil, fmt.Errorf("http client service not configured")
	}
	hs, ok := svc.(*Service)
	if !ok {
		return nil, fmt.Errorf("service does not implement HTTP client")
	}
	return hs, nil
}

func resolveString(nCtx api.ExecutionContext, config map[string]any, key string) (string, bool, error) {
	raw, ok := config[key]
	if !ok {
		return "", false, nil
	}
	expr, ok := raw.(string)
	if !ok {
		return "", false, fmt.Errorf("field %q must be a string", key)
	}
	val, err := nCtx.Resolve(expr)
	if err != nil {
		return "", false, fmt.Errorf("resolve %q: %w", key, err)
	}
	s, ok := val.(string)
	if !ok {
		return "", false, fmt.Errorf("field %q resolved to %T, expected string", key, val)
	}
	return s, true, nil
}

func resolveRequiredString(nCtx api.ExecutionContext, config map[string]any, key string) (string, error) {
	s, ok, err := resolveString(nCtx, config, key)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("missing required field %q", key)
	}
	return s, nil
}

func resolveAny(nCtx api.ExecutionContext, config map[string]any, key string) (any, bool, error) {
	raw, ok := config[key]
	if !ok {
		return nil, false, nil
	}
	if expr, ok := raw.(string); ok {
		val, err := nCtx.Resolve(expr)
		if err != nil {
			return nil, false, fmt.Errorf("resolve %q: %w", key, err)
		}
		return val, true, nil
	}
	return raw, true, nil
}

func resolveHeaders(nCtx api.ExecutionContext, config map[string]any) (map[string]string, error) {
	raw, ok := config["headers"]
	if !ok {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("field \"headers\" must be a map")
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		expr, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("header %q value must be a string", k)
		}
		val, err := nCtx.Resolve(expr)
		if err != nil {
			return nil, fmt.Errorf("resolve header %q: %w", k, err)
		}
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("header %q resolved to %T, expected string", k, val)
		}
		result[k] = s
	}
	return result, nil
}
