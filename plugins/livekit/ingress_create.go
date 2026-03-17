package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type ingressCreateDescriptor struct{}

func (d *ingressCreateDescriptor) Name() string        { return "ingressCreate" }
func (d *ingressCreateDescriptor) Description() string { return "Creates a LiveKit ingress endpoint" }
func (d *ingressCreateDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *ingressCreateDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input_type":           map[string]any{"type": "string", "description": "Input type: rtmp, whip, or url"},
			"room":                 map[string]any{"type": "string", "description": "Room to publish into"},
			"participant_identity": map[string]any{"type": "string", "description": "Identity for the ingress participant"},
			"participant_name":     map[string]any{"type": "string", "description": "Display name for the ingress participant"},
			"url":                  map[string]any{"type": "string", "description": "Source URL (required for url input type)"},
		},
		"required": []any{"input_type", "room", "participant_identity"},
	}
}
func (d *ingressCreateDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Ingress info with ingress_id, url, and stream_key",
		"error":   "Failed to create ingress",
	}
}

type ingressCreateExecutor struct{}

func newIngressCreateExecutor(_ map[string]any) api.NodeExecutor { return &ingressCreateExecutor{} }

func (e *ingressCreateExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *ingressCreateExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.ingressCreate: %w", err)
	}

	inputTypeStr, err := plugin.ResolveString(nCtx, config, "input_type")
	if err != nil {
		return "", nil, fmt.Errorf("lk.ingressCreate: %w", err)
	}

	var inputType lkproto.IngressInput
	switch inputTypeStr {
	case "rtmp":
		inputType = lkproto.IngressInput_RTMP_INPUT
	case "whip":
		inputType = lkproto.IngressInput_WHIP_INPUT
	case "url":
		inputType = lkproto.IngressInput_URL_INPUT
	default:
		return "", nil, fmt.Errorf("lk.ingressCreate: unsupported input_type %q (use rtmp, whip, or url)", inputTypeStr)
	}

	room, err := plugin.ResolveString(nCtx, config, "room")
	if err != nil {
		return "", nil, fmt.Errorf("lk.ingressCreate: %w", err)
	}

	participantIdentity, err := plugin.ResolveString(nCtx, config, "participant_identity")
	if err != nil {
		return "", nil, fmt.Errorf("lk.ingressCreate: %w", err)
	}

	req := &lkproto.CreateIngressRequest{
		InputType:           inputType,
		RoomName:            room,
		ParticipantIdentity: participantIdentity,
	}

	if name, ok, err := plugin.ResolveOptionalString(nCtx, config, "participant_name"); err != nil {
		return "", nil, fmt.Errorf("lk.ingressCreate: %w", err)
	} else if ok {
		req.ParticipantName = name
	}

	if urlStr, ok, err := plugin.ResolveOptionalString(nCtx, config, "url"); err != nil {
		return "", nil, fmt.Errorf("lk.ingressCreate: %w", err)
	} else if ok {
		req.Url = urlStr
	}

	info, err := svc.Ingress.CreateIngress(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.ingressCreate: %w", err)
	}

	return api.OutputSuccess, ingressInfoToMap(info), nil
}

func ingressInfoToMap(info *lkproto.IngressInfo) map[string]any {
	return map[string]any{
		"ingress_id":           info.IngressId,
		"url":                  info.Url,
		"stream_key":           info.StreamKey,
		"room":                 info.RoomName,
		"participant_identity": info.ParticipantIdentity,
		"participant_name":     info.ParticipantName,
		"input_type":           info.InputType.String(),
	}
}
