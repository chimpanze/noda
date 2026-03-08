package event

import "github.com/chimpanze/noda/pkg/api"

// Plugin provides the event.emit node.
type Plugin struct{}

func (p *Plugin) Name() string   { return "event" }
func (p *Plugin) Prefix() string { return "event" }

func (p *Plugin) HasServices() bool { return false }
func (p *Plugin) CreateService(_ map[string]any) (any, error) {
	return nil, nil
}
func (p *Plugin) HealthCheck(_ any) error { return nil }
func (p *Plugin) Shutdown(_ any) error    { return nil }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &emitDescriptor{}, Factory: newEmitExecutor},
	}
}
