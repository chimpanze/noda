package server

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
)

const defaultHealthTimeout = 5 * time.Second

// healthTimeout returns the configured health check timeout or the default.
func (s *Server) healthTimeout() time.Duration {
	if serverCfg, ok := s.config.Root["server"].(map[string]any); ok {
		if v, ok := serverCfg["health_timeout"].(string); ok {
			if d, err := time.ParseDuration(v); err == nil {
				return d
			}
		}
	}
	return defaultHealthTimeout
}

// pingWithTimeout runs a Ping() call bounded by the given context.
// If the context expires before Ping() returns, the goroutine will still complete
// in the background. The buffered channel (capacity 1) ensures it won't block
// forever — it writes its result and exits, then both channel and goroutine are GC'd.
func pingWithTimeout(ctx context.Context, checker interface{ Ping() error }) error {
	done := make(chan error, 1)
	go func() {
		done <- checker.Ping()
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("health check timed out")
	}
}

// registerHealthRoutes adds /health, /health/ready, and /health/live endpoints.
func (s *Server) registerHealthRoutes() {
	s.app.Get("/health/live", func(c fiber.Ctx) error {
		return c.JSON(map[string]any{"status": "ok"})
	})

	s.app.Get("/health/ready", func(c fiber.Ctx) error {
		if !s.readyFlag.Load() {
			return c.Status(fiber.StatusServiceUnavailable).JSON(map[string]any{
				"status": "not_ready",
			})
		}
		return c.JSON(map[string]any{"status": "ready"})
	})

	s.app.Get("/health", func(c fiber.Ctx) error {
		timeout := s.healthTimeout()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		services := s.services.All()
		details := make(map[string]string, len(services))
		allHealthy := true

		// Use HealthCheckAll for plugin-based checks (bounded by timeout).
		// The goroutine completes when HealthCheckAll returns; the buffered channel
		// ensures it won't block if the context expires first.
		type healthCheckResult struct {
			errs map[string]error
		}
		hcCh := make(chan healthCheckResult, 1)
		go func() {
			hcCh <- healthCheckResult{errs: s.services.HealthCheckAll()}
		}()

		var healthErrs map[string]error
		select {
		case res := <-hcCh:
			healthErrs = res.errs
		case <-ctx.Done():
			// HealthCheckAll timed out — mark all services as unhealthy
			for name := range services {
				details[name] = "unhealthy"
				s.logger.Error("health check timed out", "service", name)
			}
			return c.Status(fiber.StatusServiceUnavailable).JSON(map[string]any{
				"status":   "unhealthy",
				"services": details,
			})
		}

		for name, svc := range services {
			if err, failed := healthErrs[name]; failed {
				details[name] = "unhealthy"
				s.logger.Error("health check failed", "service", name, "error", err)
				allHealthy = false
				continue
			}
			// Also check Ping() for services not covered by plugin health checks
			if checker, ok := svc.(interface{ Ping() error }); ok {
				if err := pingWithTimeout(ctx, checker); err != nil {
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
