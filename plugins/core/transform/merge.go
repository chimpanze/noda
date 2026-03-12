package transform

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type mergeDescriptor struct{}

func (d *mergeDescriptor) Name() string { return "merge" }
func (d *mergeDescriptor) Description() string {
	return "Merges multiple arrays using different strategies"
}
func (d *mergeDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *mergeDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode":   map[string]any{"type": "string", "enum": []any{"append", "match", "position"}, "description": "Merge strategy: append, match, or position"},
			"inputs": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Expressions resolving to arrays to merge"},
			"match": map[string]any{
				"description": "Match configuration for match mode",
				"type":        "object",
				"properties": map[string]any{
					"type":   map[string]any{"type": "string", "enum": []any{"inner", "outer", "enrich"}},
					"fields": map[string]any{"type": "object"},
				},
			},
		},
		"required": []any{"mode", "inputs"},
	}
}

type mergeExecutor struct{}

func newMergeExecutor(config map[string]any) api.NodeExecutor {
	return &mergeExecutor{}
}

func (e *mergeExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *mergeExecutor) Execute(_ context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	mode, _ := config["mode"].(string)
	inputExprs, _ := config["inputs"].([]any)

	if len(inputExprs) == 0 {
		return "", nil, fmt.Errorf("transform.merge: inputs is required")
	}

	// Resolve all input arrays
	inputs := make([][]any, len(inputExprs))
	for i, expr := range inputExprs {
		exprStr, ok := expr.(string)
		if !ok {
			return "", nil, fmt.Errorf("transform.merge: input %d must be a string expression", i)
		}
		resolved, err := nCtx.Resolve(exprStr)
		if err != nil {
			return "", nil, fmt.Errorf("transform.merge: input %d: %w", i, err)
		}
		items, err := toSlice(resolved)
		if err != nil {
			return "", nil, fmt.Errorf("transform.merge: input %d must be an array: %w", i, err)
		}
		inputs[i] = items
	}

	switch mode {
	case "append":
		return e.appendMode(inputs)
	case "match":
		matchCfg, _ := config["match"].(map[string]any)
		return e.matchMode(inputs, matchCfg)
	case "position":
		return e.positionMode(inputs)
	default:
		return "", nil, fmt.Errorf("transform.merge: unknown mode %q", mode)
	}
}

func (e *mergeExecutor) appendMode(inputs [][]any) (string, any, error) {
	var result []any
	for _, items := range inputs {
		result = append(result, items...)
	}
	if result == nil {
		result = []any{}
	}
	return api.OutputSuccess, result, nil
}

func (e *mergeExecutor) matchMode(inputs [][]any, matchCfg map[string]any) (string, any, error) {
	if len(inputs) != 2 {
		return "", nil, fmt.Errorf("transform.merge: match mode requires exactly 2 inputs, got %d", len(inputs))
	}
	if matchCfg == nil {
		return "", nil, fmt.Errorf("transform.merge: match config is required for match mode")
	}

	matchType, _ := matchCfg["type"].(string)
	fields, _ := matchCfg["fields"].(map[string]any)
	if fields == nil {
		return "", nil, fmt.Errorf("transform.merge: match.fields is required")
	}
	leftField, _ := fields["left"].(string)
	rightField, _ := fields["right"].(string)
	if leftField == "" || rightField == "" {
		return "", nil, fmt.Errorf("transform.merge: match.fields.left and match.fields.right are required")
	}

	left := inputs[0]
	right := inputs[1]

	// Index right side by match field
	rightIndex := make(map[string]map[string]any)
	rightUsed := make(map[string]bool)
	for _, r := range right {
		row, ok := r.(map[string]any)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%v", row[rightField])
		rightIndex[key] = row
	}

	var result []any

	switch matchType {
	case "inner":
		for _, l := range left {
			row, ok := l.(map[string]any)
			if !ok {
				continue
			}
			key := fmt.Sprintf("%v", row[leftField])
			if rRow, ok := rightIndex[key]; ok {
				result = append(result, mergeRows(row, rRow))
			}
		}
	case "outer":
		for _, l := range left {
			row, ok := l.(map[string]any)
			if !ok {
				continue
			}
			key := fmt.Sprintf("%v", row[leftField])
			if rRow, ok := rightIndex[key]; ok {
				result = append(result, mergeRows(row, rRow))
				rightUsed[key] = true
			} else {
				result = append(result, copyRow(row))
			}
		}
		// Add unmatched right rows
		for _, r := range right {
			row, ok := r.(map[string]any)
			if !ok {
				continue
			}
			key := fmt.Sprintf("%v", row[rightField])
			if !rightUsed[key] {
				result = append(result, copyRow(row))
				rightUsed[key] = true
			}
		}
	case "enrich":
		for _, l := range left {
			row, ok := l.(map[string]any)
			if !ok {
				continue
			}
			key := fmt.Sprintf("%v", row[leftField])
			if rRow, ok := rightIndex[key]; ok {
				result = append(result, mergeRows(row, rRow))
			} else {
				result = append(result, copyRow(row))
			}
		}
	default:
		return "", nil, fmt.Errorf("transform.merge: unknown match type %q", matchType)
	}

	if result == nil {
		result = []any{}
	}
	return api.OutputSuccess, result, nil
}

func (e *mergeExecutor) positionMode(inputs [][]any) (string, any, error) {
	if len(inputs) == 0 {
		return api.OutputSuccess, []any{}, nil
	}

	length := len(inputs[0])
	for i, items := range inputs {
		if len(items) != length {
			return "", nil, fmt.Errorf("transform.merge: position mode requires all inputs to have the same length (input 0 has %d, input %d has %d)", length, i, len(items))
		}
	}

	result := make([]any, length)
	for i := 0; i < length; i++ {
		merged := make(map[string]any)
		for _, items := range inputs {
			if row, ok := items[i].(map[string]any); ok {
				for k, v := range row {
					merged[k] = v
				}
			}
		}
		result[i] = merged
	}

	return api.OutputSuccess, result, nil
}

func mergeRows(left, right map[string]any) map[string]any {
	result := make(map[string]any, len(left)+len(right))
	for k, v := range left {
		result[k] = v
	}
	for k, v := range right {
		result[k] = v
	}
	return result
}

func copyRow(row map[string]any) map[string]any {
	result := make(map[string]any, len(row))
	for k, v := range row {
		result[k] = v
	}
	return result
}
