package config

import (
	"fmt"
	"os"
	"regexp"
)

var envPattern = regexp.MustCompile(`\{\{\s*\$env\(\s*'([^']+)'\s*\)\s*\}\}`)

// resolveEnvVars replaces {{ $env('VAR') }} patterns in string values of the config map.
// Only resolves in the root config (not in routes, workflows, etc.).
// Returns the resolved config and any errors for missing environment variables.
func resolveEnvVars(config map[string]any) (map[string]any, []error) {
	var errs []error
	result := resolveEnvVarsRecursive(config, "", &errs)
	return result.(map[string]any), errs
}

func resolveEnvVarsRecursive(v any, path string, errs *[]error) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, v := range val {
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}
			result[k] = resolveEnvVarsRecursive(v, childPath, errs)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			result[i] = resolveEnvVarsRecursive(item, itemPath, errs)
		}
		return result
	case string:
		return resolveEnvString(val, path, errs)
	default:
		return v
	}
}

func resolveEnvString(s string, path string, errs *[]error) string {
	return envPattern.ReplaceAllStringFunc(s, func(match string) string {
		submatch := envPattern.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		varName := submatch[1]
		value, ok := os.LookupEnv(varName)
		if !ok {
			*errs = append(*errs, fmt.Errorf("missing environment variable %q at %s", varName, path))
			return match
		}
		return value
	})
}

// resolveEnvVarsSelective resolves $env() in the root config only.
// Since wasm_runtimes is nested inside root, their config sections are resolved too.
// Other config sections (routes, workflows, etc.) are left unchanged — their {{ }}
// expressions are runtime expressions, not config-time resolution.
func resolveEnvVarsSelective(rc *RawConfig) []error {
	if rc.Root == nil {
		return nil
	}

	var allErrs []error

	resolved, errs := resolveEnvVars(rc.Root)
	allErrs = append(allErrs, errs...)
	rc.Root = resolved

	return allErrs
}
