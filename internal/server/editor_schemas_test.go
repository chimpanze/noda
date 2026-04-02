package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/devmode"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/pathutil"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSchemasApp creates a minimal editor app for testing the schemas endpoint.
func setupSchemasApp(t *testing.T) *fiber.App {
	t.Helper()
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()
	compiler := expr.NewCompilerWithFunctions()

	tmpDir := t.TempDir()
	root, err := pathutil.NewRoot(tmpDir)
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Schemas:   map[string]map[string]any{},
		Routes:    map[string]map[string]any{},
		Workflows: map[string]map[string]any{},
	}

	reloader := devmode.NewReloader(tmpDir, "", rc, nil, slog.Default())
	editorAPI := NewEditorAPI(root, "", reloader, pluginReg, nodeReg, svcReg, compiler, nil)
	editorAPI.rc = rc
	editorAPI.Register(app)
	return app
}

func TestListOutputSchemas_ReturnsSchemas(t *testing.T) {
	app := setupSchemasApp(t)

	req := httptest.NewRequest("GET", "/_noda/schemas/output", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))

	schemas, ok := result["schemas"].(map[string]any)
	require.True(t, ok, "response should have a 'schemas' map")

	// util.uuid and util.timestamp implement NodeOutputSchemaProvider — they must appear
	assert.Contains(t, schemas, "util.uuid", "util.uuid should be in output schemas")
	assert.Contains(t, schemas, "util.timestamp", "util.timestamp should be in output schemas")

	// cache.get, cache.set, cache.del implement NodeOutputSchemaProvider
	assert.Contains(t, schemas, "cache.get", "cache.get should be in output schemas")
	assert.Contains(t, schemas, "cache.set", "cache.set should be in output schemas")
	assert.Contains(t, schemas, "cache.del", "cache.del should be in output schemas")
}

func TestListOutputSchemas_ExcludesNoProviderNodes(t *testing.T) {
	app := setupSchemasApp(t)

	req := httptest.NewRequest("GET", "/_noda/schemas/output", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))

	schemas, ok := result["schemas"].(map[string]any)
	require.True(t, ok)

	// control.if, transform.set, response.json do NOT implement NodeOutputSchemaProvider
	assert.NotContains(t, schemas, "control.if", "control.if should not be in output schemas")
	assert.NotContains(t, schemas, "transform.set", "transform.set should not be in output schemas")
	assert.NotContains(t, schemas, "response.json", "response.json should not be in output schemas")
}

func TestListOutputSchemas_SchemaValues(t *testing.T) {
	app := setupSchemasApp(t)

	req := httptest.NewRequest("GET", "/_noda/schemas/output", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))

	schemas, ok := result["schemas"].(map[string]any)
	require.True(t, ok)

	// Each present schema should be a non-nil object
	for nodeType, schema := range schemas {
		assert.NotNil(t, schema, "schema for %s should not be nil", nodeType)
		_, isMap := schema.(map[string]any)
		assert.True(t, isMap, "schema for %s should be a map", nodeType)
	}
}
