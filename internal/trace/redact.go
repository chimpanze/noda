package trace

import "strings"

// sensitiveContains lists substrings (lowercase) that make any key sensitive.
var sensitiveContains = []string{
	"password",
	"secret",
	"token",
	"authorization",
	"credential",
	"api_key",
	"apikey",
}

// sensitiveExact lists exact key names (lowercase) that are sensitive.
var sensitiveExact = []string{
	"key",
}

// redactSecrets returns a deep copy of the map with values redacted for keys
// matching common sensitive patterns. Nested maps are walked recursively.
// Slices and non-map values are left untouched.
func redactSecrets(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if IsSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		switch val := v.(type) {
		case map[string]any:
			out[k] = redactSecrets(val)
		case []any:
			out[k] = redactSlice(val)
		default:
			out[k] = v
		}
	}
	return out
}

// redactSlice returns a copy of the slice with sensitive values in nested maps redacted.
func redactSlice(s []any) []any {
	out := make([]any, len(s))
	for i, v := range s {
		switch val := v.(type) {
		case map[string]any:
			out[i] = redactSecrets(val)
		case []any:
			out[i] = redactSlice(val)
		default:
			out[i] = v
		}
	}
	return out
}

// IsSensitiveKey checks whether the key matches any sensitive pattern (case-insensitive).
func IsSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, pattern := range sensitiveContains {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	for _, exact := range sensitiveExact {
		if lower == exact {
			return true
		}
	}
	return false
}
