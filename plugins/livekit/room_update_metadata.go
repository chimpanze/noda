package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type roomUpdateMetadataDescriptor struct{}

func (d *roomUpdateMetadataDescriptor) Name() string { return "roomUpdateMetadata" }
func (d *roomUpdateMetadataDescriptor) Description() string {
	return "Updates metadata on a LiveKit room"
}
func (d *roomUpdateMetadataDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *roomUpdateMetadataDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room":     map[string]any{"type": "string", "description": "Room name"},
			"metadata": map[string]any{"type": "string", "description": "New metadata value"},
		},
		"required": []any{"room", "metadata"},
	}
}
func (d *roomUpdateMetadataDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Updated room object",
		"error":   "Failed to update room metadata",
	}
}

type roomUpdateMetadataExecutor struct{}

func newRoomUpdateMetadataExecutor(_ map[string]any) api.NodeExecutor {
	return &roomUpdateMetadataExecutor{}
}

func (e *roomUpdateMetadataExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *roomUpdateMetadataExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomUpdateMetadata: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomUpdateMetadata: %w", err)
	}

	metadata, err := plugin.ResolveString(nCtx, config, "metadata")
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomUpdateMetadata: %w", err)
	}

	updated, err := svc.Room.UpdateRoomMetadata(ctx, &lkproto.UpdateRoomMetadataRequest{
		Room:     room,
		Metadata: metadata,
	})
	if err != nil {
		return "", nil, fmt.Errorf("lk.roomUpdateMetadata: %w", err)
	}

	return api.OutputSuccess, roomToMap(updated), nil
}
