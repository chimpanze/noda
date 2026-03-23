package breaker

import (
	"errors"
	"testing"
	"time"

	"github.com/sony/gobreaker/v2"
)

func TestNew_Defaults(t *testing.T) {
	cb := New("test", Config{})

	// Should be able to execute a successful call
	result, err := cb.Execute(func() (any, error) {
		return "ok", nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result != "ok" {
		t.Fatalf("expected 'ok', got %v", result)
	}
}

func TestParseConfig_Present(t *testing.T) {
	cfg := map[string]any{
		"circuit_breaker": map[string]any{
			"max_requests": float64(10),
			"interval":     "120s",
			"timeout":      "45s",
			"threshold":    float64(5),
		},
	}

	c := ParseConfig(cfg)
	if c == nil {
		t.Fatal("expected config, got nil")
	}
	if c.MaxRequests != 10 {
		t.Errorf("max_requests: want 10, got %d", c.MaxRequests)
	}
	if c.Interval != 120*time.Second {
		t.Errorf("interval: want 120s, got %v", c.Interval)
	}
	if c.Timeout != 45*time.Second {
		t.Errorf("timeout: want 45s, got %v", c.Timeout)
	}
	if c.Threshold != 5 {
		t.Errorf("threshold: want 5, got %d", c.Threshold)
	}
}

func TestParseConfig_Missing(t *testing.T) {
	cfg := map[string]any{
		"base_url": "https://example.com",
	}
	c := ParseConfig(cfg)
	if c != nil {
		t.Fatalf("expected nil, got %+v", c)
	}
}

func TestCircuitBreaker_TripsAfterThreshold(t *testing.T) {
	cb := New("trip-test", Config{
		Threshold:   3,
		MaxRequests: 1,
		Timeout:     1 * time.Second,
	})

	errFail := errors.New("fail")

	// Execute 3 failing calls to trip the breaker
	for i := 0; i < 3; i++ {
		_, err := cb.Execute(func() (any, error) {
			return nil, errFail
		})
		if err == nil {
			t.Fatal("expected error from failing call")
		}
	}

	// Next call should be rejected by the open breaker
	_, err := cb.Execute(func() (any, error) {
		return "should not reach", nil
	})
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("expected ErrOpenState, got %v", err)
	}
}
