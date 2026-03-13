package server

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/generate"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/gofiber/fiber/v3"
)

// runTests executes a test suite and returns results.
// Only available in dev mode.
func (e *EditorAPI) runTests(c fiber.Ctx) error {
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

	// Find the test data by matching the path against rc.Tests keys
	absPath := filepath.Join(e.configDir, filepath.Clean(req.Path))
	var testData map[string]any
	var testFilePath string
	for p, data := range rc.Tests {
		if p == absPath || p == req.Path {
			testData = data
			testFilePath = p
			break
		}
	}

	if testData == nil {
		return c.Status(404).JSON(map[string]any{"error": fmt.Sprintf("test file %q not found", req.Path)})
	}

	// Load the single test suite
	suites, err := nodatesting.LoadTests(&config.ResolvedConfig{
		Tests:     map[string]map[string]any{testFilePath: testData},
		Workflows: rc.Workflows,
	})
	if err != nil {
		return c.Status(400).JSON(map[string]any{"error": err.Error()})
	}
	if len(suites) == 0 {
		return c.Status(400).JSON(map[string]any{"error": "no test suite found"})
	}

	results := nodatesting.RunTestSuite(suites[0], rc, e.nodes)

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
func (e *EditorAPI) listModels(c fiber.Ctx) error {
	rc := e.resolvedConfig()

	models := make([]map[string]any, 0, len(rc.Models))
	for path, model := range rc.Models {
		models = append(models, map[string]any{
			"path":  relPath(e.configDir, path),
			"model": model,
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i]["path"].(string) < models[j]["path"].(string)
	})

	return c.JSON(map[string]any{"models": models})
}

// generateMigration generates SQL migration from model definitions.
func (e *EditorAPI) generateMigration(c fiber.Ctx) error {
	var req struct {
		Confirm bool `json:"confirm"`
	}
	_ = c.Bind().JSON(&req)

	modelsDir := filepath.Join(e.configDir, "models")
	migrationsDir := filepath.Join(e.configDir, "migrations")

	upSQL, downSQL, err := generate.GenerateMigration(modelsDir, migrationsDir)
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
		"up_path":   relPath(e.configDir, upPath),
		"down_path": relPath(e.configDir, downPath),
	})
}

// generateCRUD generates route, workflow, and schema files for a model.
func (e *EditorAPI) generateCRUD(c fiber.Ctx) error {
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
	modelPath := filepath.Join(e.configDir, filepath.Clean(req.Model))
	if !strings.HasPrefix(modelPath, e.configDir) {
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
		absPath := filepath.Join(e.configDir, filepath.Clean(relFilePath))
		if !strings.HasPrefix(absPath, e.configDir) {
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

// openAPISpec generates an OpenAPI 3.0 spec from the resolved config.
func (e *EditorAPI) openAPISpec(c fiber.Ctx) error {
	rc := e.resolvedConfig()
	if rc == nil {
		return c.Status(500).JSON(map[string]any{"error": "no config available"})
	}

	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   "Noda API",
			"version": "1.0.0",
		},
	}

	// Build paths from routes
	paths := make(map[string]any)
	for _, routeData := range rc.Routes {
		routes := normalizeRoutes(routeData)
		for _, route := range routes {
			method, _ := route["method"].(string)
			path, _ := route["path"].(string)
			if method == "" || path == "" {
				continue
			}

			// Convert :param to {param} for OpenAPI
			oaPath := convertPath(path)
			method = strings.ToLower(method)

			op := map[string]any{}

			if summary, ok := route["summary"].(string); ok && summary != "" {
				op["summary"] = summary
			}
			if tags, ok := route["tags"].([]any); ok && len(tags) > 0 {
				op["tags"] = tags
			}

			// Parameters from path
			params := extractPathParams(path)

			// Params schema
			if paramsObj, ok := route["params"].(map[string]any); ok {
				if schema, ok := paramsObj["schema"].(map[string]any); ok {
					for _, p := range params {
						pm := p.(map[string]any)
						pm["schema"] = schema
					}
				}
			}

			// Query schema
			if queryObj, ok := route["query"].(map[string]any); ok {
				if schema, ok := queryObj["schema"].(map[string]any); ok {
					params = append(params, map[string]any{
						"name":     "query",
						"in":       "query",
						"schema":   schema,
						"required": false,
					})
				}
			}

			if len(params) > 0 {
				op["parameters"] = params
			}

			// Request body
			if body, ok := route["body"].(map[string]any); ok {
				contentType := "application/json"
				if ct, ok := body["content_type"].(string); ok && ct != "" {
					contentType = ct
				}
				reqBody := map[string]any{
					"content": map[string]any{
						contentType: map[string]any{},
					},
				}
				if schema, ok := body["schema"]; ok {
					reqBody["content"].(map[string]any)[contentType] = map[string]any{
						"schema": schema,
					}
				}
				op["requestBody"] = reqBody
			}

			// Responses
			responses := map[string]any{}
			if resp, ok := route["response"].(map[string]any); ok {
				if statuses, ok := resp["statuses"].(map[string]any); ok {
					for code, entry := range statuses {
						statusEntry := map[string]any{"description": ""}
						if em, ok := entry.(map[string]any); ok {
							if desc, ok := em["description"].(string); ok {
								statusEntry["description"] = desc
							}
							if schema, ok := em["schema"]; ok {
								statusEntry["content"] = map[string]any{
									"application/json": map[string]any{
										"schema": schema,
									},
								}
							}
						}
						responses[code] = statusEntry
					}
				}
			}
			if len(responses) == 0 {
				responses["200"] = map[string]any{"description": "OK"}
			}
			op["responses"] = responses

			// Security from middleware
			if mw, ok := route["middleware"].([]any); ok {
				for _, m := range mw {
					if ms, ok := m.(string); ok && (ms == "auth.jwt" || strings.HasPrefix(ms, "auth.")) {
						op["security"] = []any{map[string]any{"bearerAuth": []any{}}}
						break
					}
				}
			}

			if _, exists := paths[oaPath]; !exists {
				paths[oaPath] = map[string]any{}
			}
			paths[oaPath].(map[string]any)[method] = op
		}
	}
	spec["paths"] = paths

	// Components/schemas
	if len(rc.Schemas) > 0 {
		schemas := make(map[string]any)
		for path, schema := range rc.Schemas {
			name := filepath.Base(path)
			name = strings.TrimSuffix(name, filepath.Ext(name))
			schemas[name] = schema
		}
		spec["components"] = map[string]any{
			"schemas": schemas,
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "JWT",
				},
			},
		}
	}

	return c.JSON(spec)
}

func normalizeRoutes(data map[string]any) []map[string]any {
	// A route file can be a single route object or contain routes under keys
	if _, hasMethod := data["method"]; hasMethod {
		return []map[string]any{data}
	}
	// Check if it's a route group file with nested routes
	var routes []map[string]any
	for _, v := range data {
		if rm, ok := v.(map[string]any); ok {
			if _, hasMethod := rm["method"]; hasMethod {
				routes = append(routes, rm)
			}
		}
	}
	return routes
}

func convertPath(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			parts[i] = "{" + p[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

func extractPathParams(path string) []any {
	var params []any
	for _, part := range strings.Split(path, "/") {
		if strings.HasPrefix(part, ":") {
			params = append(params, map[string]any{
				"name":     part[1:],
				"in":       "path",
				"required": true,
				"schema":   map[string]any{"type": "string"},
			})
		}
	}
	return params
}
