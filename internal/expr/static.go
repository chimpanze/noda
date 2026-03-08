package expr

import (
	"fmt"
	"strings"
)

// IsStatic returns true if the string contains no {{ }} delimiters.
func IsStatic(value string) bool {
	return !strings.Contains(value, "{{")
}

// ValidateStaticFields checks that the specified fields in a config map
// do not contain expressions. Returns errors for any fields that are expressions.
func ValidateStaticFields(config map[string]any, staticFields []string) []error {
	var errs []error
	for _, field := range staticFields {
		val, ok := getNestedString(config, field)
		if !ok {
			continue // field not present or not a string — skip
		}
		if !IsStatic(val) {
			errs = append(errs, fmt.Errorf("field %q must be a static value, not an expression", field))
		}
	}
	return errs
}

// getNestedString retrieves a string value from a nested map using dot notation.
func getNestedString(m map[string]any, path string) (string, bool) {
	parts := strings.Split(path, ".")
	current := any(m)

	for _, part := range parts {
		cm, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		current, ok = cm[part]
		if !ok {
			return "", false
		}
	}

	s, ok := current.(string)
	return s, ok
}
