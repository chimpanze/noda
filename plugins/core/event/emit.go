package event

import (
	"context"
	"fmt"

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

func (e *emitExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *emitExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	mode, _ := config["mode"].(string)
	if mode == "" {
		return "", nil, fmt.Errorf("event.emit: missing 'mode'")
	}

	topicRaw, ok := config["topic"]
	if !ok {
		return "", nil, fmt.Errorf("event.emit: missing 'topic'")
	}
	topicExpr, ok := topicRaw.(string)
	if !ok {
		return "", nil, fmt.Errorf("event.emit: 'topic' must be a string")
	}
	topicVal, err := nCtx.Resolve(topicExpr)
	if err != nil {
		return "", nil, fmt.Errorf("event.emit: resolve topic: %w", err)
	}
	topic, ok := topicVal.(string)
	if !ok {
		return "", nil, fmt.Errorf("event.emit: topic resolved to %T, expected string", topicVal)
	}

	payloadRaw, ok := config["payload"]
	if !ok {
		return "", nil, fmt.Errorf("event.emit: missing 'payload'")
	}
	var payload any
	if expr, ok := payloadRaw.(string); ok {
		payload, err = nCtx.Resolve(expr)
		if err != nil {
			return "", nil, fmt.Errorf("event.emit: resolve payload: %w", err)
		}
	} else {
		payload = payloadRaw
	}

	switch mode {
	case "stream":
		svc, ok := services["stream"]
		if !ok {
			return "", nil, fmt.Errorf("event.emit: stream service not configured")
		}
		streamSvc, ok := svc.(api.StreamService)
		if !ok {
			return "", nil, fmt.Errorf("event.emit: service does not implement StreamService")
		}
		msgID, err := streamSvc.Publish(ctx, topic, payload)
		if err != nil {
			return "", nil, fmt.Errorf("event.emit: %w", err)
		}
		return "success", map[string]any{"message_id": msgID}, nil

	case "pubsub":
		svc, ok := services["pubsub"]
		if !ok {
			return "", nil, fmt.Errorf("event.emit: pubsub service not configured")
		}
		pubsubSvc, ok := svc.(api.PubSubService)
		if !ok {
			return "", nil, fmt.Errorf("event.emit: service does not implement PubSubService")
		}
		if err := pubsubSvc.Publish(ctx, topic, payload); err != nil {
			return "", nil, fmt.Errorf("event.emit: %w", err)
		}
		return "success", map[string]any{"ok": true}, nil

	default:
		return "", nil, fmt.Errorf("event.emit: unknown mode %q", mode)
	}
}
