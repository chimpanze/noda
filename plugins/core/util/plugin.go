package util

import "github.com/chimpanze/noda/pkg/api"

// Plugin is the core utility plugin (util.log, util.uuid, etc.).
type Plugin struct{}

func (p *Plugin) Name() string   { return "core.util" }
func (p *Plugin) Prefix() string { return "util" }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &logDescriptor{}, Factory: newLogExecutor},
		{Descriptor: &uuidDescriptor{}, Factory: newUUIDExecutor},
		{Descriptor: &delayDescriptor{}, Factory: newDelayExecutor},
		{Descriptor: &timestampDescriptor{}, Factory: newTimestampExecutor},
	}
}

func (p *Plugin) HasServices() bool                                { return false }
func (p *Plugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(service any) error                    { return nil }
func (p *Plugin) Shutdown(service any) error                       { return nil }
