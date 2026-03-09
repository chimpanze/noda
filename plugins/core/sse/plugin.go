package sse

import "github.com/chimpanze/noda/pkg/api"

// Plugin provides SSE workflow nodes.
type Plugin struct{}

func (p *Plugin) Name() string      { return "sse" }
func (p *Plugin) Prefix() string    { return "sse" }
func (p *Plugin) HasServices() bool { return false }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &sendDescriptor{}, Factory: newSendExecutor},
	}
}

func (p *Plugin) CreateService(_ map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(_ any) error                     { return nil }
func (p *Plugin) Shutdown(_ any) error                        { return nil }
