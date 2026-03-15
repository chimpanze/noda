package util

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/pkg/api"
)

type timestampDescriptor struct{}

func (d *timestampDescriptor) Name() string                           { return "timestamp" }
func (d *timestampDescriptor) Description() string                    { return "Returns the current UTC timestamp" }
func (d *timestampDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *timestampDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"format": map[string]any{
				"type":        "string",
				"enum":        []any{"iso8601", "unix", "unix_ms"},
				"default":     "iso8601",
				"description": "Output format: iso8601, unix, or unix_ms",
			},
		},
	}
}
func (d *timestampDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "ISO 8601 timestamp string",
	}
}

type timestampExecutor struct{}

func newTimestampExecutor(config map[string]any) api.NodeExecutor {
	return &timestampExecutor{}
}

func (e *timestampExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *timestampExecutor) Execute(_ context.Context, _ api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	format, _ := config["format"].(string)
	if format == "" {
		format = "iso8601"
	}

	now := time.Now().UTC()

	switch format {
	case "iso8601":
		return api.OutputSuccess, now.Format(time.RFC3339), nil
	case "unix":
		return api.OutputSuccess, now.Unix(), nil
	case "unix_ms":
		return api.OutputSuccess, now.UnixMilli(), nil
	default:
		return "", nil, fmt.Errorf("util.timestamp: unknown format %q", format)
	}
}
