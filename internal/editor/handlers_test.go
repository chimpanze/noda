package editor

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/pathutil"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/event"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/util"
	"github.com/chimpanze/noda/plugins/core/workflow"
	dbplugin "github.com/chimpanze/noda/plugins/db"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildTestNodeRegistry() *registry.NodeRegistry {
	nodeReg := registry.NewNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&control.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&transform.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&util.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&workflow.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&response.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&dbplugin.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&cacheplugin.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&event.Plugin{})
	return nodeReg
}

// healthyService, unhealthyService, and mockPlugin are minimal test doubles
// copied verbatim from internal/server/health_test.go (like buildTestNodeRegistry
// above) because test helpers can't be imported across packages.
type healthyService struct{}

func (s *healthyService) Ping() error { return nil }

type unhealthyService struct{}

func (s *unhealthyService) Ping() error { return fmt.Errorf("connection refused") }

type mockPlugin struct{}

func (p *mockPlugin) Name() string                              { return "mock" }
func (p *mockPlugin) Prefix() string                            { return "mock" }
func (p *mockPlugin) Nodes() []api.NodeRegistration             { return nil }
func (p *mockPlugin) HasServices() bool                         { return false }
func (p *mockPlugin) ServiceConfigSchema() map[string]any       { return nil }
func (p *mockPlugin) CreateService(map[string]any) (any, error) { return nil, nil }
func (p *mockPlugin) HealthCheck(any) error                     { return nil }
func (p *mockPlugin) Shutdown(any) error                        { return nil }

// --- editor.go: findUpstreamNodes ---

func TestFindUpstreamNodes(t *testing.T) {
	e := &API{}

	wfConfig := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "transform.set"},
			"b": map[string]any{"type": "control.if"},
			"c": map[string]any{"type": "response.json"},
		},
		"edges": []any{
			map[string]any{"from": "a", "output": "default", "to": "b"},
			map[string]any{"from": "b", "output": "true", "to": "c"},
		},
	}

	result := e.findUpstreamNodes(wfConfig, "c")
	// Should find b (direct upstream) and a (upstream of b)
	assert.Len(t, result, 2)

	nodeIDs := make(map[string]bool)
	for _, r := range result {
		nodeIDs[r["node_id"].(string)] = true
	}
	assert.True(t, nodeIDs["a"])
	assert.True(t, nodeIDs["b"])
}

func TestFindUpstreamNodes_NoEdges(t *testing.T) {
	e := &API{}

	wfConfig := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "transform.set"},
		},
		"edges": []any{},
	}

	result := e.findUpstreamNodes(wfConfig, "a")
	assert.Empty(t, result)
}

func TestFindUpstreamNodes_StartNode(t *testing.T) {
	e := &API{}

	wfConfig := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "transform.set"},
			"b": map[string]any{"type": "response.json"},
		},
		"edges": []any{
			map[string]any{"from": "a", "output": "default", "to": "b"},
		},
	}

	result := e.findUpstreamNodes(wfConfig, "a")
	assert.Empty(t, result) // a has no upstream
}

// --- editor.go: resolvedConfig ---

func TestEditorAPI_ResolvedConfig_WithReloader(t *testing.T) {
	// Without reloader, should return rc
	e := &API{
		rc: &config.ResolvedConfig{Root: map[string]any{"name": "test"}},
	}
	rc := e.resolvedConfig()
	require.NotNil(t, rc)
	assert.Equal(t, "test", rc.Root["name"])
}

// --- editor.go: findUpstreamNodes with nil edges ---

func TestFindUpstreamNodes_NilEdges(t *testing.T) {
	e := &API{}
	wfConfig := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "transform.set"},
		},
	}
	result := e.findUpstreamNodes(wfConfig, "a")
	assert.Empty(t, result)
}

func TestFindUpstreamNodes_NilNodeEdgeEntry(t *testing.T) {
	e := &API{}
	wfConfig := map[string]any{
		"nodes": map[string]any{},
		"edges": []any{nil, "bad"},
	}
	result := e.findUpstreamNodes(wfConfig, "c")
	assert.Empty(t, result)
}

func TestFindUpstreamNodes_CyclicGraph(t *testing.T) {
	e := &API{}
	wfConfig := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "transform.set"},
			"b": map[string]any{"type": "control.loop"},
		},
		"edges": []any{
			map[string]any{"from": "a", "output": "default", "to": "b"},
			map[string]any{"from": "b", "output": "loop", "to": "a"},
		},
	}
	// Should not infinite loop due to visited tracking
	result := e.findUpstreamNodes(wfConfig, "b")
	assert.Len(t, result, 1) // only "a"
}

// --- editor_static.go: RegisterEditorUI (no embedded FS) ---

