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

func (d *participantUpdateDescriptor) Name() string { return "participant_update" }
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
		return "", nil, fmt.Errorf("lk.participant_update: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.participant_update: %w", err)
	}

	identity, err := plugin.ResolveString(nCtx, config, "identity")
	if err != nil {
		return "", nil, fmt.Errorf("lk.participant_update: %w", err)
	}

	req := &lkproto.UpdateParticipantRequest{
		Room:     room,
		Identity: identity,
	}

	if metadata, ok, err := plugin.ResolveOptionalString(nCtx, config, "metadata"); err != nil {
		return "", nil, fmt.Errorf("lk.participant_update: %w", err)
	} else if ok {
		req.Metadata = metadata
	}

	if perms, err := plugin.ResolveOptionalMap(nCtx, config, "permissions"); err != nil {
		return "", nil, fmt.Errorf("lk.participant_update: %w", err)
	} else if len(perms) > 0 {
		// empty {} would otherwise cost a GetParticipant + full-replace
		// Permission send of unchanged values
		perm, err := mergedPermissions(ctx, svc, room, identity, perms)
		if err != nil {
			return "", nil, fmt.Errorf("lk.participant_update: %w", err)
		}
		req.Permission = perm
	}

	p, err := svc.Room.UpdateParticipant(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.participant_update: %w", err)
	}

	return api.OutputSuccess, participantToMap(p), nil
}

// permissionSetters is the single source of truth for the boolean permission
// keys lk.participant_update accepts — validation and overlay both iterate it,
// so a new key cannot be added to one and forgotten in the other.
var permissionSetters = map[string]func(*lkproto.ParticipantPermission, bool){
	"canPublish":     func(p *lkproto.ParticipantPermission, v bool) { p.CanPublish = v },
	"canSubscribe":   func(p *lkproto.ParticipantPermission, v bool) { p.CanSubscribe = v },
	"canPublishData": func(p *lkproto.ParticipantPermission, v bool) { p.CanPublishData = v },
	"hidden":         func(p *lkproto.ParticipantPermission, v bool) { p.Hidden = v },
	"recorder": func(p *lkproto.ParticipantPermission, v bool) {
		p.Recorder = v //nolint:staticcheck // no replacement available in ParticipantPermission; ParticipantInfo.kind is not settable here
	},
}

// mergedPermissions fetches the participant's current permission set and
// overlays the boolean keys present in perms. LiveKit's
// UpdateParticipantRequest.Permission is a full replace: sending only the
// changed fields would silently revoke every omitted permission. The read
// and the update are two RPCs, so a concurrent permission change between
// them can be lost.
func mergedPermissions(ctx context.Context, svc *Service, room, identity string, perms map[string]any) (*lkproto.ParticipantPermission, error) {
	for key, val := range perms {
		if _, known := permissionSetters[key]; !known {
			return nil, fmt.Errorf("unknown permission key %q", key)
		}
		if _, ok := val.(bool); !ok {
			return nil, fmt.Errorf("permission key %q must be a boolean, got %T", key, val)
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
	for key, val := range perms {
		permissionSetters[key](perm, val.(bool))
	}
	return perm, nil
}
