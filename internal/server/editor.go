package server

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/devmode"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/gofiber/fiber/v3"
)

// EditorAPI provides endpoints for the visual editor (dev mode only).
type EditorAPI struct {
	configDir string
	envFlag   string
	reloader  *devmode.Reloader
	plugins   *registry.PluginRegistry
	nodes     *registry.NodeRegistry
	services  *registry.ServiceRegistry
}

// NewEditorAPI creates the editor API handler.
func NewEditorAPI(
	configDir, envFlag string,
	reloader *devmode.Reloader,
	plugins *registry.PluginRegistry,
	nodes *registry.NodeRegistry,
	services *registry.ServiceRegistry,
) *EditorAPI {
	return &EditorAPI{
		configDir: configDir,
		envFlag:   envFlag,
		reloader:  reloader,
		plugins:   plugins,
		nodes:     nodes,
		services:  services,
	}
}

// Register mounts all editor API routes on the Fiber app.
func (e *EditorAPI) Register(app *fiber.App) {
	api := app.Group("/api/editor")

	// File operations
	api.Get("/files", e.listFiles)
	api.Get("/files/*", e.readFile)
	api.Put("/files/*", e.writeFile)
	api.Delete("/files/*", e.deleteFile)

	// Validation
	api.Post("/validate", e.validateFile)
	api.Post("/validate/all", e.validateAll)

	// Node registry
	api.Get("/nodes", e.listNodes)
	api.Get("/nodes/:type/schema", e.getNodeSchema)
	api.Post("/nodes/:type/outputs", e.computeOutputs)

	// Services and plugins
	api.Get("/services", e.listServices)
	api.Get("/plugins", e.listPlugins)
	api.Get("/schemas", e.listSchemas)
}

// listFiles returns all config files grouped by category.
func (e *EditorAPI) listFiles(c fiber.Ctx) error {
	discovered, err := config.Discover(e.configDir, e.envFlag)
	if err != nil {
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}

	rel := func(paths []string) []string {
		result := make([]string, 0, len(paths))
		for _, p := range paths {
			r, _ := filepath.Rel(e.configDir, p)
			result = append(result, r)
		}
		sort.Strings(result)
		return result
	}

	return c.JSON(map[string]any{
		"root":        relPath(e.configDir, discovered.Root),
		"overlay":     relPath(e.configDir, discovered.Overlay),
		"schemas":     rel(discovered.Schemas),
		"routes":      rel(discovered.Routes),
		"workflows":   rel(discovered.Workflows),
		"workers":     rel(discovered.Workers),
		"schedules":   rel(discovered.Schedules),
		"connections": rel(discovered.Connections),
		"tests":       rel(discovered.Tests),
	})
}

// readFile returns the raw JSON content of a config file.
func (e *EditorAPI) readFile(c fiber.Ctx) error {
	relPath, err := url.PathUnescape(c.Params("*"))
	if err != nil {
		return c.Status(400).JSON(map[string]any{"error": "invalid path"})
	}
	absPath := filepath.Join(e.configDir, filepath.Clean(relPath))

	// Security: ensure path is within config dir
	if !strings.HasPrefix(absPath, e.configDir) {
		return c.Status(403).JSON(map[string]any{"error": "path outside config directory"})
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return c.Status(404).JSON(map[string]any{"error": "file not found"})
		}
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}

	c.Set("Content-Type", "application/json")
	return c.Send(data)
}

// writeFile writes JSON content to a config file and triggers hot reload.
func (e *EditorAPI) writeFile(c fiber.Ctx) error {
	relFilePath, err := url.PathUnescape(c.Params("*"))
	if err != nil {
		return c.Status(400).JSON(map[string]any{"error": "invalid path"})
	}
	absPath := filepath.Join(e.configDir, filepath.Clean(relFilePath))

	if !strings.HasPrefix(absPath, e.configDir) {
		return c.Status(403).JSON(map[string]any{"error": "path outside config directory"})
	}

	// Validate JSON
	body := c.Body()
	if !json.Valid(body) {
		return c.Status(400).JSON(map[string]any{"error": "invalid JSON"})
	}

	// Pretty-print with 2-space indent
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return c.Status(400).JSON(map[string]any{"error": err.Error()})
	}
	formatted, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}

	if err := os.WriteFile(absPath, append(formatted, '\n'), 0o644); err != nil {
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}

	// Trigger hot reload
	e.reloader.HandleChange(absPath)

	return c.JSON(map[string]any{"status": "saved", "path": relFilePath})
}

// deleteFile removes a config file.
func (e *EditorAPI) deleteFile(c fiber.Ctx) error {
	relFilePath, err := url.PathUnescape(c.Params("*"))
	if err != nil {
		return c.Status(400).JSON(map[string]any{"error": "invalid path"})
	}
	absPath := filepath.Join(e.configDir, filepath.Clean(relFilePath))

	if !strings.HasPrefix(absPath, e.configDir) {
		return c.Status(403).JSON(map[string]any{"error": "path outside config directory"})
	}

	if err := os.Remove(absPath); err != nil {
		if os.IsNotExist(err) {
			return c.Status(404).JSON(map[string]any{"error": "file not found"})
		}
		return c.Status(500).JSON(map[string]any{"error": err.Error()})
	}

	e.reloader.HandleChange(absPath)

	return c.JSON(map[string]any{"status": "deleted", "path": relFilePath})
}

