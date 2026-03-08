package transform

import "github.com/chimpanze/noda/pkg/api"

// Plugin is the core transform plugin (transform.set, transform.map, etc.).
type Plugin struct{}

func (p *Plugin) Name() string   { return "core.transform" }
func (p *Plugin) Prefix() string { return "transform" }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &setDescriptor{}, Factory: newSetExecutor},
		{Descriptor: &mapDescriptor{}, Factory: newMapExecutor},
		{Descriptor: &filterDescriptor{}, Factory: newFilterExecutor},
		{Descriptor: &mergeDescriptor{}, Factory: newMergeExecutor},
		{Descriptor: &deleteDescriptor{}, Factory: newDeleteExecutor},
		{Descriptor: &validateDescriptor{}, Factory: newValidateExecutor},
	}
}

func (p *Plugin) HasServices() bool                                { return false }
func (p *Plugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(service any) error                    { return nil }
func (p *Plugin) Shutdown(service any) error                       { return nil }
