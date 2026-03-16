package livekit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type sendDataDescriptor struct{}

func (d *sendDataDescriptor) Name() string        { return "sendData" }
func (d *sendDataDescriptor) Description() string { return "Sends data to participants in a room" }
func (d *sendDataDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *sendDataDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room":                     map[string]any{"type": "string", "description": "Room name"},
			"data":                     map[string]any{"description": "Data to send (string or object, serialized as JSON)"},
			"kind":                     map[string]any{"type": "string", "description": "Delivery kind: reliable or lossy (default: reliable)"},
			"destination_identities":   map[string]any{"type": "array", "description": "Target participant identities", "items": map[string]any{"type": "string"}},
			"topic":                    map[string]any{"type": "string", "description": "Optional topic for the data message"},
		},
		"required": []any{"room", "data"},
	}
}
func (d *sendDataDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Data sent confirmation",
		"error":   "Failed to send data",
	}
}

type sendDataExecutor struct{}

func newSendDataExecutor(_ map[string]any) api.NodeExecutor { return &sendDataExecutor{} }

func (e *sendDataExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *sendDataExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.sendData: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.sendData: %w", err)
	}

	dataRaw, err := plugin.ResolveAny(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("lk.sendData: %w", err)
	}

	var dataBytes []byte
	switch v := dataRaw.(type) {
	case string:
		dataBytes = []byte(v)
	default:
		dataBytes, err = json.Marshal(v)
		if err != nil {
			return "", nil, fmt.Errorf("lk.sendData: marshal data: %w", err)
		}
	}

	kind := lkproto.DataPacket_RELIABLE
	if kindStr, ok, err := plugin.ResolveOptionalString(nCtx, config, "kind"); err != nil {
		return "", nil, fmt.Errorf("lk.sendData: %w", err)
	} else if ok && kindStr == "lossy" {
		kind = lkproto.DataPacket_LOSSY
	}

	req := &lkproto.SendDataRequest{
		Room: room,
		Data: dataBytes,
		Kind: kind,
	}

	if identities, err := plugin.ResolveOptionalArray(nCtx, config, "destination_identities"); err != nil {
		return "", nil, fmt.Errorf("lk.sendData: %w", err)
	} else if identities != nil {
		for _, id := range identities {
			if s, ok := id.(string); ok {
				req.DestinationIdentities = append(req.DestinationIdentities, s)
			}
		}
	}

	if topic, ok, err := plugin.ResolveOptionalString(nCtx, config, "topic"); err != nil {
		return "", nil, fmt.Errorf("lk.sendData: %w", err)
	} else if ok {
		req.Topic = &topic
	}

	_, err = svc.Room.SendData(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.sendData: %w", err)
	}

	return api.OutputSuccess, map[string]any{"sent": true}, nil
}
