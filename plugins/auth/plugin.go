package auth

import (
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

// Plugin implements first-party authentication primitives.
type Plugin struct{}

func (p *Plugin) Name() string      { return "auth" }
func (p *Plugin) Prefix() string    { return "auth" }
func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &createUserDescriptor{}, Factory: newCreateUserExecutor},
		{Descriptor: &getUserDescriptor{}, Factory: newGetUserExecutor},
		{Descriptor: &verifyCredentialsDescriptor{}, Factory: newVerifyCredentialsExecutor},
		{Descriptor: &createSessionDescriptor{}, Factory: newCreateSessionExecutor},
		{Descriptor: &revokeSessionDescriptor{}, Factory: newRevokeSessionExecutor},
	}
}

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	return newService(config)
}

func (p *Plugin) HealthCheck(service any) error {
	if _, ok := service.(*Service); !ok {
		return fmt.Errorf("auth: invalid service type")
	}
	return nil
}

func (p *Plugin) Shutdown(any) error { return nil }
