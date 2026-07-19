package editor

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
	storageplugin "github.com/chimpanze/noda/plugins/storage"
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

// setupValidationApp builds an API + fiber app wired with a plugin
// registry that has the response plugin registered (needed for
// ValidateStartupDryRun's node-type-prefix and config-schema checks) and
// writes the given files under a fresh temp root.
func setupValidationApp(t *testing.T, files map[string]string) *fiber.App {
	t.Helper()
	app := fiber.New()

	nodeReg := buildTestNodeRegistry()
	pluginReg := registry.NewPluginRegistry()
	require.NoError(t, pluginReg.Register(&response.Plugin{}))
	require.NoError(t, pluginReg.Register(&storageplugin.Plugin{}))
	svcReg := registry.NewServiceRegistry()
	compiler := expr.NewCompilerWithFunctions()

	root, err := pathutil.NewRoot(t.TempDir())
	require.NoError(t, err)
	writeProjectFiles(t, root.String(), files)

	editorAPI := NewAPI(root, "", nil, pluginReg, nodeReg, svcReg, compiler, nil)
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

// badServiceConfigProjectFiles has a fully clean workflow file, but noda.json
// declares a storage service with an invalid "backend" (not in the plugin's
// declared enum ["local", "memory"]) — a #376 service-schema violation.
func badServiceConfigProjectFiles() map[string]string {
	return map[string]string{
		"noda.json": `{
  "services": {
    "badstorage": {
      "plugin": "storage",
      "config": { "backend": "s3" }
    }
  }
}`,
		"routes/hello.json": `{
  "id": "hello",
  "method": "GET",
  "path": "/hello",
  "trigger": { "workflow": "hello" }
}`,
		"workflows/hello.json": `{
  "id": "hello",
  "nodes": {
    "ok": { "type": "response.error", "config": { "code": "NOT_FOUND", "message": "nope" } }
  },
  "edges": []
}`,
	}
}

// TestValidateFile_ServiceSchemaErrorDoesNotLeakOntoUnrelatedFile guards
// against I2 (final review): the service-schema dry-run check always reads
// rc.Root (services are project-wide, not per-file), so without attribution
// filtering, saving a clean workflow file while noda.json has a bad service
// config would incorrectly report that workflow file as invalid.
func TestValidateFile_ServiceSchemaErrorDoesNotLeakOntoUnrelatedFile(t *testing.T) {
	app := setupValidationApp(t, badServiceConfigProjectFiles())

	body := strings.NewReader(`{"path":"workflows/hello.json"}`)
	req := httptest.NewRequest("POST", "/_noda/validate", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.True(t, result["valid"].(bool), "expected the clean workflow file to be valid despite noda.json's bad service config, got %+v", result)
}

// TestValidateFile_ServiceSchemaErrorAttributedToRootConfig is I2's
// complementary case: validating noda.json itself must surface the
// service-schema error, since that IS the file where the bad config lives.
func TestValidateFile_ServiceSchemaErrorAttributedToRootConfig(t *testing.T) {
	app := setupValidationApp(t, badServiceConfigProjectFiles())

	body := strings.NewReader(`{"path":"noda.json"}`)
	req := httptest.NewRequest("POST", "/_noda/validate", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.False(t, result["valid"].(bool), "expected noda.json to be reported invalid, got %+v", result)

	errs, ok := result["errors"].([]any)
	require.True(t, ok)
	require.NotEmpty(t, errs)

	found := false
	for _, raw := range errs {
		m := raw.(map[string]any)
		if msg, _ := m["message"].(string); strings.Contains(msg, "badstorage") {
			found = true
			assert.True(t, strings.HasSuffix(m["file"].(string), "noda.json"),
				"expected error attributed to noda.json, got %+v", m)
		}
	}
	assert.True(t, found, "expected a service-schema error naming 'badstorage', got: %+v", errs)
}

// TestValidateAll_ServiceSchemaErrorAlwaysReported: validateAll must keep
// reporting the service-schema error regardless of per-file attribution
// (only validateFile scopes by requested file).
func TestValidateAll_ServiceSchemaErrorAlwaysReported(t *testing.T) {
	app := setupValidationApp(t, badServiceConfigProjectFiles())

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
		if msg, _ := m["message"].(string); strings.Contains(msg, "badstorage") {
			found = true
		}
	}
	assert.True(t, found, "expected a service-schema error naming 'badstorage', got: %+v", errs)
}
