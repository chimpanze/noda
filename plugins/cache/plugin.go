package cache

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

// Plugin implements the Redis cache plugin.
type Plugin struct{}

func (p *Plugin) Name() string   { return "cache" }
func (p *Plugin) Prefix() string { return "cache" }

func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration {
	return []api.NodeRegistration{
		{Descriptor: &getDescriptor{}, Factory: newGetExecutor},
		{Descriptor: &setDescriptor{}, Factory: newSetExecutor},
		{Descriptor: &delDescriptor{}, Factory: newDelExecutor},
		{Descriptor: &existsDescriptor{}, Factory: newExistsExecutor},
	}
}

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	client, err := plugin.NewRedisClient(config, "cache")
	if err != nil {
		return nil, err
	}
	return &Service{client: client}, nil
}

func (p *Plugin) HealthCheck(service any) error {
	svc, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("cache: invalid service type")
	}
	return svc.client.Ping(context.Background()).Err()
}

func (p *Plugin) Shutdown(service any) error {
	svc, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("cache: invalid service type")
	}
	return svc.client.Close()
}
