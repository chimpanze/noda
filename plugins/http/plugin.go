package http

import (
	"fmt"
	"net/http"
	"time"

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

	var defaultHeaders map[string]string
	if hdrs, ok := config["headers"].(map[string]any); ok {
		defaultHeaders = make(map[string]string, len(hdrs))
		for k, v := range hdrs {
			if s, ok := v.(string); ok {
				defaultHeaders[k] = s
			}
		}
	}

	client := &http.Client{Timeout: timeout}

	return &Service{
		client:         client,
		baseURL:        baseURL,
		defaultHeaders: defaultHeaders,
		defaultTimeout: timeout,
	}, nil
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
