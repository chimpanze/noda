package testing

import (
	"fmt"
	"reflect"
	"strings"
)

// MatchExpectation compares actual results against expected values.
// Returns (true, "") on match, (false, explanation) on mismatch.
func MatchExpectation(expected TestExpectation, actual TestActualResult) (bool, string) {
	// Match status
	if expected.Status != "" && expected.Status != actual.Status {
		return false, fmt.Sprintf("expected status %q, got %q", expected.Status, actual.Status)
	}

	// Match error node
	if expected.ErrorNode != "" && expected.ErrorNode != actual.ErrorNode {
		return false, fmt.Sprintf("expected error at node %q, got %q", expected.ErrorNode, actual.ErrorNode)
	}

	// Match output fields (dot-path assertions)
	for path, expectedValue := range expected.Output {
		actualValue, err := extractPath(actual.Outputs, path)
		if err != nil {
			return false, fmt.Sprintf("expected %s to be %v, but path not found: %s", path, expectedValue, err)
		}

		if !deepEqual(expectedValue, actualValue) {
			return false, fmt.Sprintf("expected %s to be %v (%T), got %v (%T)", path, expectedValue, expectedValue, actualValue, actualValue)
		}
	}

	return true, ""
}

// extractPath navigates into nested maps using a dot-separated path.
// e.g., "response.body.email" → map["response"]["body"]["email"]
func extractPath(data map[string]any, path string) (any, error) {
	parts := strings.Split(path, ".")
	var current any = data

	for i, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("at %q: expected object, got %T", strings.Join(parts[:i], "."), current)
		}
		val, exists := m[part]
		if !exists {
			return nil, fmt.Errorf("field %q not found at %q", part, strings.Join(parts[:i+1], "."))
		}
		current = val
	}

	return current, nil
}

// deepEqual compares two values, handling JSON number type mismatches.
func deepEqual(expected, actual any) bool {
	// Handle JSON number comparisons (JSON numbers may be float64)
	if eNum, ok := toFloat64(expected); ok {
		if aNum, ok := toFloat64(actual); ok {
			return eNum == aNum
		}
	}

	return reflect.DeepEqual(expected, actual)
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	default:
		return 0, false
	}
}
