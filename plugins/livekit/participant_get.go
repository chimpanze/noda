package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type participantGetDescriptor struct{}

func (d *participantGetDescriptor) Name() string        { return "participantGet" }
func (d *participantGetDescriptor) Description() string { return "Gets a participant by identity" }
func (d *participantGetDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *participantGetDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room":     map[string]any{"type": "string", "description": "Room name"},
			"identity": map[string]any{"type": "string", "description": "Participant identity"},
		},
		"required": []any{"room", "identity"},
	}
}
func (d *participantGetDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Participant object",
		"error":   "Participant not found or request failed",
	}
}

type participantGetExecutor struct{}

func newParticipantGetExecutor(_ map[string]any) api.NodeExecutor {
	return &participantGetExecutor{}
}

func (e *participantGetExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *participantGetExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantGet: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantGet: %w", err)
	}

	identity, err := plugin.ResolveString(nCtx, config, "identity")
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantGet: %w", err)
	}

	p, err := svc.Room.GetParticipant(ctx, &lkproto.RoomParticipantIdentity{
		Room:     room,
		Identity: identity,
	})
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantGet: %w", err)
	}

	return api.OutputSuccess, participantToMap(p), nil
}
