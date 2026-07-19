package server

import (
	"errors"

	"github.com/chimpanze/noda/internal/config"
	nodaexpr "github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/gofiber/fiber/v3"
)

// startupDryRunErrors runs the same node/service/expression startup checks
// `noda validate` performs, but without live service connections. It only
// runs when file-level validation was clean and the registries needed for
// it (plugins, nodes, compiler) are available — dev-mode EditorAPI always
// has them, but tests may construct a bare instance.
func (e *EditorAPI) startupDryRunErrors(rc *config.ResolvedConfig) []error {
	if rc == nil || e.plugins == nil || e.nodes == nil || e.compiler == nil {
		return nil
	}
	deferred, errs := registry.CollectDeferredServices(rc)
	errs = append(errs, registry.ValidateStartupDryRun(rc, e.plugins, e.nodes, e.compiler, deferred)...)
	return errs
}

// validateFile validates a single JSON config against its schema.
func (e *EditorAPI) validateFile(c fiber.Ctx) error {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(map[string]any{"error": "invalid request body"})
	}

	// Run full validation to catch cross-references.
	// Create a fresh secrets manager for validation (reuses same providers).
	sm, smErr := config.NewSecretsManager(e.root.String(), e.envFlag)
	if smErr != nil {
		return c.Status(500).JSON(map[string]any{"error": smErr.Error()})
	}
	rc, errs := config.ValidateAll(e.root.String(), e.envFlag, sm)

	// Filter errors for the requested file
	var filtered []map[string]any
	absPath, err := e.root.Resolve(req.Path)
	if err != nil {
		return c.Status(403).JSON(map[string]any{"error": "invalid path"})
	}
	for _, ve := range errs {
		if ve.FilePath == absPath {
			filtered = append(filtered, map[string]any{
				"file":    ve.FilePath,
				"path":    ve.JSONPath,
				"message": ve.Message,
			})
		}
	}

	if len(errs) == 0 {
		// Scope dry-run errors to the requested file's workflows (#349):
		// rc.Workflows is keyed by file path, so restrict to this file.
		scoped := *rc
		if wf, ok := rc.Workflows[absPath]; ok {
			scoped.Workflows = map[string]map[string]any{absPath: wf}
		} else {
			scoped.Workflows = nil // non-workflow file: workflow dry-run errors are unrelated here
		}

		// Service-schema errors (registry.ServiceConfigError) always
		// originate from rc.Root — services are declared project-wide, not
		// per-file — so scoping workflows to this file doesn't scope them.
		// Attribute them to this file only when the requested file *is* the
		// root config (noda.json); otherwise they belong to a different
		// file than the one being saved and would be misleading here.
		// validateAll (below) still reports them unconditionally.
		isRootConfig := absPath == e.root.Join("noda.json")
		for _, dErr := range e.startupDryRunErrors(&scoped) {
			var svcErr *registry.ServiceConfigError
			if errors.As(dErr, &svcErr) && !isRootConfig {
				continue
			}
			filtered = append(filtered, map[string]any{
				"file":    absPath,
				"path":    "",
				"message": dErr.Error(),
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
	sm, smErr := config.NewSecretsManager(e.root.String(), e.envFlag)
	if smErr != nil {
		return c.Status(500).JSON(map[string]any{"error": smErr.Error()})
	}
	rc, errs := config.ValidateAll(e.root.String(), e.envFlag, sm)

	var errors []map[string]any
	for _, ve := range errs {
		errors = append(errors, map[string]any{
			"file":    e.root.Rel(ve.FilePath),
			"path":    ve.JSONPath,
			"message": ve.Message,
		})
	}

	if len(errs) == 0 {
		for _, dErr := range e.startupDryRunErrors(rc) {
			errors = append(errors, map[string]any{
				"file":    "",
				"path":    "",
				"message": dErr.Error(),
			})
		}
	}

	return c.JSON(map[string]any{
		"valid":  len(errors) == 0,
		"errors": errors,
	})
}

// validateExpression compiles an expression and returns any errors.
func (e *EditorAPI) validateExpression(c fiber.Ctx) error {
	var req struct {
		Expression string `json:"expression"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(map[string]any{"error": "invalid request body"})
	}

	if req.Expression == "" {
		return c.JSON(map[string]any{"valid": true})
	}

	compiler := e.compiler
	if compiler == nil {
		compiler = nodaexpr.NewCompiler()
	}

	_, err := compiler.Compile(req.Expression)
	if err != nil {
		return c.JSON(map[string]any{
			"valid": false,
			"error": err.Error(),
		})
	}

	return c.JSON(map[string]any{"valid": true})
}
