package http

import (
	"net/http"
	"time"
)

// Service wraps net/http.Client with default configuration.
type Service struct {
	client         *http.Client
	baseURL        string
	defaultHeaders map[string]string
	defaultTimeout time.Duration
}
