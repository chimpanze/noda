package server

import (
	"github.com/chimpanze/noda/internal/config"
	nodaexpr "github.com/chimpanze/noda/internal/expr"
	"github.com/gofiber/fiber/v3"
)

// validateFile validates a single JSON config against its schema.
func (e *EditorAPI) validateFile(c fiber.Ctx) error {
	var req struct {
		Path    string `json:"path"`
		Content any    `json:"content"`
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
	_, errs := config.ValidateAll(e.root.String(), e.envFlag, sm)

	// Filter errors for the requested file
	var filtered []map[string]any
	absPath, _ := e.root.Resolve(req.Path)
	for _, ve := range errs {
		if ve.FilePath == absPath {
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
	sm, smErr := config.NewSecretsManager(e.root.String(), e.envFlag)
	if smErr != nil {
		return c.Status(500).JSON(map[string]any{"error": smErr.Error()})
	}
	_, errs := config.ValidateAll(e.root.String(), e.envFlag, sm)

	var errors []map[string]any
	for _, ve := range errs {
		errors = append(errors, map[string]any{
			"file":    e.root.Rel(ve.FilePath),
			"path":    ve.JSONPath,
			"message": ve.Message,
		})
	}

	return c.JSON(map[string]any{
		"valid":  len(errs) == 0,
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
