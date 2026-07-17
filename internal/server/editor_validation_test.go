package server

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/pathutil"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeProjectFiles writes a minimal on-disk Noda project used to exercise
// validateAll/validateFile against the real config.ValidateAll pipeline.
func writeProjectFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
}

// dryRunProjectFiles is a project that is clean at the file-level schema
// validation stage, but whose "fail" node (response.error, empty config)
// violates the response plugin's audited config schema, which requires
// "code" and "message". Only the startup dry-run catches this.
func dryRunProjectFiles() map[string]string {
	return map[string]string{
		"noda.json": `{}`,
		"routes/hello.json": `{
  "id": "hello",
  "method": "GET",
  "path": "/hello",
  "trigger": { "workflow": "hello" }
}`,
		"workflows/hello.json": `{
  "id": "hello",
  "nodes": {
    "fail": { "type": "response.error", "config": {} }
  },
  "edges": []
}`,
	}
}

// setupValidationApp builds an EditorAPI + fiber app wired with a plugin
// registry that has the response plugin registered (needed for
// ValidateStartupDryRun's node-type-prefix and config-schema checks) and
// writes the given files under a fresh temp root.
func setupValidationApp(t *testing.T, files map[string]string) *fiber.App {
	t.Helper()
	app := fiber.New()

	nodeReg := buildTestNodeRegistry()
	pluginReg := registry.NewPluginRegistry()
	require.NoError(t, pluginReg.Register(&response.Plugin{}))
	svcReg := registry.NewServiceRegistry()
	compiler := expr.NewCompilerWithFunctions()

	root, err := pathutil.NewRoot(t.TempDir())
	require.NoError(t, err)
	writeProjectFiles(t, root.String(), files)

	editorAPI := NewEditorAPI(root, "", nil, pluginReg, nodeReg, svcReg, compiler, nil)
	editorAPI.Register(app)
	return app
}

func TestValidateAll_CatchesDryRunNodeConfigError(t *testing.T) {
	app := setupValidationApp(t, dryRunProjectFiles())

	req := httptest.NewRequest("POST", "/_noda/validate/all", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))

	assert.False(t, result["valid"].(bool), "expected valid:false, got %+v", result)

	errs, ok := result["errors"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, errs)

	found := false
	for _, raw := range errs {
		m := raw.(map[string]any)
		if msg, _ := m["message"].(string); strings.Contains(msg, "missing required config field") {
			found = true
		}
	}
	assert.True(t, found, "expected a 'missing required config field' error, got: %+v", errs)
}

func TestValidateFile_CatchesDryRunNodeConfigError(t *testing.T) {
	app := setupValidationApp(t, dryRunProjectFiles())

	body := strings.NewReader(`{"path":"workflows/hello.json"}`)
	req := httptest.NewRequest("POST", "/_noda/validate", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))

	assert.False(t, result["valid"].(bool), "expected valid:false, got %+v", result)

	errs, ok := result["errors"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, errs)

	found := false
	for _, raw := range errs {
		m := raw.(map[string]any)
		if msg, _ := m["message"].(string); strings.Contains(msg, "missing required config field") {
			found = true
		}
	}
	assert.True(t, found, "expected a 'missing required config field' error, got: %+v", errs)
}

func TestValidateAll_ValidProjectStillPasses(t *testing.T) {
	app := setupValidationApp(t, map[string]string{
		"noda.json": `{}`,
		"routes/hello.json": `{
  "id": "hello",
  "method": "GET",
  "path": "/hello",
  "trigger": { "workflow": "hello" }
}`,
		"workflows/hello.json": `{
  "id": "hello",
  "nodes": {
    "fail": { "type": "response.error", "config": { "code": "NOT_FOUND", "message": "nope" } }
  },
  "edges": []
}`,
	})

	req := httptest.NewRequest("POST", "/_noda/validate/all", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.True(t, result["valid"].(bool), "expected valid:true, got %+v", result)
}
