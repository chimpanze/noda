package control

import "github.com/chimpanze/noda/pkg/api"

// Plugin is the core control flow plugin (control.if, control.switch, control.loop).
type Plugin struct{}

func (p *Plugin) Name() string   { return "core.control" }
func (p *Plugin) Prefix() string { return "control" }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &ifDescriptor{}, Factory: newIfExecutor},
		{Descriptor: &switchDescriptor{}, Factory: newSwitchExecutor},
		{Descriptor: &loopDescriptor{}, Factory: newLoopExecutor},
	}
}

func (p *Plugin) HasServices() bool                                { return false }
func (p *Plugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(service any) error                    { return nil }
func (p *Plugin) Shutdown(service any) error                       { return nil }
