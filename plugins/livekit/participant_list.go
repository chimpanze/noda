package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type participantListDescriptor struct{}

func (d *participantListDescriptor) Name() string        { return "participantList" }
func (d *participantListDescriptor) Description() string { return "Lists participants in a room" }
func (d *participantListDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *participantListDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room": map[string]any{"type": "string", "description": "Room name"},
		},
		"required": []any{"room"},
	}
}
func (d *participantListDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "List of participant objects",
		"error":   "Failed to list participants",
	}
}

type participantListExecutor struct{}

func newParticipantListExecutor(_ map[string]any) api.NodeExecutor {
	return &participantListExecutor{}
}

func (e *participantListExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *participantListExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantList: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantList: %w", err)
	}

	resp, err := svc.Room.ListParticipants(ctx, &lkproto.ListParticipantsRequest{Room: room})
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantList: %w", err)
	}

	participants := make([]any, len(resp.Participants))
	for i, p := range resp.Participants {
		participants[i] = participantToMap(p)
	}

	return api.OutputSuccess, map[string]any{"participants": participants}, nil
}

func participantToMap(p *lkproto.ParticipantInfo) map[string]any {
	return map[string]any{
		"sid":       p.Sid,
		"identity":  p.Identity,
		"name":      p.Name,
		"metadata":  p.Metadata,
		"state":     p.State.String(),
		"joined_at": p.JoinedAt,
		"region":    p.Region,
	}
}
