package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type roomDeleteDescriptor struct{}

func (d *roomDeleteDescriptor) Name() string        { return "roomDelete" }
func (d *roomDeleteDescriptor) Description() string { return "Deletes a LiveKit room" }
func (d *roomDeleteDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *roomDeleteDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room": map[string]any{"type": "string", "description": "Room name to delete"},
		},
		"required": []any{"room"},
	}
}
func (d *roomDeleteDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Room deleted confirmation",
		"error":   "Failed to delete room",
	}
}

type roomDeleteExecutor struct{}

func newRoomDeleteExecutor(_ map[string]any) api.NodeExecutor { return &roomDeleteExecutor{} }

func (e *roomDeleteExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *roomDeleteExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomDelete: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomDelete: %w", err)
	}

	_, err = svc.Room.DeleteRoom(ctx, &lkproto.DeleteRoomRequest{Room: room})
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomDelete: %w", err)
	}

	return api.OutputSuccess, map[string]any{"deleted": true}, nil
}
