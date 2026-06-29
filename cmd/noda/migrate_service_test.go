package main

import (
	"strings"
	"testing"
)

func pg(url string) map[string]any {
	return map[string]any{"plugin": "postgres", "config": map[string]any{"url": url}}
}

func TestResolveMigrateService(t *testing.T) {
	single := map[string]any{
		"main-db":   pg("postgres://x"),
		"app-cache": map[string]any{"plugin": "cache"},
	}
	multi := map[string]any{
		"main-db": pg("postgres://x"),
		"side-db": pg("postgres://y"),
	}

	// Not explicit + exactly one Postgres service -> auto-detect it.
	if got, err := resolveMigrateService(single, "db", false); err != nil || got != "main-db" {
		t.Errorf("auto-detect: got %q err=%v, want main-db", got, err)
	}

	// Not explicit + multiple Postgres services -> error listing them.
	_, err := resolveMigrateService(multi, "db", false)
	if err == nil || !strings.Contains(err.Error(), "main-db") || !strings.Contains(err.Error(), "side-db") {
		t.Errorf("multi: expected error listing services, got %v", err)
	}

	// Explicit + present -> use it.
	if got, err := resolveMigrateService(single, "main-db", true); err != nil || got != "main-db" {
		t.Errorf("explicit present: got %q err=%v", got, err)
	}

	// Explicit + missing -> error lists available names.
	_, err = resolveMigrateService(single, "db", true)
	if err == nil || !strings.Contains(err.Error(), "main-db") || !strings.Contains(err.Error(), "app-cache") {
		t.Errorf("explicit missing: expected error listing available services, got %v", err)
	}

	// Not explicit + zero postgres services -> fallthrough to name lookup,
	// which also fails; error must mention the only available service name.
	noPostgres := map[string]any{
		"app-cache": map[string]any{"plugin": "cache"},
	}
	_, err = resolveMigrateService(noPostgres, "db", false)
	if err == nil || !strings.Contains(err.Error(), "app-cache") {
		t.Errorf("no-postgres: expected error mentioning available service names, got %v", err)
	}
}
