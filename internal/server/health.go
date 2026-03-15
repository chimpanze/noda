package server

import (
	"strings"
	"sync/atomic"

	"github.com/gofiber/fiber/v3"
)

// readyFlag tracks whether the server has completed initialization.
var readyFlag atomic.Bool

// SetReady marks the server as ready to accept traffic.
func SetReady() { readyFlag.Store(true) }

// registerHealthRoutes adds /health, /health/ready, and /health/live endpoints.
func (s *Server) registerHealthRoutes() {
	s.app.Get("/health/live", func(c fiber.Ctx) error {
		return c.JSON(map[string]any{"status": "ok"})
	})

	s.app.Get("/health/ready", func(c fiber.Ctx) error {
		if !readyFlag.Load() {
			return c.Status(fiber.StatusServiceUnavailable).JSON(map[string]any{
				"status": "not_ready",
			})
		}
		return c.JSON(map[string]any{"status": "ready"})
	})

	s.app.Get("/health", func(c fiber.Ctx) error {
		services := s.services.All()
		details := make(map[string]string, len(services))
		allHealthy := true

		// Use HealthCheckAll for plugin-based checks
		healthErrs := s.services.HealthCheckAll()
		failedServices := make(map[string]bool)
		for _, err := range healthErrs {
			s.logger.Error("health check failed", "error", err)
			for name := range services {
				if strings.Contains(err.Error(), name) {
					failedServices[name] = true
				}
			}
		}

		for name, svc := range services {
			if failedServices[name] {
				details[name] = "unhealthy"
				allHealthy = false
				continue
			}
			// Also check Ping() for services not covered by plugin health checks
			if checker, ok := svc.(interface{ Ping() error }); ok {
				if err := checker.Ping(); err != nil {
					details[name] = "unhealthy"
					s.logger.Error("health check failed", "service", name, "error", err)
					allHealthy = false
					continue
				}
			}
			details[name] = "ok"
		}

		status := "healthy"
		httpStatus := fiber.StatusOK
		if !allHealthy {
			status = "unhealthy"
			httpStatus = fiber.StatusServiceUnavailable
		}

		return c.Status(httpStatus).JSON(map[string]any{
			"status":   status,
			"services": details,
		})
	})
}
