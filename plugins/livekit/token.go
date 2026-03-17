package livekit

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/livekit/protocol/auth"
)

type tokenDescriptor struct{}

func (d *tokenDescriptor) Name() string        { return "token" }
func (d *tokenDescriptor) Description() string { return "Generates a LiveKit access token with grants" }
func (d *tokenDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *tokenDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"identity": map[string]any{"type": "string", "description": "Participant identity"},
			"room":     map[string]any{"type": "string", "description": "Room name to grant access to"},
			"name":     map[string]any{"type": "string", "description": "Participant display name"},
			"metadata": map[string]any{"type": "string", "description": "Participant metadata"},
			"ttl":      map[string]any{"type": "string", "description": "Token time-to-live (default: 6h)"},
			"grants":   map[string]any{"type": "object", "description": "Map of grant booleans (roomJoin, canPublish, etc.)"},
		},
		"required": []any{"identity", "room"},
	}
}
func (d *tokenDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "JWT token string with identity and room",
		"error":   "Token generation failed",
	}
}

type tokenExecutor struct{}

func newTokenExecutor(_ map[string]any) api.NodeExecutor { return &tokenExecutor{} }

func (e *tokenExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *tokenExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.token: %w", err)
	}

	identity, err := plugin.ResolveString(nCtx, config, "identity")
	if err != nil {
		return "", nil, fmt.Errorf("lk.token: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.token: %w", err)
	}

	ttlStr := "6h"
	if v, ok, err := plugin.ResolveOptionalString(nCtx, config, "ttl"); err != nil {
		return "", nil, fmt.Errorf("lk.token: %w", err)
	} else if ok {
		ttlStr = v
	}

	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return "", nil, fmt.Errorf("lk.token: invalid ttl %q: %w", ttlStr, err)
	}

	vg := &auth.VideoGrant{
		RoomJoin: true,
		Room:     room,
	}

	if raw, ok := config["grants"]; ok {
		if grantsMap, ok := raw.(map[string]any); ok {
			applyGrants(grantsMap, vg)
		}
	}

	at := auth.NewAccessToken(svc.APIKey, svc.APISecret).
		SetIdentity(identity).
		SetValidFor(ttl).
		SetVideoGrant(vg)

	if name, ok, err := plugin.ResolveOptionalString(nCtx, config, "name"); err != nil {
		return "", nil, fmt.Errorf("lk.token: %w", err)
	} else if ok {
		at.SetName(name)
	}

	if metadata, ok, err := plugin.ResolveOptionalString(nCtx, config, "metadata"); err != nil {
		return "", nil, fmt.Errorf("lk.token: %w", err)
	} else if ok {
		at.SetMetadata(metadata)
	}

	token, err := at.ToJWT()
	if err != nil {
		return "", nil, fmt.Errorf("lk.token: generate JWT: %w", err)
	}

	return api.OutputSuccess, map[string]any{
		"token":    token,
		"identity": identity,
		"room":     room,
	}, nil
}
