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

func (d *sendDescriptor) Name() string                           { return "send" }
func (d *sendDescriptor) ServiceDeps() map[string]api.ServiceDep { return wsServiceDeps }
func (d *sendDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"channel": map[string]any{"type": "string"},
			"data":    map[string]any{},
		},
		"required": []any{"channel", "data"},
	}
}

type sendExecutor struct{}

func newSendExecutor(_ map[string]any) api.NodeExecutor { return &sendExecutor{} }

func (e *sendExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *sendExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[api.ConnectionService](services, "connections")
	if err != nil {
		return "", nil, err
	}

	channel, err := plugin.ResolveString(nCtx, config, "channel")
	if err != nil {
		return "", nil, fmt.Errorf("ws.send: %w", err)
	}

	data, _, err := plugin.ResolveOptionalAny(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("ws.send: %w", err)
	}

	if err := svc.Send(ctx, channel, data); err != nil {
		return "", nil, fmt.Errorf("ws.send: %w", err)
	}

	return "success", map[string]any{"channel": channel}, nil
}
