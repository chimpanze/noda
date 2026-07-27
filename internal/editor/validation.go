package editor

import (
	"context"
	"slices"

	"github.com/chimpanze/noda/internal/config"
	nodaexpr "github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/startup"
	"github.com/gofiber/fiber/v3"
)

// startupFailures runs the same startup phases boot and `noda validate` run,
// reusing the editor's live registries and without opening any connection.
//
// It returns nil when the registries needed for it are absent — dev-mode
// always has them, but tests may construct a bare instance.
func (e *API) startupFailures(rc *config.ResolvedConfig) []startup.Failure {
	if rc == nil || e.plugins == nil || e.nodes == nil || e.compiler == nil {
		return nil
	}
	_, failures := startup.Run(context.Background(), startup.Input{
		RC: rc,
		Live: &startup.Registries{
			Plugins:  e.plugins,
			Nodes:    e.nodes,
			Compiler: e.compiler,
		},
		RootConfigPath: e.root.Join("noda.json"),
		DryRun:         true,
	})
	return failures
}

// validateFile validates a single JSON config against its schema, then reports
// the startup failures implicating that file.
func (e *API) validateFile(c fiber.Ctx) error {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(map[string]any{"error": "invalid request body"})
	}

	sm, smErr := config.NewSecretsManager(e.root.String(), e.envFlag)
	if smErr != nil {
		return c.Status(500).JSON(map[string]any{"error": smErr.Error()})
	}
	rc, errs := config.ValidateAll(e.root.String(), e.envFlag, sm)

	absPath, err := e.root.Resolve(req.Path)
	if err != nil {
		return c.Status(403).JSON(map[string]any{"error": "invalid path"})
	}

	var filtered []map[string]any
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
		// Every startup failure names the files it implicates, so scoping is
		// one containment check. This replaces two workarounds that existed
		// only because the failures were untyped: a special case for
		// registry.ServiceConfigError, which has no file and belongs to the
		// root config, and a trick that pre-trimmed rc.Workflows to this file
		// so other files' errors would not appear (#349). The trick *hid*
		// cross-file failures rather than attributing them, so a file could
		// read clean while the project could not boot.
		for _, f := range e.startupFailures(rc) {
			if !slices.Contains(f.Files, absPath) {
				continue
			}
			filtered = append(filtered, map[string]any{
				"file":    absPath,
				"path":    f.JSONPath,
				"message": f.Err.Error(),
			})
		}
	}

	return c.JSON(map[string]any{
		"valid":  len(filtered) == 0,
		"errors": filtered,
	})
}

// validateAll runs the full validation pipeline and returns all errors.
func (e *API) validateAll(c fiber.Ctx) error {
	sm, smErr := config.NewSecretsManager(e.root.String(), e.envFlag)
	if smErr != nil {
		return c.Status(500).JSON(map[string]any{"error": smErr.Error()})
	}
	rc, errs := config.ValidateAll(e.root.String(), e.envFlag, sm)

	var out []map[string]any
	for _, ve := range errs {
		out = append(out, map[string]any{
			"file":    e.root.Rel(ve.FilePath),
			"path":    ve.JSONPath,
			"message": ve.Message,
		})
	}

	if len(errs) == 0 {
		for _, f := range e.startupFailures(rc) {
			file := ""
			if len(f.Files) > 0 {
				file = e.root.Rel(f.Files[0])
			}
			out = append(out, map[string]any{
				"file":    file,
				"path":    f.JSONPath,
				"message": f.Err.Error(),
			})
		}
	}

	return c.JSON(map[string]any{
		"valid":  len(out) == 0,
		"errors": out,
	})
}

// validateExpression compiles an expression and returns any errors.
func (e *API) validateExpression(c fiber.Ctx) error {
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
