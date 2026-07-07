package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
	"google.golang.org/protobuf/proto"
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
			"permissions": map[string]any{"type": "object", "description": "Permission overrides (merged with the participant's current permissions; unknown or non-boolean keys are rejected)"},
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
		perm, err := mergedPermissions(ctx, svc, room, identity, perms)
		if err != nil {
			return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
		}
		req.Permission = perm
	}

	p, err := svc.Room.UpdateParticipant(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
	}

	return api.OutputSuccess, participantToMap(p), nil
}

// mergedPermissions fetches the participant's current permission set and
// overlays the boolean keys present in perms. LiveKit's
// UpdateParticipantRequest.Permission is a full replace: sending only the
// changed fields would silently revoke every omitted permission. The read
// and the update are two RPCs, so a concurrent permission change between
// them can be lost.
func mergedPermissions(ctx context.Context, svc *Service, room, identity string, perms map[string]any) (*lkproto.ParticipantPermission, error) {
	for key, val := range perms {
		switch key {
		case "canPublish", "canSubscribe", "canPublishData", "hidden", "recorder":
			if _, ok := val.(bool); !ok {
				return nil, fmt.Errorf("permission key %q must be a boolean, got %T", key, val)
			}
		default:
			return nil, fmt.Errorf("unknown permission key %q", key)
		}
	}

	current, err := svc.Room.GetParticipant(ctx, &lkproto.RoomParticipantIdentity{Room: room, Identity: identity})
	if err != nil {
		return nil, fmt.Errorf("get current permissions: %w", err)
	}
	perm := &lkproto.ParticipantPermission{}
	if p := current.GetPermission(); p != nil {
		perm = proto.Clone(p).(*lkproto.ParticipantPermission)
	}
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
	return perm, nil
}
