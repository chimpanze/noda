package storage

import "github.com/chimpanze/noda/pkg/api"

// Plugin registers all storage.* nodes.
type Plugin struct{}

func (p *Plugin) Name() string   { return "core.storage" }
func (p *Plugin) Prefix() string { return "storage" }

func (p *Plugin) HasServices() bool { return false }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &readDescriptor{}, Factory: newReadExecutor},
		{Descriptor: &writeDescriptor{}, Factory: newWriteExecutor},
		{Descriptor: &deleteDescriptor{}, Factory: newDeleteExecutor},
		{Descriptor: &listDescriptor{}, Factory: newListExecutor},
	}
}

func (p *Plugin) CreateService(_ map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(_ any) error                     { return nil }
func (p *Plugin) Shutdown(_ any) error                        { return nil }
