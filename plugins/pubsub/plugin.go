package pubsub

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/redis/go-redis/v9"
)

// Plugin implements the Redis PubSub plugin.
type Plugin struct{}

func (p *Plugin) Name() string   { return "pubsub" }
func (p *Plugin) Prefix() string { return "pubsub" }

func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration { return nil }

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	url, _ := config["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("pubsub: missing 'url'")
	}

	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("pubsub: parse url: %w", err)
	}

	client := redis.NewClient(opts)
	return &Service{client: client}, nil
}

func (p *Plugin) HealthCheck(service any) error {
	svc, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("pubsub: invalid service type")
	}
	return svc.client.Ping(context.Background()).Err()
}

func (p *Plugin) Shutdown(service any) error {
	svc, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("pubsub: invalid service type")
	}
	return svc.client.Close()
}
