package livekit

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	lkproto "github.com/livekit/protocol/livekit"
)

type ingressDeleteDescriptor struct{}

func (d *ingressDeleteDescriptor) Name() string        { return "ingressDelete" }
func (d *ingressDeleteDescriptor) Description() string { return "Deletes an ingress endpoint" }
func (d *ingressDeleteDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{serviceDep: {Prefix: "lk", Required: true}}
}
func (d *ingressDeleteDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ingress_id": map[string]any{"type": "string", "description": "Ingress ID to delete"},
		},
		"required": []any{"ingress_id"},
	}
}
func (d *ingressDeleteDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Ingress deleted confirmation",
		"error":   "Failed to delete ingress",
	}
}

type ingressDeleteExecutor struct{}

func newIngressDeleteExecutor(_ map[string]any) api.NodeExecutor { return &ingressDeleteExecutor{} }

func (e *ingressDeleteExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *ingressDeleteExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, serviceDep)
	if err != nil {
		return "", nil, fmt.Errorf("lk.ingressDelete: %w", err)
	}

	ingressID, err := plugin.ResolveString(nCtx, config, "ingress_id")
	if err != nil {
		return "", nil, fmt.Errorf("lk.ingressDelete: %w", err)
	}

	_, err = svc.Ingress.DeleteIngress(ctx, &lkproto.DeleteIngressRequest{IngressId: ingressID})
	if err != nil {
		return "", nil, fmt.Errorf("lk.ingressDelete: %w", err)
	}

	return api.OutputSuccess, map[string]any{"deleted": true}, nil
}
