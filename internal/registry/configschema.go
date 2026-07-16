package registry

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

// Node ConfigSchemas are validated with a small purpose-built walker instead
// of a generic JSON Schema library for one reason: an expression string
// ("{{ … }}") must satisfy ANY declared type or enum, because its runtime
// value is unknowable at validation time. CheckSchemaVocabulary keeps schemas
// within the keyword set this walker implements, so the two cannot drift.

// annotationKeywords carry no constraints and are ignored by validation.
var annotationKeywords = map[string]bool{
	"$schema": true, "title": true, "description": true,
	"default": true, "examples": true, "deprecated": true,
}

// constraintKeywords are the schema keywords ValidateNodeConfig implements.
var constraintKeywords = map[string]bool{
	"type": true, "enum": true, "properties": true, "required": true,
	"items": true, "oneOf": true, "additionalProperties": true,
}

// CheckSchemaVocabulary returns an error for every keyword in the schema tree
// that ValidateNodeConfig does not implement. Keys of "properties" maps are
// field names, not keywords.
func CheckSchemaVocabulary(schema map[string]any) []error {
	var errs []error
	checkVocab(schema, "", &errs)
	return errs
}

func checkVocab(schema map[string]any, path string, errs *[]error) {
	for k, v := range schema {
		switch {
		case annotationKeywords[k]:
			// ignore
		case k == "properties":
			if props, ok := v.(map[string]any); ok {
				for name, sub := range props {
					if subMap, ok := sub.(map[string]any); ok {
						checkVocab(subMap, joinPath(path, name), errs)
					}
				}
			}
		case k == "items":
			if subMap, ok := v.(map[string]any); ok {
				checkVocab(subMap, path+"[]", errs)
			}
		case k == "oneOf":
			if branches, ok := v.([]any); ok {
				for i, b := range branches {
					if subMap, ok := b.(map[string]any); ok {
						checkVocab(subMap, fmt.Sprintf("%s(oneOf %d)", path, i), errs)
					}
				}
			}
		case constraintKeywords[k]:
			// implemented, nothing nested to walk
		default:
			*errs = append(*errs, fmt.Errorf("schema keyword %q at %q is not supported by node config validation", k, path))
		}
	}
}

// ValidateNodeConfig checks a node's config payload against its ConfigSchema.
// Expression strings satisfy any type or enum. Unknown keys at the top level
// of the config are errors unless the schema sets "additionalProperties": true;
// nested objects reject unknown keys only with an explicit
// "additionalProperties": false.
func ValidateNodeConfig(schema map[string]any, config map[string]any) []error {
	var errs []error
	validateValue(schema, config, "", true, &errs)
	return errs
}

func validateValue(schema map[string]any, value any, path string, rootStrict bool, errs *[]error) {
	if s, ok := value.(string); ok && strings.Contains(s, "{{") {
		return // expression: runtime type unknowable, satisfies anything
	}

	if branches, ok := schema["oneOf"].([]any); ok {
		for _, b := range branches {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			var branchErrs []error
			validateValue(bm, value, path, rootStrict, &branchErrs)
			if len(branchErrs) == 0 {
				return
			}
		}
		*errs = append(*errs, fmt.Errorf("config%s does not match any allowed variant", atPath(path)))
		return
	}

	if enum, ok := schema["enum"].([]any); ok {
		matched := false
		for _, member := range enum {
			if looseEqual(value, member) {
				matched = true
				break
			}
		}
		if !matched {
			*errs = append(*errs, fmt.Errorf("config field %q: value %v not in allowed values %v", path, value, enum))
			return
		}
	}

	if !typeAllows(schema["type"], value) {
		*errs = append(*errs, fmt.Errorf("config field %q: expected %s, got %s", path, typeNames(schema["type"]), goTypeName(value)))
		return
	}

	if obj, ok := value.(map[string]any); ok {
		props, _ := schema["properties"].(map[string]any)

		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				name, ok := r.(string)
				if !ok {
					continue
				}
				if _, present := obj[name]; !present {
					*errs = append(*errs, fmt.Errorf("missing required config field %q%s", name, atPath(path)))
				}
			}
		}

		strict := false
		switch ap := schema["additionalProperties"].(type) {
		case bool:
			strict = !ap
		default:
			// Unset: strict at the config root (catches typo'd field names),
			// permissive on nested objects.
			strict = rootStrict && path == "" && props != nil
		}

		for name, v := range obj {
			sub, declared := props[name].(map[string]any)
			if !declared {
				if strict {
					*errs = append(*errs, fmt.Errorf("unknown config field %q%s", name, atPath(path)))
				}
				continue
			}
			validateValue(sub, v, joinPath(path, name), false, errs)
		}
	}

	if arr, ok := value.([]any); ok {
		if items, ok := schema["items"].(map[string]any); ok {
			for i, el := range arr {
				validateValue(items, el, fmt.Sprintf("%s[%d]", path, i), false, errs)
			}
		}
	}
}

func typeAllows(declared any, value any) bool {
	switch t := declared.(type) {
	case nil:
		return true
	case string:
		return matchesType(value, t)
	case []any:
		for _, one := range t {
			if s, ok := one.(string); ok && matchesType(value, s) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func matchesType(v any, t string) bool {
	switch t {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "null":
		return v == nil
	case "number":
		_, ok := toFloat(v)
		return ok
	case "integer":
		f, ok := toFloat(v)
		return ok && f == math.Trunc(f)
	default:
		return false
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func looseEqual(a, b any) bool {
	if fa, ok := toFloat(a); ok {
		if fb, ok := toFloat(b); ok {
			return fa == fb
		}
		return false
	}
	return reflect.DeepEqual(a, b)
}

func typeNames(declared any) string {
	switch t := declared.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, one := range t {
			if s, ok := one.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " or ")
	default:
		return "value"
	}
}

func goTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int32, int64, float32, float64:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func joinPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func atPath(path string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf(" (in %q)", path)
}
