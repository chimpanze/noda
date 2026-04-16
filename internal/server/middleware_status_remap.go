package server

import (
	"fmt"
	"strconv"

	"github.com/chimpanze/noda/internal/metrics"
	"github.com/gofiber/fiber/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// newStatusRemapMiddleware returns a Fiber handler that rewrites the outgoing
// HTTP status code based on a static map declared in config. The handler runs
// after the workflow's response has been set; unmapped statuses pass through.
func newStatusRemapMiddleware(cfg map[string]any, rootConfig map[string]any) (fiber.Handler, error) {
	if cfg == nil {
		return nil, fmt.Errorf("response.status_remap: config with \"map\" is required")
	}

	raw, ok := cfg["map"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("response.status_remap: \"map\" is required and must be an object")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("response.status_remap: \"map\" must not be empty")
	}

	parsed := make(map[int]int, len(raw))
	for k, v := range raw {
		fromCode, err := strconv.Atoi(k)
		if err != nil {
			return nil, fmt.Errorf("response.status_remap: map key %q is not a numeric HTTP status code", k)
		}
		if fromCode < 100 || fromCode > 599 {
			return nil, fmt.Errorf("response.status_remap: map key %q is out of range (must be 100-599)", k)
		}

		toCode, err := toStatusCode(v)
		if err != nil {
			return nil, fmt.Errorf("response.status_remap: map[%q]: %w", k, err)
		}
		if toCode < 100 || toCode > 599 {
			return nil, fmt.Errorf("response.status_remap: map[%q] value %d is out of range (must be 100-599)", k, toCode)
		}
		if fromCode == toCode {
			return nil, fmt.Errorf("response.status_remap: map[%q] maps to itself; remove this entry", k)
		}
		parsed[fromCode] = toCode
	}

	m, _ := rootConfig["_metrics"].(*metrics.Metrics)

	return func(c fiber.Ctx) error {
		if err := c.Next(); err != nil {
			return err
		}
		from := c.Response().StatusCode()
		to, hit := parsed[from]
		if !hit {
			return nil
		}
		c.Status(to)
		if m != nil && m.StatusRemaps != nil {
			m.StatusRemaps.Add(c.Context(), 1, metric.WithAttributes(
				attribute.String("from", strconv.Itoa(from)),
				attribute.String("to", strconv.Itoa(to)),
			))
		}
		return nil
	}, nil
}

// toStatusCode accepts JSON numeric values (float64) or int-like values and
// returns the code as an int. Returns an error for any other type.
func toStatusCode(v any) (int, error) {
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, fmt.Errorf("value %v is not an integer", n)
		}
		return int(n), nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("value must be an integer, got %T", v)
	}
}
