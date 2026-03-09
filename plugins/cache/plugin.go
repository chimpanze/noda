package cache

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/redis/go-redis/v9"
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
	url, _ := config["url"].(string)
	if url == "" {
		return nil, fmt.Errorf("cache: missing 'url'")
	}

	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("cache: parse url: %w", err)
	}

	// Pool settings
	if v, ok := plugin.ToInt(config["pool_size"]); ok {
		opts.PoolSize = v
	}
	if v, ok := plugin.ToInt(config["min_idle"]); ok {
		opts.MinIdleConns = v
	}

	client := redis.NewClient(opts)
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
