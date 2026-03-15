package config

import (
	"fmt"
	"regexp"
)

var varPattern = regexp.MustCompile(`\{\{\s*\$var\(\s*'([^']+)'\s*\)\s*\}\}`)

// VarPattern returns the compiled regex for matching $var() references.
func VarPattern() *regexp.Regexp {
	return varPattern
}

// resolveVars replaces {{ $var('KEY') }} patterns in string values of the config map.
// Looks up values from the provided vars map.
// Returns the resolved config and any errors for unknown variables.
func resolveVars(config map[string]any, vars map[string]string) (map[string]any, []error) {
	var errs []error
	result := resolveVarsRecursive(config, vars, "", &errs)
	return result.(map[string]any), errs
}

func resolveVarsRecursive(v any, vars map[string]string, path string, errs *[]error) any {
	switch val := v.(type) {
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, v := range val {
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}
			result[k] = resolveVarsRecursive(v, vars, childPath, errs)
		}
		return result
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			itemPath := fmt.Sprintf("%s[%d]", path, i)
			result[i] = resolveVarsRecursive(item, vars, itemPath, errs)
		}
		return result
	case string:
		return resolveVarString(val, vars, path, errs)
	default:
		return v
	}
}

func resolveVarString(s string, vars map[string]string, path string, errs *[]error) string {
	return varPattern.ReplaceAllStringFunc(s, func(match string) string {
		submatch := varPattern.FindStringSubmatch(match)
		if len(submatch) < 2 {
			return match
		}
		varName := submatch[1]
		value, ok := vars[varName]
		if !ok {
			*errs = append(*errs, fmt.Errorf("unknown variable %q at %s", varName, path))
			return match
		}
		return value
	})
}

// resolveVarsAll resolves $var() across all config sections (Root, Routes, Workflows, etc.).
// Unlike $env() which only resolves in root, $var() is config-time and resolves everywhere.
func resolveVarsAll(rc *RawConfig) []error {
	if len(rc.Vars) == 0 {
		return nil
	}

	var allErrs []error

	// Resolve in root
	if rc.Root != nil {
		resolved, errs := resolveVars(rc.Root, rc.Vars)
		allErrs = append(allErrs, errs...)
		rc.Root = resolved
	}

	// Resolve in all section maps
	sections := []map[string]map[string]any{
		rc.Routes,
		rc.Workflows,
		rc.Workers,
		rc.Schedules,
		rc.Connections,
		rc.Tests,
		rc.Models,
	}

	for _, section := range sections {
		for filePath, data := range section {
			resolved, errs := resolveVars(data, rc.Vars)
			allErrs = append(allErrs, errs...)
			section[filePath] = resolved
		}
	}

	return allErrs
}
