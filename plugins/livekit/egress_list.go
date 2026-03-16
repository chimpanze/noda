package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type egressListDescriptor struct{}

func (d *egressListDescriptor) Name() string        { return "egressList" }
func (d *egressListDescriptor) Description() string { return "Lists egress recordings" }
func (d *egressListDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *egressListDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room": map[string]any{"type": "string", "description": "Optional room name filter"},
		},
	}
}
func (d *egressListDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "List of egress items",
		"error":   "Failed to list egress",
	}
}

type egressListExecutor struct{}

func newEgressListExecutor(_ map[string]any) api.NodeExecutor { return &egressListExecutor{} }

func (e *egressListExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *egressListExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressList: %w", err)
	}

	req := &lkproto.ListEgressRequest{}

	if room, ok, err := plugin.ResolveOptionalString(nCtx, config, "room"); err != nil {
		return "", nil, fmt.Errorf("lk.egressList: %w", err)
	} else if ok {
		req.RoomName = room
	}

	resp, err := svc.Egress.ListEgress(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressList: %w", err)
	}

	items := make([]any, len(resp.Items))
	for i, info := range resp.Items {
		items[i] = egressInfoToMap(info)
	}

	return api.OutputSuccess, map[string]any{"items": items}, nil
}
