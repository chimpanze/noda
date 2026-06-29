package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"reflect"
)

// jsonColumn carries pre-marshaled JSON bytes to the database. Implementing
// driver.Valuer makes GORM/pgx send it to a jsonb column as a single JSON
// text parameter, instead of GORM expanding a Go slice/map into a SQL record
// tuple (which Postgres rejects with SQLSTATE 42804, or silently corrupts a
// single-element slice into a bare object).
type jsonColumn []byte

func (c jsonColumn) Value() (driver.Value, error) {
	return string(c), nil
}

// marshalJSONComposites returns a shallow copy of data where any composite
// value (map, slice, or array — except []byte and already JSON-typed values)
// is JSON-encoded into a jsonColumn. Scalars, nil, []byte, time.Time, and
// values that already implement driver.Valuer pass through unchanged.
func marshalJSONComposites(data map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(data))
	for k, v := range data {
		jv, err := jsonifyIfComposite(v)
		if err != nil {
			return nil, fmt.Errorf("column %q: %w", k, err)
		}
		out[k] = jv
	}
	return out, nil
}

// isJSONComposite reports whether v is a map/slice/array that should be
// JSON-encoded for a JSONB column (excluding []byte and already-serializing types).
func isJSONComposite(v any) bool {
	if v == nil {
		return false
	}
	switch v.(type) {
	case []byte, json.RawMessage, jsonColumn, driver.Valuer:
		return false
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		return true
	}
	return false
}

func jsonifyIfComposite(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch v.(type) {
	case []byte, json.RawMessage, jsonColumn, driver.Valuer:
		// Raw bytes (bytea) and values that already know how to serialize
		// themselves are left alone.
		return v, nil
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return jsonColumn(b), nil
	default:
		return v, nil
	}
}
