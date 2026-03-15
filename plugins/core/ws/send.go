package ws

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

var wsServiceDeps = map[string]api.ServiceDep{
	"connections": {Prefix: "ws", Required: true},
}

type sendDescriptor struct{}

func (d *sendDescriptor) Name() string { return "send" }
func (d *sendDescriptor) Description() string {
	return "Sends data to WebSocket connections on a channel"
}
func (d *sendDescriptor) ServiceDeps() map[string]api.ServiceDep { return wsServiceDeps }
func (d *sendDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"channel": map[string]any{"type": "string", "description": "WebSocket channel name"},
			"data":    map[string]any{"description": "Data to send to connected clients"},
		},
		"required": []any{"channel", "data"},
	}
}
func (d *sendDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "null (message sent to WebSocket connections)",
		"error":   "Send error",
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
		return "", nil, fmt.Errorf("ws.send: %w", err)
	}

	data, err := plugin.ResolveDeepAny(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("ws.send: %w", err)
	}

	if err := svc.Send(ctx, channel, data); err != nil {
		return "", nil, fmt.Errorf("ws.send: %w", err)
	}

	return api.OutputSuccess, map[string]any{"channel": channel}, nil
}
