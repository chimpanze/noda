package secrets

import (
	"fmt"
	"regexp"
)

// envPattern matches {{ $env('KEY') }} patterns in config strings.
var envPattern = regexp.MustCompile(`\{\{\s*\$env\(\s*'([^']+)'\s*\)\s*\}\}`)

// EnvPattern returns the compiled regex for $env() patterns.
// Exported for the editor's variable highlighting.
func EnvPattern() *regexp.Regexp { return envPattern }

// Resolve replaces {{ $env('KEY') }} patterns in the config map using the manager's loaded secrets.
// Only meant for the root config (not routes/workflows).
func (m *Manager) Resolve(config map[string]any) (map[string]any, []error) {
	var errs []error
	result := m.resolveRecursive(config, "", &errs)
	return result.(map[string]any), errs
}

func (m *Manager) resolveRecursive(v any, path string, errs *[]error) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, v := range val {
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}
			result[k] = m.resolveRecursive(v, childPath, errs)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			result[i] = m.resolveRecursive(item, itemPath, errs)
		}
		return result
	case string:
		return m.resolveString(val, path, errs)
	default:
		return v
	}
}

func (m *Manager) resolveString(s string, path string, errs *[]error) string {
	return envPattern.ReplaceAllStringFunc(s, func(match string) string {
		submatch := envPattern.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		varName := submatch[1]
		value, ok := m.Get(varName)
		if !ok {
			*errs = append(*errs, fmt.Errorf("missing environment variable %q at %s", varName, path))
			return match
		}
		return value
	})
}
