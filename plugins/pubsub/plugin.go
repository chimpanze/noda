package pubsub

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

// Plugin implements the Redis PubSub plugin.
type Plugin struct{}

func (p *Plugin) Name() string   { return "pubsub" }
func (p *Plugin) Prefix() string { return "pubsub" }

func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration { return nil }

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	client, err := plugin.NewRedisClient(config, "pubsub")
	if err != nil {
		return nil, err
	}
	return &Service{client: client}, nil
}

// ServiceConfigSchema documents the pubsub service `config` block, read by
// internal/plugin.NewRedisClient. additionalProperties is false: unknown
// keys are silently ignored by NewRedisClient/redis.ParseURL.
func (p *Plugin) ServiceConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url": map[string]any{
				"type":        "string",
				"description": "Redis connection URL (redis://...); required — NewRedisClient errors without it",
			},
			"pool_size": map[string]any{
				"type":        "integer",
				"description": "Maximum number of Redis connections in the pool",
			},
			"min_idle": map[string]any{
				"type":        "integer",
				"description": "Minimum number of idle Redis connections to keep open",
			},
		},
		"required":             []any{"url"},
		"additionalProperties": false,
	}
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
