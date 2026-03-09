package sse

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

var sseServiceDeps = map[string]api.ServiceDep{
	"connections": {Prefix: "sse", Required: true},
}

type sendDescriptor struct{}

func (d *sendDescriptor) Name() string                          { return "send" }
func (d *sendDescriptor) ServiceDeps() map[string]api.ServiceDep { return sseServiceDeps }
func (d *sendDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"channel": map[string]any{"type": "string"},
			"data":    map[string]any{},
			"event":   map[string]any{"type": "string"},
			"id":      map[string]any{"type": "string"},
		},
		"required": []any{"channel", "data"},
	}
}

type sendExecutor struct{}

func newSendExecutor(_ map[string]any) api.NodeExecutor { return &sendExecutor{} }

func (e *sendExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *sendExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := getConnectionService(services)
	if err != nil {
		return "", nil, err
	}

	channel, err := resolveRequiredString(nCtx, config, "channel")
	if err != nil {
		return "", nil, fmt.Errorf("sse.send: %w", err)
	}

	data, _, err := resolveAny(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("sse.send: %w", err)
	}

	event, _, _ := resolveString(nCtx, config, "event")
	id, _, _ := resolveString(nCtx, config, "id")

	if err := svc.SendSSE(ctx, channel, event, data, id); err != nil {
		return "", nil, fmt.Errorf("sse.send: %w", err)
	}

	return "success", map[string]any{"channel": channel}, nil
}

func getConnectionService(services map[string]any) (api.ConnectionService, error) {
	svc, ok := services["connections"]
	if !ok {
		return nil, fmt.Errorf("sse connection service not configured")
	}
	cs, ok := svc.(api.ConnectionService)
	if !ok {
		return nil, fmt.Errorf("service does not implement ConnectionService")
	}
	return cs, nil
}

func resolveRequiredString(nCtx api.ExecutionContext, config map[string]any, key string) (string, error) {
	raw, ok := config[key]
	if !ok {
		return "", fmt.Errorf("missing required field %q", key)
	}
	expr, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("field %q must be a string", key)
	}
	val, err := nCtx.Resolve(expr)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", key, err)
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("field %q resolved to %T, expected string", key, val)
	}
	return s, nil
}

func resolveString(nCtx api.ExecutionContext, config map[string]any, key string) (string, bool, error) {
	raw, ok := config[key]
	if !ok {
		return "", false, nil
	}
	expr, ok := raw.(string)
	if !ok {
		return "", false, fmt.Errorf("field %q must be a string", key)
	}
	val, err := nCtx.Resolve(expr)
	if err != nil {
		return "", false, fmt.Errorf("resolve %q: %w", key, err)
	}
	s, ok := val.(string)
	if !ok {
		return "", false, fmt.Errorf("field %q resolved to %T, expected string", key, val)
	}
	return s, true, nil
}

func resolveAny(nCtx api.ExecutionContext, config map[string]any, key string) (any, bool, error) {
	raw, ok := config[key]
	if !ok {
		return nil, false, nil
	}
	if expr, ok := raw.(string); ok {
		val, err := nCtx.Resolve(expr)
		if err != nil {
			return nil, false, fmt.Errorf("resolve %q: %w", key, err)
		}
		return val, true, nil
	}
	return raw, true, nil
}
