package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

// setupCodegenApp creates an editor app with a temp dir containing model files.
func setupCodegenApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()
	compiler := expr.NewCompilerWithFunctions()

	tmpDir := t.TempDir()

	// Create a model file
	modelsDir := filepath.Join(tmpDir, "models")
	require.NoError(t, os.MkdirAll(modelsDir, 0o755))
	modelJSON := `{
		"table": "tasks",
		"columns": {
			"id": {"type": "serial", "primary": true},
			"title": {"type": "text", "required": true},
			"status": {"type": "text", "default": "'pending'"}
		}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(modelsDir, "task.json"), []byte(modelJSON), 0o644))

	modelPath := filepath.Join(modelsDir, "task.json")

	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Schemas:   map[string]map[string]any{},
		Routes:    map[string]map[string]any{},
		Workflows: map[string]map[string]any{},
		Models: map[string]map[string]any{
			modelPath: {
				"table": "tasks",
				"columns": map[string]any{
					"id":     map[string]any{"type": "serial", "primary": true},
					"title":  map[string]any{"type": "text", "required": true},
					"status": map[string]any{"type": "text", "default": "'pending'"},
				},
			},
		},
	}

	root, _ := pathutil.NewRoot(tmpDir)
	reloader := devmode.NewReloader(tmpDir, "", rc, nil, slog.Default())
	editorAPI := NewEditorAPI(root, "", reloader, pluginReg, nodeReg, svcReg, compiler, nil)
	editorAPI.rc = rc
	editorAPI.Register(app)
	return app, tmpDir
}

func TestGenerateMigration_Preview(t *testing.T) {
	app, _ := setupCodegenApp(t)

	body := strings.NewReader(`{"confirm": false}`)
	req := httptest.NewRequest("POST", "/_noda/models/generate-migration", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))

	// Should return either a preview or no_changes (depending on whether
	// there's a snapshot to diff against)
	status, _ := result["status"].(string)
	assert.Contains(t, []string{"preview", "no_changes"}, status)
}

func TestGenerateMigration_Confirm(t *testing.T) {
	app, tmpDir := setupCodegenApp(t)

	body := strings.NewReader(`{"confirm": true}`)
	req := httptest.NewRequest("POST", "/_noda/models/generate-migration", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))

	status, _ := result["status"].(string)
	if status == "created" {
		// Verify migration files were created
		migrationsDir := filepath.Join(tmpDir, "migrations")
		entries, err := os.ReadDir(migrationsDir)
		require.NoError(t, err)
		assert.NotEmpty(t, entries)
	}
}

func TestGenerateCRUD_Preview(t *testing.T) {
	app, _ := setupCodegenApp(t)

	body := strings.NewReader(`{
		"model": "models/task.json",
		"confirm": false,
		"service": "postgres",
		"base_path": "/api/tasks",
		"operations": ["list", "get", "create", "update", "delete"]
	}`)
	req := httptest.NewRequest("POST", "/_noda/models/generate-crud", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))

	assert.Equal(t, "preview", result["status"])
	assert.NotNil(t, result["files"])
}

func TestListModels_WithModels(t *testing.T) {
	app, _ := setupCodegenApp(t)

	req := httptest.NewRequest("GET", "/_noda/models", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))

	models, ok := result["models"].([]any)
	require.True(t, ok)
	assert.Len(t, models, 1)
}
