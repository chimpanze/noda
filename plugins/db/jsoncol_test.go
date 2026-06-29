package db

import (
	"database/sql/driver"
	"testing"
	"time"
)

func TestMarshalJSONComposites(t *testing.T) {
	in := map[string]any{
		"scalar_str": "A",
		"scalar_int": 5,
		"nil_val":    nil,
		"bytes":      []byte{0x01, 0x02},
		"a_slice":    []any{map[string]any{"type": "insert", "position": 0}},
		"a_map":      map[string]any{"k": "v"},
		"when":       time.Unix(0, 0).UTC(),
	}

	out, err := marshalJSONComposites(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Scalars, nil, []byte, and time.Time pass through untouched.
	if out["scalar_str"] != "A" || out["scalar_int"] != 5 {
		t.Errorf("scalars should pass through, got %#v", out)
	}
	if out["nil_val"] != nil {
		t.Errorf("nil should pass through, got %#v", out["nil_val"])
	}
	if _, ok := out["bytes"].([]byte); !ok {
		t.Errorf("[]byte should pass through as []byte, got %T", out["bytes"])
	}
	if _, ok := out["when"].(time.Time); !ok {
		t.Errorf("time.Time should pass through, got %T", out["when"])
	}

	// Slice and map become jsonColumn whose driver value is JSON text.
	sliceCol, ok := out["a_slice"].(jsonColumn)
	if !ok {
		t.Fatalf("slice should become jsonColumn, got %T", out["a_slice"])
	}
	v, err := driver.Valuer(sliceCol).Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if v.(string) != `[{"position":0,"type":"insert"}]` {
		t.Errorf("slice JSON mismatch, got %s", v)
	}
	mapCol, ok := out["a_map"].(jsonColumn)
	if !ok {
		t.Errorf("map should become jsonColumn, got %T", out["a_map"])
	} else {
		mv, err := driver.Valuer(mapCol).Value()
		if err != nil {
			t.Fatalf("a_map Value() error: %v", err)
		}
		if mv.(string) != `{"k":"v"}` {
			t.Errorf("map JSON mismatch, got %s", mv)
		}
	}

	// Input map must not be mutated.
	if _, ok := in["a_slice"].([]any); !ok {
		t.Errorf("input map was mutated")
	}
}
