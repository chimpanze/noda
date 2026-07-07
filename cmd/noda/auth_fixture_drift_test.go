package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAuthFixtureMatchesTemplates pins testdata/auth to the auth_templates:
// the committed fixture must be byte-identical to a fresh scaffold rendered
// with the fixture's own service names (main-db + email). If this fails you
// changed the templates without regenerating the fixture — regenerate:
//
//  1. remove the "auth" service entry from testdata/auth/noda.json
//     (runAuthInit refuses to run while it exists; it re-adds it)
//  2. rm testdata/auth/workflows/auth.*.json testdata/auth/routes/auth.*.json \
//     testdata/auth/tests/test-auth-*.json testdata/auth/migrations/*_auth_tables.*.sql
//  3. go run ./cmd/noda auth init --dir testdata/auth
//  4. re-run go test -tags=integration ./plugins/auth/ and adapt the e2e
//     if the flow shapes changed
//
// (#291 — the fixture rotted silently for three tranches before this guard.)
func TestAuthFixtureMatchesTemplates(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "auth")

	// Scaffold with the fixture's service names so [[.EmailService]] renders
	// to "email" (scaffoldAuthProject's writeMinimalProject uses "mailer").
	dir := t.TempDir()
	services := map[string]any{
		"main-db": map[string]any{"plugin": "db", "config": map[string]any{"driver": "sqlite", "path": "data/app.db"}},
		"email":   map[string]any{"plugin": "email", "config": map[string]any{"host": "localhost", "port": 1025}},
	}
	b, err := json.MarshalIndent(map[string]any{"services": services}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "noda.json"), b, 0644))
	require.NoError(t, runAuthInit(dir))

	// Byte-compare the rendered trees (file set + contents). The fixture's
	// workflows/routes/tests directories contain only auth-owned files.
	for _, sub := range []string{"workflows", "routes", "tests"} {
		requireDirsEqual(t, filepath.Join(dir, sub), filepath.Join(fixture, sub))
	}

	// Migrations: content-equal, generation-timestamp prefix ignored.
	requireMigrationsEqual(t, filepath.Join(dir, "migrations"), filepath.Join(fixture, "migrations"))
}

func requireDirsEqual(t *testing.T, got, want string) {
	t.Helper()
	gotNames := dirFileNames(t, got)
	wantNames := dirFileNames(t, want)
	require.Equal(t, wantNames, gotNames,
		"testdata/auth/%s file set lags the auth templates — regenerate the fixture (see TestAuthFixtureMatchesTemplates doc comment)", filepath.Base(want))
	for _, name := range wantNames {
		gotB, err := os.ReadFile(filepath.Join(got, name))
		require.NoError(t, err)
		wantB, err := os.ReadFile(filepath.Join(want, name))
		require.NoError(t, err)
		require.Equal(t, string(wantB), string(gotB),
			"testdata/auth/%s/%s lags the auth templates — regenerate the fixture (see TestAuthFixtureMatchesTemplates doc comment)", filepath.Base(want), name)
	}
}

var migrationTS = regexp.MustCompile(`^\d{14}_`)

func requireMigrationsEqual(t *testing.T, got, want string) {
	t.Helper()
	norm := func(dir string) map[string]string {
		out := map[string]string{}
		for _, name := range dirFileNames(t, dir) {
			b, err := os.ReadFile(filepath.Join(dir, name))
			require.NoError(t, err)
			out[migrationTS.ReplaceAllString(name, "TS_")] = string(b)
		}
		return out
	}
	require.Equal(t, norm(want), norm(got),
		"testdata/auth/migrations lag the auth templates (timestamps normalized) — regenerate the fixture")
}

func dirFileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}
