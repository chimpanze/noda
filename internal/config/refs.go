package config

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// ResolveRefs resolves all $ref references in the config, inlining shared schema definitions.
// Schema files can contain multiple top-level keys: schemas/User.json with {"User": {...}, "Pagination": {...}}
// produces refs "schemas/User" and "schemas/Pagination".
func ResolveRefs(rc *RawConfig) []ValidationError {
	// Build schema registry
	registry, errs := buildSchemaRegistry(rc.Schemas)

	// Resolve refs in all config sections
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
			resolved, refErrs := resolveRefsInValue(data, registry, filePath, nil)
			errs = append(errs, refErrs...)
			section[filePath] = resolved.(map[string]any)
		}
	}

	return errs
}

// schemaSource records which file contributed a ref name, and how. An empty
// Key means the file is itself a JSON Schema document and registered whole.
type schemaSource struct {
	FilePath string
	Key      string
}

func (s schemaSource) describe() string {
	if s.Key == "" {
		return s.FilePath + " (whole file)"
	}
	return fmt.Sprintf("%s (key %q)", s.FilePath, s.Key)
}

func buildSchemaRegistry(schemas map[string]map[string]any) (map[string]map[string]any, []ValidationError) {
	registry := make(map[string]map[string]any)
	sources := make(map[string][]schemaSource)

	for filePath, content := range schemas {
		relDir := extractSchemasRelPath(filePath)

		// A file that is itself a JSON Schema document registers whole
		// under schemas/<filename-without-extension> (#373); otherwise
		// each top-level key is a named schema definition.
		if isBareSchema(content) {
			base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
			refName := relDir + "/" + base
			registry[refName] = content
			sources[refName] = append(sources[refName], schemaSource{FilePath: filePath})
			continue
		}

		for key, val := range content {
			if schema, ok := val.(map[string]any); ok {
				refName := relDir + "/" + key
				registry[refName] = schema
				sources[refName] = append(sources[refName], schemaSource{FilePath: filePath, Key: key})
			}
		}
	}

	return registry, collisionErrors(sources)
}

// collisionErrors reports every ref name claimed by more than one source (#405).
// Without this the registry silently keeps whichever definition Go's randomized
// map iteration happened to write last, so the same config can validate against
// a different schema on the next boot.
//
// Everything here is sorted: the input is derived from a map, and a
// nondeterministic message would defeat the purpose.
func collisionErrors(sources map[string][]schemaSource) []ValidationError {
	names := make([]string, 0, len(sources))
	for name, srcs := range sources {
		if len(srcs) > 1 {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var errs []ValidationError
	for _, name := range names {
		srcs := sources[name]
		sort.Slice(srcs, func(i, j int) bool {
			if srcs[i].FilePath != srcs[j].FilePath {
				return srcs[i].FilePath < srcs[j].FilePath
			}
			return srcs[i].Key < srcs[j].Key
		})

		described := make([]string, len(srcs))
		for i, s := range srcs {
			described[i] = s.describe()
		}

		jsonPath := ""
		if srcs[0].Key != "" {
			jsonPath = "/" + srcs[0].Key
		}

		errs = append(errs, ValidationError{
			FilePath: srcs[0].FilePath,
			JSONPath: jsonPath,
			Message: fmt.Sprintf(
				"duplicate schema ref %q defined %d times: %s — ref names must be unique; rename one definition, or move one file into a schemas/ subdirectory (the directory is part of the ref name)",
				name, len(srcs), strings.Join(described, ", ")),
		})
	}

	return errs
}

// isBareSchema reports whether a schema file's content is itself a JSON
// Schema document (identified by top-level schema keywords) rather than a
// map of name → schema definitions.
func isBareSchema(content map[string]any) bool {
	for _, kw := range []string{"$schema", "type", "properties", "items", "enum", "oneOf", "anyOf", "allOf", "$ref"} {
		if _, ok := content[kw]; ok {
			return true
		}
	}
	return false
}

// extractSchemasRelPath returns the directory path from the "schemas" segment onward,
// e.g. "/project/schemas/validation/Task.json" → "schemas/validation".
// For the flat case "/project/schemas/Task.json" → "schemas".
func extractSchemasRelPath(filePath string) string {
	parts := strings.Split(filepath.ToSlash(filePath), "/")
	for i, p := range parts {
		if p == "schemas" {
			return strings.Join(parts[i:len(parts)-1], "/")
		}
	}
	return "schemas"
}

func resolveRefsInValue(v any, registry map[string]map[string]any, filePath string, seen []string) (any, []ValidationError) {
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
		var errs []ValidationError
		result := make(map[string]any, len(val))
		for k, child := range val {
			resolved, childErrs := resolveRefsInValue(child, registry, filePath, seen)
			errs = append(errs, childErrs...)
			result[k] = resolved
		}
		return result, errs

	case []any:
		var errs []ValidationError
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

func resolveRef(refName string, registry map[string]map[string]any, filePath string, seen []string) (any, []ValidationError) {
	// Check for circular reference
	for _, s := range seen {
		if s == refName {
			cycle := make([]string, len(seen)+1)
			copy(cycle, seen)
			cycle[len(seen)] = refName
			return nil, []ValidationError{{
				FilePath: filePath,
				Message:  fmt.Sprintf("circular $ref detected: %s", strings.Join(cycle, " → ")),
			}}
		}
	}

	schema, ok := registry[refName]
	if !ok {
		known := make([]string, 0, len(registry))
		for k := range registry {
			known = append(known, k)
		}
		sort.Strings(known)
		knownList := "none"
		if len(known) > 0 {
			knownList = strings.Join(known, ", ")
		}
		return nil, []ValidationError{{
			FilePath: filePath,
			Message: fmt.Sprintf("unresolved $ref %q (known refs: %s — a schemas/ file maps each top-level key to a schema, registered as schemas/<Key>; a file that is itself a JSON Schema registers as schemas/<filename without .json>)",
				refName, knownList),
		}}
	}

	// Resolve nested refs within the schema
	newSeen := append(append([]string{}, seen...), refName)
	resolved, errs := resolveRefsInValue(deepCopy(schema), registry, filePath, newSeen)
	return resolved, errs
}
