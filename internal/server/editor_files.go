package server

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/gofiber/fiber/v3"
)

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
		"vars":        relPath(e.configDir, discovered.Vars),
		"schemas":     rel(discovered.Schemas),
		"routes":      rel(discovered.Routes),
		"workflows":   rel(discovered.Workflows),
		"workers":     rel(discovered.Workers),
		"schedules":   rel(discovered.Schedules),
		"connections": rel(discovered.Connections),
		"tests":       rel(discovered.Tests),
		"models":      rel(discovered.Models),
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
// Only available in dev mode.
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
// Only available in dev mode.
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
