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

// ServiceConfigSchema documents the stream service `config` block, read by
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
				// plugin.ToInt accepts numeric strings too ($env() substitution
				// always produces strings), so both types must validate.
				"type":        []any{"string", "integer"},
				"description": "Maximum number of Redis connections in the pool, as an integer or numeric string ($env() always produces strings)",
			},
			"min_idle": map[string]any{
				// plugin.ToInt accepts numeric strings too ($env() substitution
				// always produces strings), so both types must validate.
				"type":        []any{"string", "integer"},
				"description": "Minimum number of idle Redis connections to keep open, as an integer or numeric string ($env() always produces strings)",
			},
		},
		"required":             []any{"url"},
		"additionalProperties": false,
	}
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
