package breaker

import (
	"fmt"
	"time"

	"github.com/sony/gobreaker/v2"
)

// Config holds circuit breaker settings parsed from service config.
type Config struct {
	MaxRequests uint32        // max requests in half-open state (default 5)
	Interval    time.Duration // cyclic period for clearing counts in closed state (default 60s)
	Timeout     time.Duration // period of open state before half-open (default 30s)
	Threshold   uint32        // consecutive failures to trip (default 3)
}

// New creates a gobreaker CircuitBreaker from config.
func New(name string, cfg Config) *gobreaker.CircuitBreaker[any] {
	// Apply defaults
	if cfg.MaxRequests == 0 {
		cfg.MaxRequests = 5
	}
	if cfg.Interval == 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Threshold == 0 {
		cfg.Threshold = 3
	}

	return gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:        fmt.Sprintf("breaker:%s", name),
		MaxRequests: cfg.MaxRequests,
		Interval:    cfg.Interval,
		Timeout:     cfg.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= cfg.Threshold
		},
	})
}

// ParseConfig extracts circuit breaker config from a service config map.
// Returns nil if no circuit_breaker key is present.
func ParseConfig(cfg map[string]any) *Config {
	cb, ok := cfg["circuit_breaker"].(map[string]any)
	if !ok {
		return nil
	}
	c := &Config{}
	if v, ok := cb["max_requests"].(float64); ok {
		c.MaxRequests = uint32(v)
	}
	if v, ok := cb["interval"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			c.Interval = d
		}
	}
	if v, ok := cb["timeout"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			c.Timeout = d
		}
	}
	if v, ok := cb["threshold"].(float64); ok {
		c.Threshold = uint32(v)
	}
	return c
}
