package util

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type logDescriptor struct{}

func (d *logDescriptor) Name() string                           { return "log" }
func (d *logDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *logDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"level":   map[string]any{"type": "string", "enum": []any{"debug", "info", "warn", "error"}},
			"message": map[string]any{"type": "string"},
			"fields":  map[string]any{"type": "object"},
		},
		"required": []any{"level", "message"},
	}
}

type logExecutor struct{}

func newLogExecutor(config map[string]any) api.NodeExecutor {
	return &logExecutor{}
}

func (e *logExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *logExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	level, _ := config["level"].(string)
	messageExpr, _ := config["message"].(string)

	message, err := nCtx.Resolve(messageExpr)
	if err != nil {
		return "", nil, fmt.Errorf("util.log: message: %w", err)
	}

	messageStr := fmt.Sprintf("%v", message)

	// Resolve optional fields
	var resolvedFields map[string]any
	if fieldsCfg, ok := config["fields"].(map[string]any); ok {
		resolvedFields = make(map[string]any, len(fieldsCfg))
		for k, v := range fieldsCfg {
			if exprStr, ok := v.(string); ok {
				resolved, err := nCtx.Resolve(exprStr)
				if err != nil {
					return "", nil, fmt.Errorf("util.log: field %q: %w", k, err)
				}
				resolvedFields[k] = resolved
			} else {
				resolvedFields[k] = v
			}
		}
	}

	nCtx.Log(level, messageStr, resolvedFields)

	return api.OutputSuccess, nil, nil
}
