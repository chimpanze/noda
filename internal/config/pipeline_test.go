package config

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chimpanze/noda/internal/secrets"
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

	sm := secrets.New()
	_ = sm.Load(context.Background())
	rc, errs := ValidateAll(dir, "development", sm)
	assert.Empty(t, errs)
	require.NotNil(t, rc)
	assert.Equal(t, "development", rc.Environment)
	assert.NotNil(t, rc.Root)
	assert.Len(t, rc.Routes, 1)
	assert.Len(t, rc.Workflows, 1)
	assert.Equal(t, 3, rc.FileCount)
}

// TestValidateAll_PopulatesSchemaRegistry proves the $ref registry survives
// the full pipeline and lands on ResolvedConfig.SchemaRegistry keyed by ref
// name ("schemas/User", "schemas/validation/Task") rather than by the file
// path used for ResolvedConfig.Schemas ("<dir>/schemas/User.json"). The
// subdirectory schema is what proves directory qualification survives the
// pipeline, not just flat schemas/ files.
func TestValidateAll_PopulatesSchemaRegistry(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
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
		"schemas/User.json": `{
			"User": {"type": "object", "properties": {"id": {"type": "string"}}},
			"Pagination": {"type": "object", "properties": {"page": {"type": "integer"}}}
		}`,
		"schemas/validation/Task.json": `{
			"Task": {"type": "object", "properties": {"title": {"type": "string"}}}
		}`,
	})

	sm := secrets.New()
	_ = sm.Load(context.Background())
	rc, errs := ValidateAll(dir, "development", sm)
	require.Empty(t, errs)
	require.NotNil(t, rc)

	require.NotEmpty(t, rc.SchemaRegistry)

	expectedRefs := []string{"schemas/User", "schemas/Pagination", "schemas/validation/Task"}
	for _, ref := range expectedRefs {
		assert.Contains(t, rc.SchemaRegistry, ref, "expected ref %q to be registered", ref)
	}
	assert.Len(t, rc.SchemaRegistry, len(expectedRefs),
		"registry should contain exactly the expected refs, no more")

	// Registry keys are ref names, never the file paths ResolvedConfig.Schemas
	// uses — a leaked file path is the exact failure mode #405 pinned down.
	for name := range rc.SchemaRegistry {
		assert.True(t, strings.HasPrefix(name, "schemas/"),
			"registry key %q should be a $ref name, not a file path", name)
		assert.False(t, strings.HasSuffix(name, ".json"),
			"registry key %q looks like a file path, not a $ref name", name)
	}
}

func TestValidateAll_BrokenJSON(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":       `{}`,
		"routes/bad.json": `{invalid json}`,
	})

	sm := secrets.New()
	_ = sm.Load(context.Background())
	_, errs := ValidateAll(dir, "", sm)
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

	sm := secrets.New()
	_ = sm.Load(context.Background())
	_, errs := ValidateAll(dir, "", sm)
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

	sm := secrets.New()
	_ = sm.Load(context.Background())
	_, errs := ValidateAll(dir, "", sm)
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

	sm := secrets.New()
	_ = sm.Load(context.Background())
	_, errs := ValidateAll(dir, "", sm)
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

	sm := secrets.New()
	_ = sm.Load(context.Background())
	rc, errs := ValidateAll(dir, "production", sm)
	assert.Empty(t, errs)
	require.NotNil(t, rc)

	// Overlay should be merged
	db := rc.Root["services"].(map[string]any)["db"].(map[string]any)
	cfg := db["config"].(map[string]any)
	assert.Equal(t, "prod-db.example.com", cfg["host"])
}

func TestValidateAll_MissingNodaJSON(t *testing.T) {
	dir := t.TempDir()

	sm := secrets.New()
	_ = sm.Load(context.Background())
	_, errs := ValidateAll(dir, "", sm)
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Message, "missing required config file")
}

func TestGetValidateInfo(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":             `{}`,
		"noda.development.json": `{}`,
		"routes/a.json":         `{}`,
		"routes/b.json":         `{}`,
		"workflows/c.json":      `{}`,
	})

	info, err := GetValidateInfo(dir, "development")
	require.NoError(t, err)
	assert.Equal(t, "development", info.Environment)
	assert.Equal(t, filepath.Join(dir, "noda.development.json"), info.OverlayFile)
	assert.Equal(t, 2, info.FileCounts["routes"])
	assert.Equal(t, 1, info.FileCounts["workflows"])
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

	sm := secrets.New(&secrets.ProcessEnvProvider{})
	_ = sm.Load(context.Background())
	rc, errs := ValidateAll(dir, "", sm)
	assert.Empty(t, errs)
	require.NotNil(t, rc)

	db := rc.Root["services"].(map[string]any)["db"].(map[string]any)
	cfg := db["config"].(map[string]any)
	assert.Equal(t, "postgres://resolved/test", cfg["url"])
}

// TestValidateAll_InlineConnectionsInRootRejected pins #380: connection
// endpoints live ONLY in connections/*.json. The root schema used to
// advertise an inline "connections" object that the runtime silently
// ignored; validation must now reject it with a message that names the
// connections/ directory convention.
func TestValidateAll_InlineConnectionsInRootRejected(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{
			"connections": {
				"endpoints": {
					"board": {"type": "websocket", "path": "/ws/board"}
				}
			}
		}`,
	})

	sm := secrets.New()
	_ = sm.Load(context.Background())
	_, errs := ValidateAll(dir, "", sm)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, `not read from noda.json`)
	assert.Contains(t, errs[0].Message, "connections/")
	assert.Equal(t, "/connections", errs[0].JSONPath)
}

// The same endpoints in a connections/*.json file remain fully valid and
// satisfy a ws.send node's endpoint reference (#380).
func TestValidateAll_ConnectionsDirIsTheOnlySource(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{}`,
		"connections/realtime.json": `{
			"endpoints": {
				"board": {"type": "websocket", "path": "/ws/board"}
			}
		}`,
		"workflows/notify.json": `{
			"id": "notify",
			"nodes": {
				"send": {
					"type": "ws.send",
					"services": {"connections": "board"},
					"config": {"channel": "b", "data": "hi"}
				}
			},
			"edges": []
		}`,
	})

	sm := secrets.New()
	_ = sm.Load(context.Background())
	rc, errs := ValidateAll(dir, "", sm)
	require.Empty(t, errs)
	require.NotNil(t, rc)
	require.Len(t, rc.Connections, 1)
}
