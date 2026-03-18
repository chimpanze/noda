package server

import (
	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/devmode"
	nodaexpr "github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/pathutil"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/secrets"
	"github.com/gofiber/fiber/v3"
)

// EditorAPI provides endpoints for the visual editor (dev mode only).
type EditorAPI struct {
	root     pathutil.Root
	envFlag  string
	reloader *devmode.Reloader
	rc       *config.ResolvedConfig // static fallback for tests
	plugins  *registry.PluginRegistry
	nodes    *registry.NodeRegistry
	services *registry.ServiceRegistry
	compiler *nodaexpr.Compiler
	secrets  *secrets.Manager
}

// NewEditorAPI creates the editor API handler for dev mode.
func NewEditorAPI(
	root pathutil.Root,
	envFlag string,
	reloader *devmode.Reloader,
	plugins *registry.PluginRegistry,
	nodes *registry.NodeRegistry,
	services *registry.ServiceRegistry,
	compiler *nodaexpr.Compiler,
	sm *secrets.Manager,
) *EditorAPI {
	return &EditorAPI{
		root:     root,
		envFlag:  envFlag,
		reloader: reloader,
		plugins:  plugins,
		nodes:    nodes,
		services: services,
		compiler: compiler,
		secrets:  sm,
	}
}

// Register mounts all editor API routes on the Fiber app.
func (e *EditorAPI) Register(app *fiber.App) {
	api := app.Group("/_noda")

	// File operations
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
// reloader's live config in dev mode, falling back to a static config.
func (e *EditorAPI) resolvedConfig() *config.ResolvedConfig {
	if e.reloader != nil {
		return e.reloader.Config()
	}
	return e.rc
}
