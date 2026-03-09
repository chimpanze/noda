package event

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

type emitDescriptor struct{}

func (d *emitDescriptor) Name() string { return "emit" }
func (d *emitDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"stream": {Prefix: "stream", Required: false},
		"pubsub": {Prefix: "pubsub", Required: false},
	}
}
func (d *emitDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode":    map[string]any{"type": "string", "enum": []any{"stream", "pubsub"}},
			"topic":   map[string]any{"type": "string"},
			"payload": map[string]any{},
		},
		"required": []any{"mode", "topic", "payload"},
	}
}

type emitExecutor struct{}

func newEmitExecutor(_ map[string]any) api.NodeExecutor { return &emitExecutor{} }

func (e *emitExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *emitExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	mode, _ := config["mode"].(string)
	if mode == "" {
		return "", nil, fmt.Errorf("event.emit: missing 'mode'")
	}

	topic, err := plugin.ResolveString(nCtx, config, "topic")
	if err != nil {
		return "", nil, fmt.Errorf("event.emit: %w", err)
	}

	payload, err := plugin.ResolveAny(nCtx, config, "payload")
	if err != nil {
		return "", nil, fmt.Errorf("event.emit: %w", err)
	}

	switch mode {
	case "stream":
		streamSvc, err := plugin.GetService[api.StreamService](services, "stream")
		if err != nil {
			return "", nil, err
		}
		msgID, err := streamSvc.Publish(ctx, topic, payload)
		if err != nil {
			return "", nil, fmt.Errorf("event.emit: %w", err)
		}
		return api.OutputSuccess, map[string]any{"message_id": msgID}, nil

	case "pubsub":
		pubsubSvc, err := plugin.GetService[api.PubSubService](services, "pubsub")
		if err != nil {
			return "", nil, err
		}
		if err := pubsubSvc.Publish(ctx, topic, payload); err != nil {
			return "", nil, fmt.Errorf("event.emit: %w", err)
		}
		return api.OutputSuccess, map[string]any{"ok": true}, nil

	default:
		return "", nil, fmt.Errorf("event.emit: unknown mode %q", mode)
	}
}
