package server

import (
	"io/fs"
	"mime"
	"path/filepath"
	"strings"

	"github.com/chimpanze/noda/editorfs"
	"github.com/chimpanze/noda/internal/trace"
	"github.com/gofiber/fiber/v3"
)

// RegisterEditorUI serves the embedded editor SPA at /editor/.
// If the binary was built without the embed_editor tag, a placeholder is shown.
// In production mode a no-op trace WebSocket is registered so the editor
// connects without errors; in dev mode the real trace endpoint is registered
// separately via trace.RegisterTraceWebSocket.
func (s *Server) RegisterEditorUI() {
	if editorfs.FS == nil {
		s.app.Get("/editor", func(c fiber.Ctx) error {
			return c.Status(fiber.StatusOK).SendString("Editor not embedded. Build with: go build -tags embed_editor")
		})
		s.app.Get("/editor/*", func(c fiber.Ctx) error {
			return c.Status(fiber.StatusOK).SendString("Editor not embedded. Build with: go build -tags embed_editor")
		})
		return
	}

	// Read index.html once at startup for SPA fallback
	indexHTML, err := fs.ReadFile(editorfs.FS, "index.html")
	if err != nil {
		s.logger.Warn("editor index.html not found in embedded assets", "error", err.Error())
		return
	}

	s.app.Get("/editor", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html; charset=utf-8")
		c.Set("Cache-Control", "no-cache")
		return c.Send(indexHTML)
	})

	s.app.Get("/editor/*", func(c fiber.Ctx) error {
		path := c.Params("*")
		if path == "" || path == "/" {
			c.Set("Content-Type", "text/html; charset=utf-8")
			c.Set("Cache-Control", "no-cache")
			return c.Send(indexHTML)
		}

		// Try to serve the exact file
		data, err := fs.ReadFile(editorfs.FS, path)
		if err == nil {
			ct := mime.TypeByExtension(filepath.Ext(path))
			if ct == "" {
				ct = "application/octet-stream"
			}
			c.Set("Content-Type", ct)

			// Content-hashed assets get immutable caching
			if strings.HasPrefix(path, "assets/") {
				c.Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			return c.Send(data)
		}

		// SPA fallback: paths without file extension serve index.html
		if !strings.Contains(filepath.Base(path), ".") {
			c.Set("Content-Type", "text/html; charset=utf-8")
			c.Set("Cache-Control", "no-cache")
			return c.Send(indexHTML)
		}

		return fiber.ErrNotFound
	})

	// Register a no-op /ws/trace so the editor can connect without errors.
	// In dev mode this is overridden by trace.RegisterTraceWebSocket which
	// streams real execution events.
	trace.RegisterNoOpTraceWebSocket(s.app)

	s.logger.Info("editor UI registered at /editor/")
}
