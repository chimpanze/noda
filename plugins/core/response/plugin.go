package response

import "github.com/chimpanze/noda/pkg/api"

// Plugin is the core response plugin (response.json, response.redirect, response.error).
type Plugin struct{}

func (p *Plugin) Name() string   { return "core.response" }
func (p *Plugin) Prefix() string { return "response" }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &jsonDescriptor{}, Factory: newJSONExecutor},
		{Descriptor: &redirectDescriptor{}, Factory: newRedirectExecutor},
		{Descriptor: &errorDescriptor{}, Factory: newErrorExecutor},
	}
}

func (p *Plugin) HasServices() bool                                { return false }
func (p *Plugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(service any) error                    { return nil }
func (p *Plugin) Shutdown(service any) error                       { return nil }
