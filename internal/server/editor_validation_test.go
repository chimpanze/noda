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

// twoWorkflowProjectFiles has one workflow file that is clean at the
// file-level schema stage but fails the dry-run node-config check
// ("broken"), and a second workflow file that is fully clean ("clean").
// Used to verify validateFile scopes dry-run errors to the requested file
// rather than folding in errors from unrelated workflows (#349).
func twoWorkflowProjectFiles() map[string]string {
	return map[string]string{
		"noda.json": `{}`,
		"routes/hello.json": `{
  "id": "hello",
  "method": "GET",
  "path": "/hello",
  "trigger": { "workflow": "hello" }
}`,
		"routes/other.json": `{
  "id": "other",
  "method": "GET",
  "path": "/other",
  "trigger": { "workflow": "other" }
}`,
		"workflows/hello.json": `{
  "id": "hello",
  "nodes": {
    "fail": { "type": "response.error", "config": {} }
  },
  "edges": []
}`,
		"workflows/other.json": `{
  "id": "other",
  "nodes": {
    "ok": { "type": "response.error", "config": { "code": "NOT_FOUND", "message": "nope" } }
  },
  "edges": []
}`,
	}
}

func TestValidateFile_ScopesDryRunErrorsToRequestedFile(t *testing.T) {
	app := setupValidationApp(t, twoWorkflowProjectFiles())

	// The clean file must be reported valid — dry-run errors from the
	// unrelated broken workflow must not leak in.
	cleanBody := strings.NewReader(`{"path":"workflows/other.json"}`)
	req := httptest.NewRequest("POST", "/_noda/validate", cleanBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.True(t, result["valid"].(bool), "expected clean file to be valid, got %+v", result)

	// The broken file must be reported invalid, with errors attributed to
	// that file's path (not "").
	brokenBody := strings.NewReader(`{"path":"workflows/hello.json"}`)
	req2 := httptest.NewRequest("POST", "/_noda/validate", brokenBody)
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := app.Test(req2)
	require.NoError(t, err)
	assert.Equal(t, 200, resp2.StatusCode)

	respBody2, _ := io.ReadAll(resp2.Body)
	var result2 map[string]any
	require.NoError(t, json.Unmarshal(respBody2, &result2))
	assert.False(t, result2["valid"].(bool), "expected broken file to be invalid, got %+v", result2)

	errs, ok := result2["errors"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, errs)

	for _, raw := range errs {
		m := raw.(map[string]any)
		file, _ := m["file"].(string)
		assert.NotEmpty(t, file, "expected dry-run error to carry a non-empty file attribution, got: %+v", m)
		assert.True(t, strings.HasSuffix(file, filepath.Join("workflows", "hello.json")),
			"expected error file to be workflows/hello.json, got %q", file)
	}
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
