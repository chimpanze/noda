package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupValidProject(t *testing.T) string {
	t.Helper()
	return setupTestProject(t, map[string]string{
		"noda.json": `{
			"services": {
				"main-db": {"plugin": "postgres", "config": {"url": "postgres://localhost/test"}}
			}
		}`,
		"routes/tasks.json": `{
			"id": "list-tasks",
			"method": "GET",
			"path": "/api/tasks",
			"trigger": {"workflow": "list-tasks"}
		}`,
		"workflows/list-tasks.json": `{
			"id": "list-tasks",
			"nodes": {
				"query": {"type": "db.query", "services": {"database": "main-db"}}
			},
			"edges": []
		}`,
	})
}

func TestValidateAll_ValidProject(t *testing.T) {
	dir := setupValidProject(t)

	rc, errs := ValidateAll(dir, "development")
	assert.Empty(t, errs)
	require.NotNil(t, rc)
	assert.Equal(t, "development", rc.Environment)
	assert.NotNil(t, rc.Root)
	assert.Len(t, rc.Routes, 1)
	assert.Len(t, rc.Workflows, 1)
	assert.Equal(t, 3, rc.FileCount)
}

func TestValidateAll_BrokenJSON(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":        `{}`,
		"routes/bad.json":  `{invalid json}`,
	})

	_, errs := ValidateAll(dir, "")
	require.NotEmpty(t, errs)
	// Should be JSON error, not schema error
	assert.Contains(t, errs[0].Message, "invalid JSON")
}

func TestValidateAll_SchemaErrors(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{}`,
		"routes/tasks.json": `{
			"id": "test"
		}`,
	})

	_, errs := ValidateAll(dir, "")
	require.NotEmpty(t, errs)
}

func TestValidateAll_CrossRefErrors(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{}`,
		"routes/tasks.json": `{
			"id": "list-tasks",
			"method": "GET",
			"path": "/api/tasks",
			"trigger": {"workflow": "non-existent"}
		}`,
	})

	_, errs := ValidateAll(dir, "")
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "non-existent")
}

func TestValidateAll_MissingEnvVars(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{
			"services": {
				"db": {"plugin": "postgres", "config": {"url": "{{ $env('MISSING_DB_URL') }}"}}
			}
		}`,
	})

	_, errs := ValidateAll(dir, "")
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "MISSING_DB_URL")
}

func TestValidateAll_WithOverlay(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{
			"services": {
				"db": {"plugin": "postgres", "config": {"host": "localhost"}}
			}
		}`,
		"noda.production.json": `{
			"services": {
				"db": {"plugin": "postgres", "config": {"host": "prod-db.example.com"}}
			}
		}`,
	})

	rc, errs := ValidateAll(dir, "production")
	assert.Empty(t, errs)
	require.NotNil(t, rc)

	// Overlay should be merged
	db := rc.Root["services"].(map[string]any)["db"].(map[string]any)
	cfg := db["config"].(map[string]any)
	assert.Equal(t, "prod-db.example.com", cfg["host"])
}

func TestValidateAll_MissingNodaJSON(t *testing.T) {
	dir := t.TempDir()

	_, errs := ValidateAll(dir, "")
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "missing required config file")
}

func TestGetValidateInfo(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":                    `{}`,
		"noda.development.json":        `{}`,
		"routes/a.json":                `{}`,
		"routes/b.json":                `{}`,
		"workflows/c.json":             `{}`,
	})

	info, err := GetValidateInfo(dir, "development")
	require.NoError(t, err)
	assert.Equal(t, "development", info.Environment)
	assert.Equal(t, filepath.Join(dir, "noda.development.json"), info.OverlayFile)
	assert.Equal(t, 2, info.FileCounts["routes"])
	assert.Equal(t, 1, info.FileCounts["workflows"])
}

// Helper to set env var for testing
func withEnvVar(t *testing.T, key, value string) {
	t.Helper()
	t.Setenv(key, value)
}

func TestValidateAll_EnvVarsResolved(t *testing.T) {
	t.Setenv("TEST_DB_URL", "postgres://resolved/test")

	dir := setupTestProject(t, map[string]string{
		"noda.json": `{
			"services": {
				"db": {"plugin": "postgres", "config": {"url": "{{ $env('TEST_DB_URL') }}"}}
			}
		}`,
	})

	rc, errs := ValidateAll(dir, "")
	assert.Empty(t, errs)
	require.NotNil(t, rc)

	db := rc.Root["services"].(map[string]any)["db"].(map[string]any)
	cfg := db["config"].(map[string]any)
	assert.Equal(t, "postgres://resolved/test", cfg["url"])
}

// Ensure os import is used
var _ = os.Getenv
