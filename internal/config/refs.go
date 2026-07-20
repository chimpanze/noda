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
	registry, errs := BuildSchemaRegistry(rc.Schemas)
	rc.SchemaRegistry = registry

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

// BuildSchemaRegistry maps every schema $ref name to its definition, and
// reports files that cannot be classified and ref names claimed more than
// once. Ref names are "<reldir>/<key>", e.g. "schemas/User" or
// "schemas/validation/Task" — the exact strings configs write in "$ref".
//
// The returned registry is only meaningful when the returned error slice is
// empty; if there are any errors, the registry is nil rather than a
// partially- or arbitrarily-populated map.
func BuildSchemaRegistry(schemas map[string]map[string]any) (map[string]map[string]any, []ValidationError) {
	registry := make(map[string]map[string]any)
	sources := make(map[string][]schemaSource)
	var ambiguous []ValidationError

	for filePath, content := range schemas {
		relDir := extractSchemasRelPath(filePath)

		switch classifySchemaFile(content) {
		case schemaFileAmbiguous:
			ambiguous = append(ambiguous, ValidationError{
				FilePath: filePath,
				Message: `cannot tell whether this file is a JSON Schema document or a map of schema definitions — ` +
					`it has a top-level "properties" or "items" but no "type"/"$schema"/"$ref"/"enum"/"oneOf"/"anyOf"/"allOf". ` +
					`Add "type" to make it a schema document, or rename the definition to make it a named-definitions file`,
			})

		case schemaFileBare:
			// A file that is itself a JSON Schema document registers whole
			// under schemas/<filename-without-extension> (#373).
			base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
			refName := relDir + "/" + base
			registry[refName] = content
			sources[refName] = append(sources[refName], schemaSource{FilePath: filePath})

		default: // schemaFileKeyed — each top-level key is a named schema definition.
			for key, val := range content {
				if schema, ok := val.(map[string]any); ok {
					refName := relDir + "/" + key
					registry[refName] = schema
					sources[refName] = append(sources[refName], schemaSource{FilePath: filePath, Key: key})
				}
			}
		}
	}

	// Sorted by FilePath so the message order does not depend on map iteration.
	sort.Slice(ambiguous, func(i, j int) bool { return ambiguous[i].FilePath < ambiguous[j].FilePath })

	errs := append(ambiguous, collisionErrors(sources)...)
	if len(errs) > 0 {
		return nil, errs
	}
	return registry, errs
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

// schemaFileKind is how a schemas/ file's top level should be read.
type schemaFileKind int

const (
	// schemaFileKeyed: each top-level key is a named schema definition.
	schemaFileKeyed schemaFileKind = iota
	// schemaFileBare: the file is itself a JSON Schema document (#373).
	schemaFileBare
	// schemaFileAmbiguous: the two readings cannot be told apart.
	schemaFileAmbiguous
)

// bareSchemaKeywords maps a JSON Schema keyword to a predicate reporting
// whether the value has the shape that keyword actually takes in a schema
// document. Presence alone is not enough: a top-level "type" whose value is an
// object is a schema *named* "type", not the type keyword, and reading it as a
// bare schema silently discards every definition in the file (#405).
var bareSchemaKeywords = map[string]func(any) bool{
	"$schema": isJSONString,
	"$ref":    isJSONString,
	"type":    func(v any) bool { return isJSONString(v) || isJSONArray(v) },
	"enum":    isJSONArray,
	"oneOf":   isJSONArray,
	"anyOf":   isJSONArray,
	"allOf":   isJSONArray,
}

// ambiguousSchemaKeywords take object values both as schema keywords and as
// definition names, so shape cannot separate the two readings.
var ambiguousSchemaKeywords = []string{"properties", "items"}

func isJSONString(v any) bool {
	_, ok := v.(string)
	return ok
}

func isJSONArray(v any) bool {
	_, ok := v.([]any)
	return ok
}

// classifySchemaFile decides how to read a schemas/ file's top level.
//
// A bareSchemaKeywords key present with the *wrong* shape (e.g. "type"
// holding an object) is not neutral: it proves that key cannot be the
// keyword, so it must be a definition name, so the file cannot be a bare
// schema at all — that proof holds regardless of what else is in the file
// (e.g. an "items" that would otherwise be ambiguous on its own). Iteration
// order over bareSchemaKeywords does not matter for either the immediate
// "found a correctly-shaped keyword" return or the accumulated
// wrong-shape proof: both are order-independent boolean reductions.
func classifySchemaFile(content map[string]any) schemaFileKind {
	sawWrongShapedKeyword := false
	for kw, hasKeywordShape := range bareSchemaKeywords {
		v, ok := content[kw]
		if !ok {
			continue
		}
		if hasKeywordShape(v) {
			return schemaFileBare
		}
		sawWrongShapedKeyword = true
	}
	if sawWrongShapedKeyword {
		return schemaFileKeyed
	}

	for _, kw := range ambiguousSchemaKeywords {
		if _, ok := content[kw]; ok {
			return schemaFileAmbiguous
		}
	}

	return schemaFileKeyed
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
