package util

import (
	"context"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
)

type uuidDescriptor struct{}

func (d *uuidDescriptor) Name() string                           { return "uuid" }
func (d *uuidDescriptor) Description() string                    { return "Generates a UUID v4" }
func (d *uuidDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *uuidDescriptor) ConfigSchema() map[string]any           { return nil }

type uuidExecutor struct{}

func newUUIDExecutor(config map[string]any) api.NodeExecutor {
	return &uuidExecutor{}
}

func (e *uuidExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *uuidExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return api.OutputSuccess, uuid.New().String(), nil
}
