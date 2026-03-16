package server

import (
	"path/filepath"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/devmode"
	nodaexpr "github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/gofiber/fiber/v3"
)

// EditorAPI provides endpoints for the visual editor.
// In dev mode all endpoints are available (including write/delete).
// In production mode only read-only endpoints are registered.
type EditorAPI struct {
	configDir string
	envFlag   string
	reloader  *devmode.Reloader // nil in production mode
	rc        *config.ResolvedConfig
	plugins   *registry.PluginRegistry
	nodes     *registry.NodeRegistry
	services  *registry.ServiceRegistry
	compiler  *nodaexpr.Compiler
}

// NewEditorAPI creates the editor API handler for dev mode (all endpoints).
func NewEditorAPI(
	configDir, envFlag string,
	reloader *devmode.Reloader,
	plugins *registry.PluginRegistry,
	nodes *registry.NodeRegistry,
	services *registry.ServiceRegistry,
	compiler *nodaexpr.Compiler,
) *EditorAPI {
	absDir, err := filepath.Abs(configDir)
	if err != nil {
		absDir = configDir
	}
	return &EditorAPI{
		configDir: absDir,
		envFlag:   envFlag,
		reloader:  reloader,
		plugins:   plugins,
		nodes:     nodes,
		services:  services,
		compiler:  compiler,
	}
}

// NewEditorAPIReadOnly creates the editor API handler for production mode.
// Write and delete endpoints are not registered.
func NewEditorAPIReadOnly(
	configDir, envFlag string,
	rc *config.ResolvedConfig,
	plugins *registry.PluginRegistry,
	nodes *registry.NodeRegistry,
	services *registry.ServiceRegistry,
	compiler *nodaexpr.Compiler,
) *EditorAPI {
	absDir, err := filepath.Abs(configDir)
	if err != nil {
		absDir = configDir
	}
	return &EditorAPI{
		configDir: absDir,
		envFlag:   envFlag,
		rc:        rc,
		plugins:   plugins,
		nodes:     nodes,
		services:  services,
		compiler:  compiler,
	}
}

// Register mounts all editor API routes on the Fiber app.
func (e *EditorAPI) Register(app *fiber.App) {
	api := app.Group("/_noda")

	// File operations (read always available)
	api.Get("/files", e.listFiles)
	api.Get("/files/*", e.readFile)

	// Write/delete only in dev mode
	if e.reloader != nil {
		api.Put("/files/*", e.writeFile)
		api.Delete("/files/*", e.deleteFile)
		api.Post("/tests/run", e.runTests)
		api.Post("/models/generate-migration", e.generateMigration)
		api.Post("/models/generate-crud", e.generateCRUD)
	}

	// Models
	api.Get("/models", e.listModels)

	// Validation
	api.Post("/validate", e.validateFile)
	api.Post("/validate/all", e.validateAll)

	// Node registry
	api.Get("/nodes", e.listNodes)
	api.Get("/nodes/:type/schema", e.getNodeSchema)
	api.Post("/nodes/:type/outputs", e.computeOutputs)

	// Expression tools
	api.Post("/expressions/validate", e.validateExpression)
	api.Get("/expressions/context", e.expressionContext)

	// OpenAPI spec
	api.Get("/openapi", e.openAPISpec)

	// Environment variables
	api.Get("/env", e.listEnvVars)

	// Shared variables
	api.Get("/vars", e.listVars)

	// Services and plugins
	api.Get("/services", e.listServices)
	api.Get("/plugins", e.listPlugins)
	api.Get("/schemas", e.listSchemas)
	api.Get("/middleware", e.listMiddleware)
}

// resolvedConfig returns the current resolved config, preferring the
// reloader's live config in dev mode, falling back to the static config.
func (e *EditorAPI) resolvedConfig() *config.ResolvedConfig {
	if e.reloader != nil {
		return e.reloader.Config()
	}
	return e.rc
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
