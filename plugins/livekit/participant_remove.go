package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type participantRemoveDescriptor struct{}

func (d *participantRemoveDescriptor) Name() string { return "participantRemove" }
func (d *participantRemoveDescriptor) Description() string {
	return "Removes a participant from a room"
}
func (d *participantRemoveDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *participantRemoveDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room":     map[string]any{"type": "string", "description": "Room name"},
			"identity": map[string]any{"type": "string", "description": "Participant identity"},
		},
		"required": []any{"room", "identity"},
	}
}
func (d *participantRemoveDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Participant removed confirmation",
		"error":   "Failed to remove participant",
	}
}

type participantRemoveExecutor struct{}

func newParticipantRemoveExecutor(_ map[string]any) api.NodeExecutor {
	return &participantRemoveExecutor{}
}

func (e *participantRemoveExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *participantRemoveExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantRemove: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantRemove: %w", err)
	}

	identity, err := plugin.ResolveString(nCtx, config, "identity")
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantRemove: %w", err)
	}

	_, err = svc.Room.RemoveParticipant(ctx, &lkproto.RoomParticipantIdentity{
		Room:     room,
		Identity: identity,
	})
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantRemove: %w", err)
	}

	return api.OutputSuccess, map[string]any{"removed": true}, nil
}
