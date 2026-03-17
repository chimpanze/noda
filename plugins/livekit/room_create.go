package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type roomCreateDescriptor struct{}

func (d *roomCreateDescriptor) Name() string        { return "roomCreate" }
func (d *roomCreateDescriptor) Description() string { return "Creates a LiveKit room" }
func (d *roomCreateDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *roomCreateDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":             map[string]any{"type": "string", "description": "Room name"},
			"empty_timeout":    map[string]any{"type": "integer", "description": "Seconds before empty room is closed"},
			"max_participants": map[string]any{"type": "integer", "description": "Maximum number of participants"},
			"metadata":         map[string]any{"type": "string", "description": "Room metadata"},
		},
		"required": []any{"name"},
	}
}
func (d *roomCreateDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Created room object",
		"error":   "Room creation failed",
	}
}

type roomCreateExecutor struct{}

func newRoomCreateExecutor(_ map[string]any) api.NodeExecutor { return &roomCreateExecutor{} }

func (e *roomCreateExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *roomCreateExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomCreate: %w", err)
	}

	name, err := plugin.ResolveString(nCtx, config, "name")
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomCreate: %w", err)
	}

	req := &lkproto.CreateRoomRequest{Name: name}

	if v, ok, err := plugin.ResolveOptionalInt(nCtx, config, "empty_timeout"); err != nil {
		return "", nil, fmt.Errorf("lk.roomCreate: %w", err)
	} else if ok {
		req.EmptyTimeout = uint32(v)
	}

	if v, ok, err := plugin.ResolveOptionalInt(nCtx, config, "max_participants"); err != nil {
		return "", nil, fmt.Errorf("lk.roomCreate: %w", err)
	} else if ok {
		req.MaxParticipants = uint32(v)
	}

	if v, ok, err := plugin.ResolveOptionalString(nCtx, config, "metadata"); err != nil {
		return "", nil, fmt.Errorf("lk.roomCreate: %w", err)
	} else if ok {
		req.Metadata = v
	}

	room, err := svc.Room.CreateRoom(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomCreate: %w", err)
	}

	return api.OutputSuccess, roomToMap(room), nil
}

func roomToMap(r *lkproto.Room) map[string]any {
	return map[string]any{
		"sid":              r.Sid,
		"name":             r.Name,
		"empty_timeout":    r.EmptyTimeout,
		"max_participants": r.MaxParticipants,
		"metadata":         r.Metadata,
		"num_participants": r.NumParticipants,
		"creation_time":    r.CreationTime,
		"active_recording": r.ActiveRecording,
	}
}
