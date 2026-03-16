package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type ingressListDescriptor struct{}

func (d *ingressListDescriptor) Name() string        { return "ingressList" }
func (d *ingressListDescriptor) Description() string { return "Lists ingress endpoints" }
func (d *ingressListDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *ingressListDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room": map[string]any{"type": "string", "description": "Optional room name filter"},
		},
	}
}
func (d *ingressListDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "List of ingress items",
		"error":   "Failed to list ingress",
	}
}

type ingressListExecutor struct{}

func newIngressListExecutor(_ map[string]any) api.NodeExecutor { return &ingressListExecutor{} }

func (e *ingressListExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *ingressListExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.ingressList: %w", err)
	}

	req := &lkproto.ListIngressRequest{}

	if room, ok, err := plugin.ResolveOptionalString(nCtx, config, "room"); err != nil {
		return "", nil, fmt.Errorf("lk.ingressList: %w", err)
	} else if ok {
		req.RoomName = room
	}

	resp, err := svc.Ingress.ListIngress(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.ingressList: %w", err)
	}

	items := make([]any, len(resp.Items))
	for i, info := range resp.Items {
		items[i] = ingressInfoToMap(info)
	}

	return api.OutputSuccess, map[string]any{"items": items}, nil
}
