package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type egressStopDescriptor struct{}

func (d *egressStopDescriptor) Name() string        { return "egressStop" }
func (d *egressStopDescriptor) Description() string { return "Stops an active egress" }
func (d *egressStopDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *egressStopDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"egress_id": map[string]any{"type": "string", "description": "Egress ID to stop"},
		},
		"required": []any{"egress_id"},
	}
}
func (d *egressStopDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Final egress info",
		"error":   "Failed to stop egress",
	}
}

type egressStopExecutor struct{}

func newEgressStopExecutor(_ map[string]any) api.NodeExecutor { return &egressStopExecutor{} }

func (e *egressStopExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *egressStopExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStop: %w", err)
	}

	egressID, err := plugin.ResolveString(nCtx, config, "egress_id")
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStop: %w", err)
	}

	info, err := svc.Egress.StopEgress(ctx, &lkproto.StopEgressRequest{EgressId: egressID})
	if err != nil {
		return "", nil, fmt.Errorf("lk.egressStop: %w", err)
	}

	return api.OutputSuccess, egressInfoToMap(info), nil
}
