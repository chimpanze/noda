package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type muteTrackDescriptor struct{}

func (d *muteTrackDescriptor) Name() string        { return "muteTrack" }
func (d *muteTrackDescriptor) Description() string { return "Mutes or unmutes a published track" }
func (d *muteTrackDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *muteTrackDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room":      map[string]any{"type": "string", "description": "Room name"},
			"identity":  map[string]any{"type": "string", "description": "Participant identity"},
			"track_sid": map[string]any{"type": "string", "description": "Track SID to mute/unmute"},
			"muted":     map[string]any{"type": "boolean", "description": "Whether to mute (true) or unmute (false)"},
		},
		"required": []any{"room", "identity", "track_sid", "muted"},
	}
}
func (d *muteTrackDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Track info after mute/unmute",
		"error":   "Failed to mute/unmute track",
	}
}

type muteTrackExecutor struct{}

func newMuteTrackExecutor(_ map[string]any) api.NodeExecutor { return &muteTrackExecutor{} }

func (e *muteTrackExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *muteTrackExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.muteTrack: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.muteTrack: %w", err)
	}

	identity, err := plugin.ResolveString(nCtx, config, "identity")
	if err != nil {
		return "", nil, fmt.Errorf("lk.muteTrack: %w", err)
	}

	trackSID, err := plugin.ResolveString(nCtx, config, "track_sid")
	if err != nil {
		return "", nil, fmt.Errorf("lk.muteTrack: %w", err)
	}

	mutedRaw, err := plugin.ResolveAny(nCtx, config, "muted")
	if err != nil {
		return "", nil, fmt.Errorf("lk.muteTrack: %w", err)
	}
	muted, ok := mutedRaw.(bool)
	if !ok {
		return "", nil, fmt.Errorf("lk.muteTrack: field \"muted\" must be a boolean")
	}

	resp, err := svc.Room.MutePublishedTrack(ctx, &lkproto.MuteRoomTrackRequest{
		Room:     room,
		Identity: identity,
		TrackSid: trackSID,
		Muted:    muted,
	})
	if err != nil {
		return "", nil, fmt.Errorf("lk.muteTrack: %w", err)
	}

	result := map[string]any{"muted": muted}
	if resp.Track != nil {
		result["track_sid"] = resp.Track.Sid
		result["track_name"] = resp.Track.Name
		result["track_type"] = resp.Track.Type.String()
	}

	return api.OutputSuccess, result, nil
}
