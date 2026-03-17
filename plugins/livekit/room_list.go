package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type roomListDescriptor struct{}

func (d *roomListDescriptor) Name() string        { return "roomList" }
func (d *roomListDescriptor) Description() string { return "Lists LiveKit rooms" }
func (d *roomListDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *roomListDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"names": map[string]any{"type": "array", "description": "Optional room name filter", "items": map[string]any{"type": "string"}},
		},
	}
}
func (d *roomListDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "List of room objects",
		"error":   "Failed to list rooms",
	}
}

type roomListExecutor struct{}

func newRoomListExecutor(_ map[string]any) api.NodeExecutor { return &roomListExecutor{} }

func (e *roomListExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *roomListExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomList: %w", err)
	}

	req := &lkproto.ListRoomsRequest{}

	if names, err := plugin.ResolveOptionalArray(nCtx, config, "names"); err != nil {
		return "", nil, fmt.Errorf("lk.roomList: %w", err)
	} else {
		for _, n := range names {
			if s, ok := n.(string); ok {
				req.Names = append(req.Names, s)
			}
		}
	}

	resp, err := svc.Room.ListRooms(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomList: %w", err)
	}

	rooms := make([]any, len(resp.Rooms))
	for i, r := range resp.Rooms {
		rooms[i] = roomToMap(r)
	}

	return api.OutputSuccess, map[string]any{"rooms": rooms}, nil
}
