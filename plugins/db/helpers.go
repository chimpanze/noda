package db

import (
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

// getDB extracts the *gorm.DB from the resolved services map.
func getDB(services map[string]any) (*gorm.DB, error) {
	svc, ok := services["database"]
	if !ok {
		return nil, fmt.Errorf("database service not configured")
	}
	db, ok := svc.(*gorm.DB)
	if !ok {
		return nil, fmt.Errorf("database service is not a *gorm.DB")
	}
	return db, nil
}

// resolveString resolves a config key as a string expression.
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

// resolveMap resolves a config key as a map expression.
func resolveMap(nCtx api.ExecutionContext, config map[string]any, key string) (map[string]any, error) {
	raw, ok := config[key]
	if !ok {
		return nil, fmt.Errorf("missing required field %q", key)
	}

	switch v := raw.(type) {
	case map[string]any:
		// Resolve each value that's an expression string
		result := make(map[string]any, len(v))
		for k, val := range v {
			if expr, ok := val.(string); ok {
				resolved, err := nCtx.Resolve(expr)
				if err != nil {
					return nil, fmt.Errorf("resolve %s.%s: %w", key, k, err)
				}
				result[k] = resolved
			} else {
				result[k] = val
			}
		}
		return result, nil
	case string:
		// Resolve as expression that should return a map
		val, err := nCtx.Resolve(v)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", key, err)
		}
		m, ok := val.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("field %q resolved to %T, expected map", key, val)
		}
		return m, nil
	default:
		return nil, fmt.Errorf("field %q must be an object or expression string", key)
	}
}

// resolveParams resolves the optional "params" config as a slice of values.
func resolveParams(nCtx api.ExecutionContext, config map[string]any) ([]any, error) {
	raw, ok := config["params"]
	if !ok {
		return nil, nil
	}

	switch v := raw.(type) {
	case []any:
		params := make([]any, len(v))
		for i, item := range v {
			if expr, ok := item.(string); ok {
				val, err := nCtx.Resolve(expr)
				if err != nil {
					return nil, fmt.Errorf("resolve params[%d]: %w", i, err)
				}
				params[i] = val
			} else {
				params[i] = item
			}
		}
		return params, nil
	case string:
		val, err := nCtx.Resolve(v)
		if err != nil {
			return nil, fmt.Errorf("resolve params: %w", err)
		}
		arr, ok := val.([]any)
		if !ok {
			return nil, fmt.Errorf("params resolved to %T, expected array", val)
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("params must be an array or expression string")
	}
}
