package plugin

import "reflect"

// IsTruthy determines if a value is truthy using Go/Noda rules.
// nil, false, 0, "" and empty slices/arrays are falsy. Everything else is truthy.
func IsTruthy(v any) bool {
	if v == nil {
		return false
	}

	switch val := v.(type) {
	case bool:
		return val
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val != ""
	default:
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			return rv.Len() > 0
		}
		return true
	}
}
