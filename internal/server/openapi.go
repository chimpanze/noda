package server

import (
	"encoding/json"
	"fmt"

	"github.com/chimpanze/noda/internal/openapi"
	"github.com/gofiber/fiber/v3"
)

// RegisterOpenAPIRoutes serves the OpenAPI spec and Scalar docs UI when
// server.openapi.enabled is true. It is a no-op otherwise.
func (s *Server) RegisterOpenAPIRoutes() error {
	cfg := s.config.OpenAPIConfig()
	if !cfg.Enabled {
		return nil
	}

	doc, err := openapi.Generate(s.config)
	if err != nil {
		return err
	}
	specBytes, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal openapi spec: %w", err)
	}

	s.app.Get(cfg.Path, func(c fiber.Ctx) error {
		c.Set("Content-Type", "application/json")
		return c.Send(specBytes)
	})

	if cfg.Docs {
		html := openapi.ScalarHTML(cfg.Path)
		s.app.Get(cfg.DocsPath, func(c fiber.Ctx) error {
			c.Set("Content-Type", "text/html")
			return c.SendString(html)
		})
	}

	return nil
}
