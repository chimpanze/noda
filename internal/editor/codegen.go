package editor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/generate"
	"github.com/chimpanze/noda/internal/openapi"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/gofiber/fiber/v3"
)

// runTests executes a test suite and returns results.
// Only available in dev mode.
func (e *API) runTests(c fiber.Ctx) error {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.Bind().JSON(&req); err != nil || req.Path == "" {
		return c.Status(400).JSON(map[string]any{"error": "path is required"})
	}

	rc := e.resolvedConfig()
	if rc == nil {
		return c.Status(500).JSON(map[string]any{"error": "no config available"})
	}

	// Resolve the test path and look it up by absolute key
	absPath, err := e.root.Resolve(req.Path)
	if err != nil {
		return c.Status(403).JSON(map[string]any{"error": "path outside config directory"})
	}
	testData, ok := rc.Tests[absPath]
	if !ok {
		return c.Status(404).JSON(map[string]any{"error": fmt.Sprintf("test file %q not found", req.Path)})
	}

	// Load the single test suite
	suites, err := nodatesting.LoadTests(&config.ResolvedConfig{
		Tests:     map[string]map[string]any{absPath: testData},
		Workflows: rc.Workflows,
	})
	if err != nil {
		return c.Status(400).JSON(map[string]any{"error": err.Error()})
	}
	if len(suites) == 0 {
		return c.Status(400).JSON(map[string]any{"error": "no test suite found"})
	}

	var secretsCtx map[string]any
	if e.secrets != nil {
		secretsCtx = e.secrets.ExpressionContext()
	}
	results := nodatesting.RunTestSuite(suites[0], rc, e.nodes, secretsCtx)

	// Serialize results
	output := make([]map[string]any, 0, len(results))
	for _, r := range results {
		entry := map[string]any{
			"case_name": r.CaseName,
			"passed":    r.Passed,
			"duration":  r.Duration.String(),
			"expected": map[string]any{
				"status":     r.Expected.Status,
				"output":     r.Expected.Output,
				"error_node": r.Expected.ErrorNode,
			},
			"actual": map[string]any{
				"status":     r.Actual.Status,
				"outputs":    r.Actual.Outputs,
				"error_node": r.Actual.ErrorNode,
				"error_msg":  r.Actual.ErrorMsg,
			},
		}
		if r.Error != "" {
			entry["error"] = r.Error
		}
		output = append(output, entry)
	}

	return c.JSON(output)
}

// listModels returns all parsed model definitions.
func (e *API) listModels(c fiber.Ctx) error {
	rc := e.resolvedConfig()
	if rc == nil {
		return c.Status(500).JSON(map[string]any{"error": "no config available"})
	}

	models := make([]map[string]any, 0, len(rc.Models))
	for path, model := range rc.Models {
		models = append(models, map[string]any{
			"path":  e.root.Rel(path),
			"model": model,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i]["path"].(string) < models[j]["path"].(string)
	})

	return c.JSON(map[string]any{"models": models})
}

// generateMigration generates SQL migration from model definitions.
func (e *API) generateMigration(c fiber.Ctx) error {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	_ = c.Bind().JSON(&req)

	modelsDir := e.root.Join("models")
	migrationsDir := e.root.Join("migrations")

	dialect := e.detectDBDialect()
	upSQL, downSQL, err := generate.GenerateMigration(modelsDir, dialect)
	if err != nil {
		return c.Status(400).JSON(map[string]any{"error": err.Error()})
	}

	if upSQL == "" {
		return c.JSON(map[string]any{"status": "no_changes", "up": "", "down": ""})
	}

	if !req.Confirm {
		return c.JSON(map[string]any{"status": "preview", "up": upSQL, "down": downSQL})
	}

	// Write migration files
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}

	// Find next migration number
	entries, _ := os.ReadDir(migrationsDir)
	nextNum := 1
	for _, entry := range entries {
		name := entry.Name()
		if len(name) >= 4 {
			var n int
			if _, err := fmt.Sscanf(name, "%04d", &n); err == nil && n >= nextNum {
				nextNum = n + 1
			}
		}
	}

	prefix := fmt.Sprintf("%04d", nextNum)
	upPath := filepath.Join(migrationsDir, prefix+"_models.up.sql")
	downPath := filepath.Join(migrationsDir, prefix+"_models.down.sql")

	if err := os.WriteFile(upPath, []byte(upSQL), 0o644); err != nil {
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}
	if err := os.WriteFile(downPath, []byte(downSQL), 0o644); err != nil {
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}

	// Save snapshot
	if err := generate.SaveSnapshot(modelsDir); err != nil {
		return c.Status(500).JSON(map[string]any{"error": fmt.Sprintf("snapshot: %v", err)})
	}

	e.reloader.HandleChange(upPath)

	return c.JSON(map[string]any{
		"status":    "created",
		"up":        upSQL,
		"down":      downSQL,
		"up_path":   e.root.Rel(upPath),
		"down_path": e.root.Rel(downPath),
	})
}

