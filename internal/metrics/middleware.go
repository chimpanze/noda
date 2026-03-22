package metrics

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// NewHTTPMiddleware returns a Fiber handler that records HTTP request metrics.
func NewHTTPMiddleware(m *Metrics) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()

		err := c.Next()

		duration := time.Since(start).Seconds()
		method := c.Method()
		route := c.Route().Path
		status := strconv.Itoa(c.Response().StatusCode())

		attrs := metric.WithAttributes(
			attribute.String("method", method),
			attribute.String("route", route),
			attribute.String("status", status),
		)

		m.RequestDuration.Record(c.Context(), duration, attrs)
		m.RequestsTotal.Add(c.Context(), 1, attrs)

		if c.Response().StatusCode() >= 400 {
			m.ErrorsTotal.Add(c.Context(), 1, attrs)
		}

		return err
	}
}
