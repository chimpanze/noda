package expr

import (
	"fmt"
	"strings"
)

// isStatic returns true if the string contains no {{ }} delimiters.
func isStatic(value string) bool {
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
		if !isStatic(val) {
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

// ValidateExpressions pre-compiles all {{ }} expressions found in a config map,
// returning errors for any that have syntax errors. This catches malformed
// expressions at startup rather than at runtime.
func ValidateExpressions(compiler *Compiler, config map[string]any) []error {
	var errs []error
	walkConfigExpressions(config, "", func(path, value string) {
		if !isStatic(value) {
			if _, err := compiler.Compile(value); err != nil {
				errs = append(errs, fmt.Errorf("expression error at %s: %w", path, err))
			}
		}
	})
	return errs
}

// walkConfigExpressions recursively walks a config map, calling fn for each
// string value that might contain an expression.
func walkConfigExpressions(m map[string]any, prefix string, fn func(path, value string)) {
	for key, val := range m {
		path := key
		if prefix != "" {
			path = prefix + "/" + key
		}
		switch v := val.(type) {
		case string:
			fn(path, v)
		case map[string]any:
			walkConfigExpressions(v, path, fn)
		case []any:
			for i, item := range v {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				switch iv := item.(type) {
				case string:
					fn(itemPath, iv)
				case map[string]any:
					walkConfigExpressions(iv, itemPath, fn)
				}
			}
		}
	}
}
