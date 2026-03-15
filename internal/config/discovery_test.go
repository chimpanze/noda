package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for path, content := range files {
		fullPath := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o644))
	}
	return dir
}

func TestDiscover_FullProject(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":                   `{}`,
		"noda.development.json":       `{}`,
		"schemas/User.json":           `{}`,
		"schemas/Task.json":           `{}`,
		"routes/tasks.json":           `{}`,
		"routes/v1/users.json":        `{}`,
		"workflows/create-task.json":  `{}`,
		"workers/notifications.json":  `{}`,
		"schedules/cleanup.json":      `{}`,
		"connections/realtime.json":   `{}`,
		"tests/test-create-task.json": `{}`,
	})

	d, err := Discover(dir, "development")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "noda.json"), d.Root)
	assert.Equal(t, filepath.Join(dir, "noda.development.json"), d.Overlay)
	assert.Len(t, d.Schemas, 2)
	assert.Len(t, d.Routes, 2)
	assert.Len(t, d.Workflows, 1)
	assert.Len(t, d.Workers, 1)
	assert.Len(t, d.Schedules, 1)
	assert.Len(t, d.Connections, 1)
	assert.Len(t, d.Tests, 1)
}

func TestDiscover_MinimalProject(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{}`,
	})

	d, err := Discover(dir, "development")
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(dir, "noda.json"), d.Root)
	assert.Empty(t, d.Overlay)
	assert.Empty(t, d.Schemas)
	assert.Empty(t, d.Routes)
	assert.Empty(t, d.Workflows)
}

func TestDiscover_NestedSubdirectories(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":                  `{}`,
		"routes/v1/users.json":       `{}`,
		"routes/v1/admin/roles.json": `{}`,
		"routes/health.json":         `{}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)
	assert.Len(t, d.Routes, 3)
}

func TestDiscover_NonJSONFilesIgnored(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":         `{}`,
		"routes/tasks.json": `{}`,
		"routes/README.md":  `# Routes`,
		"routes/.gitkeep":   ``,
		"routes/backup.bak": `{}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)
	assert.Len(t, d.Routes, 1)
}

func TestDiscover_MissingNodaJSON(t *testing.T) {
	dir := t.TempDir()

	_, err := Discover(dir, "development")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required config file")
}

func TestDiscover_OverlayMatchedByEnv(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":             `{}`,
		"noda.development.json": `{}`,
		"noda.production.json":  `{}`,
	})

	d, err := Discover(dir, "production")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "noda.production.json"), d.Overlay)
}

func TestDiscover_WithVarsFile(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{}`,
		"vars.json": `{"key": "value"}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "vars.json"), d.Vars)
}

func TestDiscover_WithModels(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":        `{}`,
		"models/user.json": `{"table": "users", "columns": {}}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)
	assert.Len(t, d.Models, 1)
}

func TestDiscover_OverlayNotFound(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{}`,
	})

	d, err := Discover(dir, "staging")
	require.NoError(t, err)
	assert.Empty(t, d.Overlay)
}
