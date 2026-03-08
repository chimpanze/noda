package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveRefs resolves all $ref references in the config, inlining shared schema definitions.
// Schema files can contain multiple top-level keys: schemas/User.json with {"User": {...}, "Pagination": {...}}
// produces refs "schemas/User" and "schemas/Pagination".
func ResolveRefs(rc *RawConfig) []error {
	// Build schema registry
	registry := buildSchemaRegistry(rc.Schemas)

	var errs []error

	// Resolve refs in all config sections
	sections := []map[string]map[string]any{
		rc.Routes,
		rc.Workflows,
		rc.Workers,
		rc.Schedules,
		rc.Connections,
		rc.Tests,
	}

	for _, section := range sections {
		for filePath, data := range section {
			resolved, refErrs := resolveRefsInValue(data, registry, filePath, nil)
			errs = append(errs, refErrs...)
			section[filePath] = resolved.(map[string]any)
		}
	}

	return errs
}

func buildSchemaRegistry(schemas map[string]map[string]any) map[string]map[string]any {
	registry := make(map[string]map[string]any)

	for filePath, content := range schemas {
		// Extract directory-relative name: schemas/User.json → schemas
		dir := filepath.Dir(filePath)
		baseDirName := filepath.Base(dir) // "schemas"

		for key, val := range content {
			if schema, ok := val.(map[string]any); ok {
				refName := baseDirName + "/" + key
				registry[refName] = schema
			}
		}
	}

	return registry
}

func resolveRefsInValue(v any, registry map[string]map[string]any, filePath string, seen []string) (any, []error) {
	switch val := v.(type) {
	case map[string]any:
		// Check if this is a $ref object
		if ref, ok := val["$ref"]; ok {
			refStr, isStr := ref.(string)
			if isStr {
				return resolveRef(refStr, registry, filePath, seen)
			}
		}

		// Recurse into object
		var errs []error
		result := make(map[string]any, len(val))
		for k, child := range val {
			resolved, childErrs := resolveRefsInValue(child, registry, filePath, seen)
			errs = append(errs, childErrs...)
			result[k] = resolved
		}
		return result, errs

	case []any:
		var errs []error
		result := make([]any, len(val))
		for i, item := range val {
			resolved, itemErrs := resolveRefsInValue(item, registry, filePath, seen)
			errs = append(errs, itemErrs...)
			result[i] = resolved
		}
		return result, errs

	default:
		return v, nil
	}
}

func resolveRef(refName string, registry map[string]map[string]any, filePath string, seen []string) (any, []error) {
	// Check for circular reference
	for _, s := range seen {
		if s == refName {
			cycle := append(seen, refName)
			return nil, []error{
				fmt.Errorf("circular $ref detected: %s (in %s)", strings.Join(cycle, " → "), filePath),
			}
		}
	}

	schema, ok := registry[refName]
	if !ok {
		return nil, []error{
			fmt.Errorf("unresolved $ref %q in %s", refName, filePath),
		}
	}

	// Resolve nested refs within the schema
	newSeen := append(append([]string{}, seen...), refName)
	resolved, errs := resolveRefsInValue(deepCopy(schema), registry, filePath, newSeen)
	return resolved, errs
}