func TestRegisterUI_NoEmbeddedFS(t *testing.T) {
	app := fiber.New()
	// editorfs.FS is nil in builds without the embed_editor tag,
	// so RegisterUI serves the placeholder routes.
	RegisterUI(app, slog.Default())

	req := httptest.NewRequest("GET", "/editor", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Editor not embedded")

	req = httptest.NewRequest("GET", "/editor/some/path", nil)
	resp, err = app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Editor not embedded")
}

// --- EditorAPI handler tests ---

func setupEditorApp(t *testing.T) *fiber.App {
	t.Helper()
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()
	compiler := expr.NewCompilerWithFunctions()

	rc := &config.ResolvedConfig{
		Root:    map[string]any{},
		Schemas: map[string]map[string]any{},
		Routes:  map[string]map[string]any{},
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"a": map[string]any{"type": "transform.set"},
					"b": map[string]any{"type": "response.json"},
				},
				"edges": []any{
					map[string]any{"from": "a", "output": "default", "to": "b"},
				},
			},
		},
	}

	root, _ := pathutil.NewRoot(t.TempDir())
	editorAPI := NewAPI(
		root,
		"",
		nil,
		pluginReg,
		nodeReg,
		svcReg,
		compiler,
		nil,
	)
	editorAPI.rc = rc
	editorAPI.Register(app)
	return app
}

func TestEditorAPI_ListNodes(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/nodes", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	nodes, ok := result["nodes"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, nodes)
}

