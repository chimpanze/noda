package stream

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

// Plugin implements the Redis Streams plugin.
type Plugin struct{}

func (p *Plugin) Name() string   { return "stream" }
func (p *Plugin) Prefix() string { return "stream" }

func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration { return nil }

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	client, err := plugin.NewRedisClient(config, "stream")
	if err != nil {
		return nil, err
	}
	return &Service{client: client}, nil
}

func (p *Plugin) HealthCheck(service any) error {
	svc, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("stream: invalid service type")
	}
	return svc.client.Ping(context.Background()).Err()
}

func (p *Plugin) Shutdown(service any) error {
	svc, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("stream: invalid service type")
	}
	return svc.client.Close()
}
