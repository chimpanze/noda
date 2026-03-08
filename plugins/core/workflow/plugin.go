package workflow

import "github.com/chimpanze/noda/pkg/api"

// Plugin is the core workflow plugin (workflow.run, workflow.output).
type Plugin struct{}

func (p *Plugin) Name() string   { return "core.workflow" }
func (p *Plugin) Prefix() string { return "workflow" }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &runDescriptor{}, Factory: newRunExecutor},
		{Descriptor: &outputDescriptor{}, Factory: newOutputExecutor},
	}
}

func (p *Plugin) HasServices() bool                                { return false }
func (p *Plugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(service any) error                    { return nil }
func (p *Plugin) Shutdown(service any) error                       { return nil }