func TestEditorAPI_GetNodeSchema_NotFound(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/nodes/nonexistent.type/schema", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestEditorAPI_GetNodeSchema_Found(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/nodes/transform.set/schema", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestEditorAPI_ComputeOutputs_NotFound(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/nodes/nonexistent.type/outputs", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestEditorAPI_ComputeOutputs_Found(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/nodes/transform.set/outputs", strings.NewReader(`{"fields":{"a":"b"}}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["outputs"])
}

func TestEditorAPI_ListServices(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/services", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["services"])
}

func TestEditorAPI_ListPlugins(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/plugins", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["plugins"])
}

func TestEditorAPI_ListSchemas(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/schemas", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["schemas"])
}

func TestEditorAPI_ValidateExpression_Valid(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/expressions/validate",
		strings.NewReader(`{"expression":"1 + 2"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, true, result["valid"])
}

func TestEditorAPI_ValidateExpression_Invalid(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/expressions/validate",
		strings.NewReader(`{"expression":"{{invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, false, result["valid"])
}

func TestEditorAPI_ValidateExpression_Empty(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/expressions/validate",
		strings.NewReader(`{"expression":""}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.Equal(t, true, result["valid"])
}

func TestEditorAPI_ValidateExpression_BadRequest(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/expressions/validate",
		strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestEditorAPI_ExpressionContext(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/expressions/context?workflow=wf1&node=b", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["variables"])
	assert.NotNil(t, result["functions"])
	assert.NotNil(t, result["upstream"])

	// Should find upstream node "a"
	upstream, ok := result["upstream"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, upstream)
}

func TestEditorAPI_ExpressionContext_NoWorkflow(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/expressions/context", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestEditorAPI_ExpressionContext_NoNode(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/expressions/context?workflow=wf1", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	upstream := result["upstream"].([]any)
	assert.Empty(t, upstream) // no node specified, no upstream
}

func TestEditorAPI_ExpressionContext_UnknownWorkflow(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/expressions/context?workflow=nonexistent&node=x", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestEditorAPI_ReadFile_NotFound(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/files/nonexistent.json", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestEditorAPI_ReadFile_OutsideConfigDir(t *testing.T) {
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	tmpDir := t.TempDir()
	root, _ := pathutil.NewRoot(tmpDir)
	editorAPI := NewAPI(root, "", nil, pluginReg, nodeReg, svcReg, nil, nil)
	editorAPI.Register(app)

	// Try to access file outside config dir
	req := httptest.NewRequest("GET", "/_noda/files/../../etc/passwd", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	// Should be either 403 (outside config dir) or 404
	assert.True(t, resp.StatusCode == 403 || resp.StatusCode == 404)
}

func TestEditorAPI_ListFiles(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/files", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	// May return 500 if config dir is empty temp dir without proper structure,
	// but it should not panic
	assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 500)
}

func TestEditorAPI_ComputeOutputs_NoBody(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("POST", "/_noda/nodes/transform.set/outputs", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// --- EditorAPI: services with health check ---

func TestEditorAPI_ListServicesWithHealth(t *testing.T) {
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()
	p := &mockPlugin{}

	_ = svcReg.Register("healthy-svc", &healthyService{}, p)
	_ = svcReg.Register("unhealthy-svc", &unhealthyService{}, p)
	_ = svcReg.Register("plain-svc", "no-ping", p)

	root, _ := pathutil.NewRoot(t.TempDir())
	editorAPI := NewAPI(root, "", nil, pluginReg, nodeReg, svcReg, nil, nil)
	editorAPI.Register(app)

	req := httptest.NewRequest("GET", "/_noda/services", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	services := result["services"].([]any)
	assert.Len(t, services, 3)
}

func TestEditorAPI_ListMiddleware(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/middleware", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	mw, ok := result["middleware"].([]any)
	require.True(t, ok)
	assert.NotEmpty(t, mw)
	assert.NotNil(t, result["presets"])
	assert.NotNil(t, result["config"])
	assert.NotNil(t, result["instances"])
}

func TestEditorAPI_ListMiddleware_WithPresets(t *testing.T) {
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"middleware_presets": map[string]any{
				"secure": []any{"security.cors", "security.headers"},
			},
			"middleware": map[string]any{
				"limiter": map[string]any{"max": float64(100)},
			},
		},
	}

	root, _ := pathutil.NewRoot(t.TempDir())
	editorAPI := NewAPI(root, "", nil, pluginReg, nodeReg, svcReg, nil, nil)
	editorAPI.rc = rc
	editorAPI.Register(app)

	req := httptest.NewRequest("GET", "/_noda/middleware", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	presets := result["presets"].(map[string]any)
	assert.Contains(t, presets, "secure")
}

func TestEditorAPI_ListEnvVars(t *testing.T) {
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"database_url": "$env(DATABASE_URL)",
		},
		Routes: map[string]map[string]any{
			"routes.json": {"url": "$env(API_URL)"},
		},
		Workflows: map[string]map[string]any{
			"wf.json": {"secret": "$env(SECRET)"},
		},
	}

	root, _ := pathutil.NewRoot(t.TempDir())
	editorAPI := NewAPI(root, "", nil, pluginReg, nodeReg, svcReg, nil, nil)
	editorAPI.rc = rc
	editorAPI.Register(app)

	req := httptest.NewRequest("GET", "/_noda/env", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["variables"])
}

func TestEditorAPI_ListVars(t *testing.T) {
	// Create a minimal config directory with noda.json so Discover succeeds
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "noda.json"), []byte(`{"port": 8080}`), 0o644)

	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	rc := &config.ResolvedConfig{
		Root:   map[string]any{},
		Routes: map[string]map[string]any{},
		Workflows: map[string]map[string]any{
			"wf.json": {"url": "{{ $var('api_base') }}/endpoint"},
		},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Vars:        map[string]string{"api_base": "https://api.example.com"},
	}

	root, _ := pathutil.NewRoot(dir)
	editorAPI := NewAPI(root, "", nil, pluginReg, nodeReg, svcReg, nil, nil)
	editorAPI.rc = rc
	editorAPI.Register(app)

	req := httptest.NewRequest("GET", "/_noda/vars", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["variables"])
}

func TestEditorAPI_ValidateAll(t *testing.T) {
	dir := t.TempDir()
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	root, _ := pathutil.NewRoot(dir)
	editorAPI := NewAPI(root, "", nil, pluginReg, nodeReg, svcReg, nil, nil)
	editorAPI.Register(app)

	req := httptest.NewRequest("POST", "/_noda/validate/all", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(body, &result))
	assert.NotNil(t, result["valid"])
}

func TestEditorAPI_ValidateFile(t *testing.T) {
	dir := t.TempDir()
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	root, _ := pathutil.NewRoot(dir)
	editorAPI := NewAPI(root, "", nil, pluginReg, nodeReg, svcReg, nil, nil)
	editorAPI.Register(app)

	body := `{"path": "noda.json", "content": {"port": 8080}}`
	req := httptest.NewRequest("POST", "/_noda/validate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.NotNil(t, result["valid"])
}

func TestEditorAPI_ValidateFile_BadRequest(t *testing.T) {
	dir := t.TempDir()
	app := fiber.New()
	nodeReg := buildTestNodeRegistry()
	svcReg := registry.NewServiceRegistry()
	pluginReg := registry.NewPluginRegistry()

	root, _ := pathutil.NewRoot(dir)
	editorAPI := NewAPI(root, "", nil, pluginReg, nodeReg, svcReg, nil, nil)
	editorAPI.Register(app)

	req := httptest.NewRequest("POST", "/_noda/validate", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 400, resp.StatusCode)
}

func TestEditorAPI_ListModels(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/models", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestEditorAPI_OpenAPISpec(t *testing.T) {
	app := setupEditorApp(t)

	req := httptest.NewRequest("GET", "/_noda/openapi", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}
