package http

import (
	"net/http"
	"time"

	"github.com/sony/gobreaker/v2"
)

// Service wraps net/http.Client with default configuration.
type Service struct {
	client         *http.Client
	baseURL        string
	defaultHeaders map[string]string
	defaultTimeout time.Duration
	breaker        *gobreaker.CircuitBreaker[any]
}
