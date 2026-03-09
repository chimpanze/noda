package plugin

import (
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

// ResolveString resolves a required config key as a string expression.
// Returns an error if the key is missing.
func ResolveString(nCtx api.ExecutionContext, config map[string]any, key string) (string, error) {
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

// ResolveOptionalString resolves an optional config key as a string expression.
// Returns ("", false, nil) if the key is absent.
func ResolveOptionalString(nCtx api.ExecutionContext, config map[string]any, key string) (string, bool, error) {
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

// ResolveAny resolves a required config key as any type.
func ResolveAny(nCtx api.ExecutionContext, config map[string]any, key string) (any, error) {
	raw, ok := config[key]
	if !ok {
		return nil, fmt.Errorf("missing required field %q", key)
	}
	if expr, ok := raw.(string); ok {
		return nCtx.Resolve(expr)
	}
	return raw, nil
}

// ResolveOptionalAny resolves an optional config key as any type.
// Returns (nil, false, nil) if the key is absent.
func ResolveOptionalAny(nCtx api.ExecutionContext, config map[string]any, key string) (any, bool, error) {
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

// ResolveInt resolves an optional config key as an integer.
// Returns (0, false, nil) if the key is absent.
func ResolveInt(nCtx api.ExecutionContext, config map[string]any, key string) (int, bool, error) {
	raw, ok := config[key]
	if !ok {
		return 0, false, nil
	}
	switch v := raw.(type) {
	case float64:
		return int(v), true, nil
	case int:
		return v, true, nil
	case string:
		val, err := nCtx.Resolve(v)
		if err != nil {
			return 0, false, fmt.Errorf("resolve %q: %w", key, err)
		}
		switch n := val.(type) {
		case float64:
			return int(n), true, nil
		case int:
			return n, true, nil
		}
		return 0, false, fmt.Errorf("field %q resolved to %T, expected int", key, val)
	}
	return 0, false, fmt.Errorf("field %q has invalid type %T", key, raw)
}

// ToInt converts a numeric value (float64, int, int64) to int.
// Returns (0, false) if the value is not a recognized numeric type.
func ToInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	}
	return 0, false
}

// ToInt64 converts a numeric value (float64, int, int64) to int64.
// Returns (0, false) if the value is not a recognized numeric type.
func ToInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	}
	return 0, false
}

// ResolveIntRaw resolves a raw value (already extracted from config) as an integer.
func ResolveIntRaw(nCtx api.ExecutionContext, raw any) (int, error) {
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
