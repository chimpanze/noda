package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type egressStartRoomCompositeDescriptor struct{}

func (d *egressStartRoomCompositeDescriptor) Name() string { return "egressStartRoomComposite" }
func (d *egressStartRoomCompositeDescriptor) Description() string {
	return "Starts a room composite egress (recording)"
}
func (d *egressStartRoomCompositeDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *egressStartRoomCompositeDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room":       map[string]any{"type": "string", "description": "Room name to record"},
			"layout":     map[string]any{"type": "string", "description": "Layout template (default: speaker-dark)"},
			"audio_only": map[string]any{"type": "boolean", "description": "Record audio only"},
			"output":     map[string]any{"type": "object", "description": "Output config (type, bucket, filepath, etc.)"},
		},
		"required": []any{"room", "output"},
	}
}
func (d *egressStartRoomCompositeDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Egress info with egress_id",
		"error":   "Failed to start egress",
	}
}

type egressStartRoomCompositeExecutor struct{}

func newEgressStartRoomCompositeExecutor(_ map[string]any) api.NodeExecutor {
	return &egressStartRoomCompositeExecutor{}
}

func (e *egressStartRoomCompositeExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *egressStartRoomCompositeExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStartRoomComposite: %w", err)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStartRoomComposite: %w", err)
	}

	fileOutput, err := buildFileOutput(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStartRoomComposite: %w", err)
	}

	layout := "speaker-dark"
	if v, ok, err := plugin.ResolveOptionalString(nCtx, config, "layout"); err != nil {
		return "", nil, fmt.Errorf("lk.egressStartRoomComposite: %w", err)
	} else if ok {
		layout = v
	}

	req := &lkproto.RoomCompositeEgressRequest{
		RoomName: room,
		Layout:   layout,
		Output: &lkproto.RoomCompositeEgressRequest_File{
			File: fileOutput,
		},
	}

	if audioOnlyRaw, ok, err := plugin.ResolveOptionalAny(nCtx, config, "audio_only"); err != nil {
		return "", nil, fmt.Errorf("lk.egressStartRoomComposite: %w", err)
	} else if ok {
		if v, ok := audioOnlyRaw.(bool); ok {
			req.AudioOnly = v
		}
	}

	info, err := svc.Egress.StartRoomCompositeEgress(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStartRoomComposite: %w", err)
	}

	return api.OutputSuccess, egressInfoToMap(info), nil
}
