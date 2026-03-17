package oidc

import "github.com/chimpanze/noda/pkg/api"

// Plugin is the core OIDC plugin (oidc.auth_url, oidc.exchange, oidc.refresh).
type Plugin struct{}

func (p *Plugin) Name() string   { return "core.oidc" }
func (p *Plugin) Prefix() string { return "oidc" }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &authURLDescriptor{}, Factory: newAuthURLExecutor},
		{Descriptor: &exchangeDescriptor{}, Factory: newExchangeExecutor},
		{Descriptor: &refreshDescriptor{}, Factory: newRefreshExecutor},
	}
}

func (p *Plugin) HasServices() bool                                { return false }
func (p *Plugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(service any) error                    { return nil }
func (p *Plugin) Shutdown(service any) error                       { return nil }
