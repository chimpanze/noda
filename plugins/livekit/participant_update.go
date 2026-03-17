package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type participantUpdateDescriptor struct{}

func (d *participantUpdateDescriptor) Name() string { return "participantUpdate" }
func (d *participantUpdateDescriptor) Description() string {
	return "Updates a participant's metadata or permissions"
}
func (d *participantUpdateDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *participantUpdateDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room":        map[string]any{"type": "string", "description": "Room name"},
			"identity":    map[string]any{"type": "string", "description": "Participant identity"},
			"metadata":    map[string]any{"type": "string", "description": "New metadata value"},
			"permissions": map[string]any{"type": "object", "description": "Permission overrides"},
		},
		"required": []any{"room", "identity"},
	}
}
func (d *participantUpdateDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Updated participant object",
		"error":   "Failed to update participant",
	}
}

type participantUpdateExecutor struct{}

func newParticipantUpdateExecutor(_ map[string]any) api.NodeExecutor {
	return &participantUpdateExecutor{}
}

func (e *participantUpdateExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *participantUpdateExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
	}

	identity, err := plugin.ResolveString(nCtx, config, "identity")
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
	}

	req := &lkproto.UpdateParticipantRequest{
		Room:     room,
		Identity: identity,
	}

	if metadata, ok, err := plugin.ResolveOptionalString(nCtx, config, "metadata"); err != nil {
		return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
	} else if ok {
		req.Metadata = metadata
	}

	if perms, err := plugin.ResolveOptionalMap(nCtx, config, "permissions"); err != nil {
		return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
	} else if perms != nil {
		perm := &lkproto.ParticipantPermission{}
		if v, ok := perms["canPublish"].(bool); ok {
			perm.CanPublish = v
		}
		if v, ok := perms["canSubscribe"].(bool); ok {
			perm.CanSubscribe = v
		}
		if v, ok := perms["canPublishData"].(bool); ok {
			perm.CanPublishData = v
		}
		if v, ok := perms["hidden"].(bool); ok {
			perm.Hidden = v
		}
		if v, ok := perms["recorder"].(bool); ok {
			perm.Recorder = v //nolint:staticcheck // no replacement available in ParticipantPermission; ParticipantInfo.kind is not settable here
		}
		req.Permission = perm
	}

	p, err := svc.Room.UpdateParticipant(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
	}

	return api.OutputSuccess, participantToMap(p), nil
}
