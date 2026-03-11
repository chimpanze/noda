package sse

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

var sseServiceDeps = map[string]api.ServiceDep{
	"connections": {Prefix: "sse", Required: true},
}

type sendDescriptor struct{}

func (d *sendDescriptor) Name() string                           { return "send" }
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

func (e *sendExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *sendExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[api.ConnectionService](services, "connections")
	if err != nil {
		return "", nil, err
	}

	channel, err := plugin.ResolveString(nCtx, config, "channel")
	if err != nil {
		return "", nil, fmt.Errorf("sse.send: %w", err)
	}

	data, err := plugin.ResolveDeepAny(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("sse.send: %w", err)
	}

	event, _, _ := plugin.ResolveOptionalString(nCtx, config, "event")
	id, _, _ := plugin.ResolveOptionalString(nCtx, config, "id")

	if err := svc.SendSSE(ctx, channel, event, data, id); err != nil {
		return "", nil, fmt.Errorf("sse.send: %w", err)
	}

	return api.OutputSuccess, map[string]any{"channel": channel}, nil
}
