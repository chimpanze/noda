package http

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/chimpanze/noda/internal/breaker"
	"github.com/chimpanze/noda/internal/netguard"
	"github.com/chimpanze/noda/pkg/api"
)

// Plugin implements the outbound HTTP client plugin.
type Plugin struct{}

func (p *Plugin) Name() string   { return "http" }
func (p *Plugin) Prefix() string { return "http" }

func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &requestDescriptor{}, Factory: newRequestExecutor},
		{Descriptor: &getDescriptor{}, Factory: newGetExecutor},
		{Descriptor: &postDescriptor{}, Factory: newPostExecutor},
	}
}

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	timeout := 30 * time.Second
	if v, ok := config["timeout"].(string); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("http: invalid timeout %q: %w", v, err)
		}
		timeout = d
	} else if v, ok := config["timeout"].(float64); ok {
		timeout = time.Duration(v) * time.Second
	}

	baseURL, _ := config["base_url"].(string)
	if baseURL != "" {
		if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
			return nil, fmt.Errorf("http: base_url must use http:// or https:// scheme, got %q", baseURL)
		}
	}

	var defaultHeaders map[string]string
	if hdrs, ok := config["headers"].(map[string]any); ok {
		defaultHeaders = make(map[string]string, len(hdrs))
		for k, v := range hdrs {
			if s, ok := v.(string); ok {
				defaultHeaders[k] = s
			}
		}
	}

	// --- Outbound network policy ---
	allowPrivate := false
	if v, ok := config["allow_private_networks"].(bool); ok {
		allowPrivate = v
	}

	var allowedHosts []string
	if raw, ok := config["allowed_hosts"]; ok {
		arr, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("http: allowed_hosts must be an array of strings")
		}
		for i, item := range arr {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("http: allowed_hosts[%d] must be a string", i)
			}
			if strings.Contains(s, "/") || strings.Contains(s, ":") || strings.Contains(s, "://") {
				return nil, fmt.Errorf("http: allowed_hosts[%d] %q must be a bare hostname (no scheme, no path, no port)", i, s)
			}
			if s == "" {
				return nil, fmt.Errorf("http: allowed_hosts[%d] is empty", i)
			}
			// IP literals never match in CheckHost (they short-circuit DNS); listing one
			// here silently has no effect. Reject to surface the mis-config.
			if net.ParseIP(s) != nil {
				return nil, fmt.Errorf("http: allowed_hosts[%d] %q is an IP literal; allowed_hosts applies to hostnames only. Use allow_private_networks: true if you need to reach private IPs", i, s)
			}
			allowedHosts = append(allowedHosts, s)
		}
	}

	redirectMode := "strip_auth"
	if v, ok := config["redirects"].(string); ok {
		switch v {
		case "none", "same_origin", "strip_auth":
			redirectMode = v
		default:
			return nil, fmt.Errorf("http: redirects must be one of \"none\", \"same_origin\", \"strip_auth\", got %q", v)
		}
	}

	maxRedirects := 10
	if raw, ok := config["max_redirects"]; ok {
		var n int
		switch v := raw.(type) {
		case float64:
			n = int(v)
		case int:
			n = v
		default:
			return nil, fmt.Errorf("http: max_redirects must be a number, got %T", raw)
		}
		if n < 0 || n > 50 {
			return nil, fmt.Errorf("http: max_redirects must be in [0, 50], got %d", n)
		}
		maxRedirects = n
	}

	policy := netguard.Policy{
		AllowPrivateNetworks: allowPrivate,
		AllowedHosts:         allowedHosts,
	}
	client := &http.Client{
		Timeout:       timeout,
		Transport:     newTransport(policy, nil),
		CheckRedirect: buildCheckRedirect(redirectMode, maxRedirects),
	}

	svc := &Service{
		client:               client,
		baseURL:              baseURL,
		defaultHeaders:       defaultHeaders,
		defaultTimeout:       timeout,
		allowPrivateNetworks: allowPrivate,
		allowedHosts:         allowedHosts,
		redirectMode:         redirectMode,
		maxRedirects:         maxRedirects,
	}

	if cbCfg := breaker.ParseConfig(config); cbCfg != nil {
		name, _ := config["name"].(string)
		if name == "" {
			name = "http"
		}
		svc.breaker = breaker.New(name, *cbCfg)
	}

	return svc, nil
}

// ServiceConfigSchema documents the http service `config` block. Every key
// here is read either directly by CreateService or by
// internal/breaker.ParseConfig (the nested circuit_breaker object).
// additionalProperties is false at both levels: unknown keys are silently
// ignored by both parsers.
func (p *Plugin) ServiceConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"timeout": map[string]any{
				"type":        []any{"string", "number"},
				"description": "Default request timeout: a Go duration string, or a number of seconds (default 30s)",
			},
			"base_url": map[string]any{
				"type":        "string",
				"description": "Base URL prepended to relative request paths; must use http:// or https://",
			},
			"headers": map[string]any{
				"type":                 "object",
				"description":          "Default headers sent with every request from this service",
				"additionalProperties": true,
			},
			"allow_private_networks": map[string]any{
				"type":        "boolean",
				"description": "Allow requests to private/loopback/link-local IPs (default false)",
			},
			"allowed_hosts": map[string]any{
				"type":        "array",
				"description": "Allowlist of bare hostnames this service may reach (no scheme, path, port, or IP literals)",
				"items":       map[string]any{"type": "string"},
			},
			"redirects": map[string]any{
				"type":        "string",
				"enum":        []any{"none", "same_origin", "strip_auth"},
				"description": "Redirect-following policy (default strip_auth)",
			},
			"max_redirects": map[string]any{
				"type":        "integer",
				"description": "Maximum redirects to follow, 0-50 (default 10)",
			},
			"name": map[string]any{
				"type":        "string",
				"description": "Circuit breaker name used in logs/metrics when circuit_breaker is set (default http)",
			},
			"circuit_breaker": map[string]any{
				"type":        "object",
				"description": "Enables a circuit breaker for this service's requests; omit to disable",
				"properties": map[string]any{
					"max_requests": map[string]any{"type": "integer", "description": "Max requests allowed through in the half-open state"},
					"interval":     map[string]any{"type": "string", "description": "Closed-state counter reset interval, as a Go duration"},
					"timeout":      map[string]any{"type": "string", "description": "Open-state duration before probing half-open, as a Go duration"},
					"threshold":    map[string]any{"type": "integer", "description": "Consecutive failures required to trip the breaker"},
				},
				"additionalProperties": false,
			},
		},
		"required":             []any{},
		"additionalProperties": false,
	}
}

func (p *Plugin) HealthCheck(service any) error {
	_, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("http: invalid service type")
	}
	return nil
}

func (p *Plugin) Shutdown(service any) error {
	svc, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("http: invalid service type")
	}
	svc.client.CloseIdleConnections()
	return nil
}
