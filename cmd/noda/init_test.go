package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScaffoldProject_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "app")
	require.NoError(t, os.MkdirAll(proj, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "noda.json"), []byte("{}"), 0644))

	err := scaffoldProject(proj, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "noda.json")
	// existing file untouched, no partial scaffold
	b, _ := os.ReadFile(filepath.Join(proj, "noda.json"))
	require.Equal(t, "{}", string(b))
	// no partial scaffold: other template output files must not have been written
	_, err = os.Stat(filepath.Join(proj, "docker-compose.yml"))
	require.True(t, os.IsNotExist(err), "docker-compose.yml should not have been written")
}

func TestScaffoldProject_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "app")
	require.NoError(t, os.MkdirAll(proj, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "noda.json"), []byte("{}"), 0644))
	require.NoError(t, scaffoldProject(proj, true))
	b, _ := os.ReadFile(filepath.Join(proj, "noda.json"))
	require.NotEqual(t, "{}", string(b)) // overwritten with the template
}

func TestScaffoldProject_GeneratesEnvWithUniqueJWTSecret(t *testing.T) {
	dir := t.TempDir()
	projA := filepath.Join(dir, "app-a")
	projB := filepath.Join(dir, "app-b")
	require.NoError(t, scaffoldProject(projA, false))
	require.NoError(t, scaffoldProject(projB, false))

	exampleB, err := os.ReadFile(filepath.Join(projA, ".env.example"))
	require.NoError(t, err)
	require.Contains(t, string(exampleB), "at least 32 bytes")
	require.Contains(t, string(exampleB), "replace-with-at-least-32-bytes")

	envA, err := os.ReadFile(filepath.Join(projA, ".env"))
	require.NoError(t, err)
	envB, err := os.ReadFile(filepath.Join(projB, ".env"))
	require.NoError(t, err)

	secretA := extractJWTSecret(t, string(envA))
	secretB := extractJWTSecret(t, string(envB))
	require.Len(t, secretA, 64)
	require.Len(t, secretB, 64)
	require.NotEqual(t, secretA, secretB, "each init must generate a unique secret")
	require.NotContains(t, string(envA), "replace-with-at-least-32-bytes")

	// Other lines from .env.example must survive untouched in .env.
	require.Contains(t, string(envA), "DATABASE_URL=postgres://noda:noda@localhost:5432/noda?sslmode=disable")
}

func extractJWTSecret(t *testing.T, envContent string) string {
	t.Helper()
	for _, line := range strings.Split(envContent, "\n") {
		if strings.HasPrefix(line, "JWT_SECRET=") {
			return strings.TrimPrefix(line, "JWT_SECRET=")
		}
	}
	t.Fatal("JWT_SECRET= line not found in .env")
	return ""
}

func TestScaffoldProject_ConflictsSorted(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "app")
	require.NoError(t, os.MkdirAll(proj, 0755))

	// Create multiple conflicting files and directories in non-lexicographic order to verify sorting.
	// These names correspond to actual template files/dirs that will be walked in arbitrary order.
	require.NoError(t, os.MkdirAll(filepath.Join(proj, "workflows"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(proj, "routes"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "workflows/hello.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "noda.json"), []byte("{}"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "routes/api.json"), []byte("{}"), 0644))

	err := scaffoldProject(proj, false)
	require.Error(t, err)

	// Conflicts should be sorted lexicographically in the error message
	errMsg := err.Error()
	require.Contains(t, errMsg, "noda.json")
	require.Contains(t, errMsg, "routes")
	require.Contains(t, errMsg, "workflows")

	// Verify that conflicts are in sorted order: noda.json < routes < workflows
	nodaIdx := strings.Index(errMsg, "noda.json")
	routesIdx := strings.Index(errMsg, "routes")
	workflowsIdx := strings.Index(errMsg, "workflows")
	require.True(t, nodaIdx >= 0 && routesIdx >= 0 && workflowsIdx >= 0, "all conflict files should be in error message")
	require.True(t, nodaIdx < routesIdx, "noda.json should appear before routes in sorted order")
	require.True(t, routesIdx < workflowsIdx, "routes should appear before workflows in sorted order")
}
