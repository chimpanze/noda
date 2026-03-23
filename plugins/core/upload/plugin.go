package upload

import "github.com/chimpanze/noda/pkg/api"

// Plugin registers the upload.handle node.
type Plugin struct{}

func (p *Plugin) Name() string   { return "core.upload" }
func (p *Plugin) Prefix() string { return "upload" }

func (p *Plugin) HasServices() bool { return false }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &handleDescriptor{}, Factory: newHandleExecutor},
	}
}

func (p *Plugin) CreateService(_ map[string]any) (any, error) { return nil, nil }
func (p *Plugin) HealthCheck(_ any) error                     { return nil }
func (p *Plugin) Shutdown(_ any) error                        { return nil }
