package workflow

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type outputDescriptor struct{}

func (d *outputDescriptor) Name() string { return "output" }
func (d *outputDescriptor) Description() string {
	return "Terminal node that declares a named output for the workflow"
}
func (d *outputDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *outputDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string", "description": "Output name for this workflow exit point"},
			"data": map[string]any{"description": "Output data expression"},
		},
		"required": []any{"name"},
	}
}
func (d *outputDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "The workflow output data",
		"error":   "Expression evaluation error",
	}
}

type outputExecutor struct{}

func newOutputExecutor(config map[string]any) api.NodeExecutor {
	return &outputExecutor{}
}

// Outputs returns empty — workflow.output is a terminal node.
func (e *outputExecutor) Outputs() []string { return []string{} }

func (e *outputExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	name := OutputName(config)
	if name == "" {
		return "", nil, fmt.Errorf("workflow.output: missing required field \"name\"")
	}

	// Resolve the data value if present. The value may be a string expression,
	// or a nested map/slice whose string leaves are expressions; resolve all of
	// them recursively (mirroring the engine's structured resolver) so object
	// and array outputs are supported, not just bare expression strings.
	if data, ok := config["data"]; ok {
		resolved, err := resolveOutputData(nCtx, data)
		if err != nil {
			return "", nil, err
		}
		return name, resolved, nil
	}

	// No data field — return nil
	return name, nil, nil
}

// resolveOutputData recursively resolves expression strings inside the data
// value. Strings are resolved as expressions; maps and slices are walked;
// any other value passes through unchanged.
func resolveOutputData(nCtx api.ExecutionContext, v any) (any, error) {
	switch val := v.(type) {
	case string:
		return nCtx.Resolve(val)
	case map[string]any:
		result := make(map[string]any, len(val))
		for k, item := range val {
			resolved, err := resolveOutputData(nCtx, item)
			if err != nil {
				return nil, err
			}
			result[k] = resolved
		}
		return result, nil
	case []any:
		result := make([]any, len(val))
		for i, item := range val {
			resolved, err := resolveOutputData(nCtx, item)
			if err != nil {
				return nil, err
			}
			result[i] = resolved
		}
		return result, nil
	default:
		return v, nil
	}
}

// OutputName returns the static name from the config.
func OutputName(config map[string]any) string {
	name, _ := config["name"].(string)
	return name
}