// validateFile validates a single JSON config against its schema.
func (e *EditorAPI) validateFile(c fiber.Ctx) error {
	var req struct {
		Path    string `json:"path"`
		Content any    `json:"content"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(map[string]any{"error": "invalid request body"})
	}

	// Run full validation to catch cross-references
	_, errs := config.ValidateAll(e.configDir, e.envFlag)

	// Filter errors for the requested file
	var filtered []map[string]any
	absPath := filepath.Join(e.configDir, filepath.Clean(req.Path))
	for _, ve := range errs {
		if ve.FilePath == absPath || ve.FilePath == req.Path {
			filtered = append(filtered, map[string]any{
				"file":    ve.FilePath,
				"path":    ve.JSONPath,
				"message": ve.Message,
			})
		}
	}

	return c.JSON(map[string]any{
		"valid":  len(filtered) == 0,
		"errors": filtered,
	})
}

// validateAll runs the full validation pipeline and returns all errors.
func (e *EditorAPI) validateAll(c fiber.Ctx) error {
	_, errs := config.ValidateAll(e.configDir, e.envFlag)

	var errors []map[string]any
	for _, ve := range errs {
		errors = append(errors, map[string]any{
			"file":    relPath(e.configDir, ve.FilePath),
			"path":    ve.JSONPath,
			"message": ve.Message,
		})
	}

	return c.JSON(map[string]any{
		"valid":  len(errs) == 0,
		"errors": errors,
	})
}

// listNodes returns all registered node types with their descriptors.
func (e *EditorAPI) listNodes(c fiber.Ctx) error {
	types := e.nodes.AllTypes()
	sort.Strings(types)

	nodes := make([]map[string]any, 0, len(types))
	for _, t := range types {
		desc, ok := e.nodes.GetDescriptor(t)
		if !ok {
			continue
		}

		// Get outputs by creating a temporary executor
		outputs, _ := e.nodes.OutputsForType(t)

		entry := map[string]any{
			"type":    t,
			"name":    desc.Name(),
			"outputs": outputs,
		}

		if deps := desc.ServiceDeps(); len(deps) > 0 {
			depsMap := make(map[string]any, len(deps))
			for slot, dep := range deps {
				depsMap[slot] = map[string]any{
					"prefix":   dep.Prefix,
					"required": dep.Required,
				}
			}
			entry["service_deps"] = depsMap
		}

		if schema := desc.ConfigSchema(); schema != nil {
			entry["has_schema"] = true
		}

		nodes = append(nodes, entry)
	}

	return c.JSON(map[string]any{"nodes": nodes})
}

// getNodeSchema returns the JSON Schema for a node type's config.
func (e *EditorAPI) getNodeSchema(c fiber.Ctx) error {
	nodeType := c.Params("type")
	desc, ok := e.nodes.GetDescriptor(nodeType)
	if !ok {
		return c.Status(404).JSON(map[string]any{"error": fmt.Sprintf("node type %q not found", nodeType)})
	}

	schema := desc.ConfigSchema()
	if schema == nil {
		return c.JSON(map[string]any{})
	}
	return c.JSON(schema)
}

// computeOutputs creates a temporary executor and returns its outputs.
func (e *EditorAPI) computeOutputs(c fiber.Ctx) error {
	nodeType := c.Params("type")
	factory, ok := e.nodes.GetFactory(nodeType)
	if !ok {
		return c.Status(404).JSON(map[string]any{"error": fmt.Sprintf("node type %q not found", nodeType)})
	}

	var cfg map[string]any
	if err := c.Bind().JSON(&cfg); err != nil {
		cfg = nil
	}

	executor := factory(cfg)
	return c.JSON(map[string]any{"outputs": executor.Outputs()})
}

// listServices returns all configured service instances.
func (e *EditorAPI) listServices(c fiber.Ctx) error {
	all := e.services.All()
	services := make([]map[string]any, 0, len(all))

	for name, svc := range all {
		prefix, _ := e.services.GetPrefix(name)
		entry := map[string]any{
			"name":   name,
			"prefix": prefix,
		}

		// Check health
		if checker, ok := svc.(interface{ Ping() error }); ok {
			if err := checker.Ping(); err != nil {
				entry["health"] = "unhealthy"
				entry["error"] = err.Error()
			} else {
				entry["health"] = "healthy"
			}
		} else {
			entry["health"] = "unknown"
		}

		services = append(services, entry)
	}

	sort.Slice(services, func(i, j int) bool {
		return services[i]["name"].(string) < services[j]["name"].(string)
	})

	return c.JSON(map[string]any{"services": services})
}

// listPlugins returns all loaded plugins with their prefixes and node counts.
func (e *EditorAPI) listPlugins(c fiber.Ctx) error {
	all := e.plugins.All()
	plugins := make([]map[string]any, 0, len(all))

	for _, p := range all {
		plugins = append(plugins, map[string]any{
			"name":         p.Name(),
			"prefix":       p.Prefix(),
			"has_services": p.HasServices(),
			"node_count":   len(p.Nodes()),
		})
	}

	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i]["prefix"].(string) < plugins[j]["prefix"].(string)
	})

	return c.JSON(map[string]any{"plugins": plugins})
}

// listSchemas returns all shared schema definitions.
func (e *EditorAPI) listSchemas(c fiber.Ctx) error {
	rc := e.reloader.Config()

	schemas := make([]map[string]any, 0, len(rc.Schemas))
	for path, schema := range rc.Schemas {
		schemas = append(schemas, map[string]any{
			"path":   relPath(e.configDir, path),
			"schema": schema,
		})
	}

	sort.Slice(schemas, func(i, j int) bool {
		return schemas[i]["path"].(string) < schemas[j]["path"].(string)
	})

	return c.JSON(map[string]any{"schemas": schemas})
}

// relPath returns a relative path from base, or the original if Rel fails.
func relPath(base, path string) string {
	if path == "" {
		return ""
	}
	r, err := filepath.Rel(base, path)
	if err != nil {
		return path
	}
	return r
}
