package config

// securityKeys are config keys whose removal by an overlay should produce a warning.
var securityKeys = []string{"security", "middleware"}

// ValidateMergePreservedKeys warns if security-critical sections present in
// the base were removed (set to null) by the overlay.
func ValidateMergePreservedKeys(base, overlay map[string]any) []string {
	var warnings []string
	for _, key := range securityKeys {
		if _, baseHas := base[key]; !baseHas {
			continue
		}
		if v, overlayHas := overlay[key]; overlayHas && v == nil {
			warnings = append(warnings, key+" section removed by overlay")
		}
	}
	return warnings
}

// MergeOverlay deep-merges the overlay into the base config.
// Scalars: overlay replaces base. Objects: merged recursively.
// Arrays: overlay replaces entirely. Null in overlay: removes the key.
// Returns a new map without mutating base or overlay.
func MergeOverlay(base, overlay map[string]any) map[string]any {
	if overlay == nil {
		return copyMap(base)
	}

	result := copyMap(base)

	for key, overlayVal := range overlay {
		if overlayVal == nil {
			delete(result, key)
			continue
		}

		overlayMap, overlayIsMap := overlayVal.(map[string]any)
		baseVal, baseExists := result[key]

		if overlayIsMap && baseExists {
			if baseMap, baseIsMap := baseVal.(map[string]any); baseIsMap {
				result[key] = MergeOverlay(baseMap, overlayMap)
				continue
			}
		}

		result[key] = deepCopy(overlayVal)
	}

	return result
}

func copyMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = deepCopy(v)
	}
	return result
}

func deepCopy(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return copyMap(val)
	case []any:
		cp := make([]any, len(val))
		for i, item := range val {
			cp[i] = deepCopy(item)
		}
		return cp
	default:
		return v
	}
}
