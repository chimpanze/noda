package main

import "testing"

func TestBuildGroundTruth(t *testing.T) {
	nodes, funcs, err := buildGroundTruth()
	if err != nil {
		t.Fatalf("buildGroundTruth: %v", err)
	}
	if len(nodes) < 80 {
		t.Errorf("expected >= 80 node types, got %d", len(nodes))
	}
	want := []string{"auth.create_user", "lk.token", "db.query", "control.if", "wasm.send"}
	byType := map[string]bool{}
	for _, n := range nodes {
		byType[n.Type] = true
	}
	for _, w := range want {
		if !byType[w] {
			t.Errorf("missing node type %q — plugin list does not mirror cmd/noda/main.go", w)
		}
	}
	if len(funcs) == 0 {
		t.Error("expected registered expression functions")
	}
}