// detectDBDialect inspects the resolved config for a db service driver.
// Returns "sqlite" if any db service uses the sqlite driver, otherwise "postgres".
func (e *API) detectDBDialect() string {
	rc := e.resolvedConfig()
	if rc == nil {
		return "postgres"
	}
	services, _ := rc.Root["services"].(map[string]any)
	for _, svc := range services {
		svcMap, ok := svc.(map[string]any)
		if !ok {
			continue
		}
		pluginName, _ := svcMap["plugin"].(string)
		if pluginName != "db" && pluginName != "postgres" {
			continue
		}
		cfg, _ := svcMap["config"].(map[string]any)
		if driver, _ := cfg["driver"].(string); driver == "sqlite" {
			return "sqlite"
		}
	}
	return "postgres"
}

// generateCRUD generates route, workflow, and schema files for a model.
func (e *API) generateCRUD(c fiber.Ctx) error {
	var req struct {
		Model      string   `json:"model"`
		Confirm    bool     `json:"confirm"`
		Service    string   `json:"service"`
		BasePath   string   `json:"base_path"`
		Operations []string `json:"operations"`
		Artifacts  []string `json:"artifacts"`
		ScopeCol   string   `json:"scope_column"`
		ScopeParam string   `json:"scope_param"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(map[string]any{"error": "invalid request body"})
	}

	// Load the model
	modelPath, err := e.root.Resolve(req.Model)
	if err != nil {
		return c.Status(403).JSON(map[string]any{"error": "path outside config directory"})
	}

	modelData, err := os.ReadFile(modelPath)
	if err != nil {
		return c.Status(404).JSON(map[string]any{"error": fmt.Sprintf("model not found: %s", req.Model)})
	}

	var model map[string]any
	if err := json.Unmarshal(modelData, &model); err != nil {
		return c.Status(400).JSON(map[string]any{"error": fmt.Sprintf("invalid model JSON: %v", err)})
	}

	opts := generate.CRUDOptions{
		Service:    req.Service,
		BasePath:   req.BasePath,
		Operations: req.Operations,
		Artifacts:  req.Artifacts,
		ScopeCol:   req.ScopeCol,
		ScopeParam: req.ScopeParam,
	}

	result := generate.GenerateCRUD(model, opts)

	if !req.Confirm {
		return c.JSON(map[string]any{"status": "preview", "files": result.Files})
	}

	// Write all files
	for relFilePath, content := range result.Files {
		absPath, err := e.root.Resolve(relFilePath)
		if err != nil {
			return c.Status(400).JSON(map[string]any{"error": "generated path outside config directory"})
		}
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		data, err := json.MarshalIndent(content, "", "  ")
		if err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		if err := os.WriteFile(absPath, append(data, '\n'), 0o644); err != nil {
			return c.Status(500).JSON(map[string]any{"error": err.Error()})
		}
		e.reloader.HandleChange(absPath)
	}

	return c.JSON(map[string]any{"status": "created", "files": result.Files})
}

// openAPISpec generates the OpenAPI spec via the unified generator. It mirrors
// public exposure: when server.openapi.enabled is false it returns
// {"enabled": false} so the editor tab can show a disabled notice.
func (e *API) openAPISpec(c fiber.Ctx) error {
	rc := e.resolvedConfig()
	if rc == nil {
		return c.Status(500).JSON(map[string]any{"error": "no config available"})
	}
	if !rc.OpenAPIConfig().Enabled {
		return c.JSON(map[string]any{"enabled": false})
	}
	doc, err := openapi.Generate(rc)
	if err != nil {
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}
	return c.JSON(doc)
}
