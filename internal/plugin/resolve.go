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

// ToInt converts a numeric value (float64, int, int64, or numeric string) to int.
// Returns (0, false) if the value is not a recognized numeric type.
func ToInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case string:
		var i int
		if _, err := fmt.Sscanf(n, "%d", &i); err == nil {
			return i, true
		}
		return 0, false
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

// ResolveHeaders resolves a "headers" config field as a map of string→string.
// Each value is resolved as an expression. Non-string resolved values are
// formatted via fmt.Sprintf. Returns nil if the field is absent.
func ResolveHeaders(nCtx api.ExecutionContext, config map[string]any) (map[string]string, error) {
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
			result[k] = fmt.Sprintf("%v", v)
			continue
		}
		val, err := nCtx.Resolve(expr)
		if err != nil {
			return nil, fmt.Errorf("resolve header %q: %w", k, err)
		}
		result[k] = fmt.Sprintf("%v", val)
	}
	return result, nil
}
