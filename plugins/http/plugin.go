package http

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chimpanze/noda/internal/breaker"
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

	client := &http.Client{Timeout: timeout}

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
