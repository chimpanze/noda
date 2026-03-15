package util

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/pkg/api"
)

type delayDescriptor struct{}

func (d *delayDescriptor) Name() string                           { return "delay" }
func (d *delayDescriptor) Description() string                    { return "Pauses execution for a specified duration" }
func (d *delayDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *delayDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"timeout": map[string]any{"type": "string", "description": "Duration to pause: 5s, 100ms, 1m"},
		},
		"required": []any{"timeout"},
	}
}
func (d *delayDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "null (delay completed)",
		"error":   "Context cancelled",
	}
}

type delayExecutor struct {
	duration time.Duration
}

func newDelayExecutor(config map[string]any) api.NodeExecutor {
	timeoutStr, _ := config["timeout"].(string)
	d, err := time.ParseDuration(timeoutStr)
	if err != nil {
		// Return executor that will report the error
		return &delayExecutor{duration: -1}
	}
	return &delayExecutor{duration: d}
}

func (e *delayExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *delayExecutor) Execute(ctx context.Context, _ api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	if e.duration < 0 {
		timeoutStr, _ := config["timeout"].(string)
		return "", nil, fmt.Errorf("util.delay: invalid duration %q", timeoutStr)
	}

	select {
	case <-time.After(e.duration):
		return api.OutputSuccess, nil, nil
	case <-ctx.Done():
		return "", nil, &api.TimeoutError{
			Duration:  e.duration,
			Operation: "util.delay",
		}
	}
}
