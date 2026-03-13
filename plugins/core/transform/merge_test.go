package transform

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMerge_Append(t *testing.T) {
	executor := newMergeExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"a": []any{1, 2},
		"b": []any{3, 4},
	}))

	config := map[string]any{
		"mode":   "append",
		"inputs": []any{"{{ input.a }}", "{{ input.b }}"},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{1, 2, 3, 4}, data)
}

func TestMerge_AppendThreeInputs(t *testing.T) {
	executor := newMergeExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"a": []any{1},
		"b": []any{2},
		"c": []any{3},
	}))

	config := map[string]any{
		"mode":   "append",
		"inputs": []any{"{{ input.a }}", "{{ input.b }}", "{{ input.c }}"},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, []any{1, 2, 3}, data)
}

func TestMerge_MatchInner(t *testing.T) {
	executor := newMergeExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"users": []any{
			map[string]any{"id": 1, "name": "Alice"},
			map[string]any{"id": 2, "name": "Bob"},
			map[string]any{"id": 3, "name": "Carol"},
		},
		"orders": []any{
			map[string]any{"user_id": 1, "total": 100},
			map[string]any{"user_id": 3, "total": 200},
		},
	}))

	config := map[string]any{
		"mode":   "match",
		"inputs": []any{"{{ input.users }}", "{{ input.orders }}"},
		"match": map[string]any{
			"type":   "inner",
			"fields": map[string]any{"left": "id", "right": "user_id"},
		},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.([]any)
	assert.Len(t, result, 2)
	assert.Equal(t, "Alice", result[0].(map[string]any)["name"])
	assert.Equal(t, 100, result[0].(map[string]any)["total"])
}

func TestMerge_MatchOuter(t *testing.T) {
	executor := newMergeExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"left":  []any{map[string]any{"id": 1, "name": "Alice"}},
		"right": []any{map[string]any{"id": 2, "email": "bob@example.com"}},
	}))

	config := map[string]any{
		"mode":   "match",
		"inputs": []any{"{{ input.left }}", "{{ input.right }}"},
		"match": map[string]any{
			"type":   "outer",
			"fields": map[string]any{"left": "id", "right": "id"},
		},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.([]any)
	assert.Len(t, result, 2)
}

func TestMerge_MatchEnrich(t *testing.T) {
	executor := newMergeExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"users": []any{
			map[string]any{"id": 1, "name": "Alice"},
			map[string]any{"id": 2, "name": "Bob"},
		},
		"profiles": []any{
			map[string]any{"user_id": 1, "bio": "Engineer"},
		},
	}))

	config := map[string]any{
		"mode":   "match",
		"inputs": []any{"{{ input.users }}", "{{ input.profiles }}"},
		"match": map[string]any{
			"type":   "enrich",
			"fields": map[string]any{"left": "id", "right": "user_id"},
		},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.([]any)
	assert.Len(t, result, 2)
	assert.Equal(t, "Engineer", result[0].(map[string]any)["bio"])
	assert.Equal(t, "Bob", result[1].(map[string]any)["name"])
	_, hasBio := result[1].(map[string]any)["bio"]
	assert.False(t, hasBio)
}

func TestMerge_Position(t *testing.T) {
	executor := newMergeExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"names":  []any{map[string]any{"name": "Alice"}, map[string]any{"name": "Bob"}},
		"emails": []any{map[string]any{"email": "alice@example.com"}, map[string]any{"email": "bob@example.com"}},
	}))

	config := map[string]any{
		"mode":   "position",
		"inputs": []any{"{{ input.names }}", "{{ input.emails }}"},
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.([]any)
	assert.Len(t, result, 2)
	assert.Equal(t, "Alice", result[0].(map[string]any)["name"])
	assert.Equal(t, "alice@example.com", result[0].(map[string]any)["email"])
}

func TestMerge_PositionDifferentLengths(t *testing.T) {
	executor := newMergeExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"a": []any{1, 2},
		"b": []any{3},
	}))

	config := map[string]any{
		"mode":   "position",
		"inputs": []any{"{{ input.a }}", "{{ input.b }}"},
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same length")
}

func TestMerge_MatchMoreThanTwoInputs(t *testing.T) {
	executor := newMergeExecutor(nil)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"a": []any{1},
		"b": []any{2},
		"c": []any{3},
	}))

	config := map[string]any{
		"mode":   "match",
		"inputs": []any{"{{ input.a }}", "{{ input.b }}", "{{ input.c }}"},
		"match": map[string]any{
			"type":   "inner",
			"fields": map[string]any{"left": "id", "right": "id"},
		},
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exactly 2 inputs")
}

func TestMerge_AppendExceedsMaxItems(t *testing.T) {
	executor := newMergeExecutor(nil)

	// Create arrays that together exceed maxMergeItems
	bigArray := make([]any, maxMergeItems/2+1)
	for i := range bigArray {
		bigArray[i] = i
	}

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"a": bigArray,
		"b": bigArray,
	}))

	config := map[string]any{
		"mode":   "append",
		"inputs": []any{"{{ input.a }}", "{{ input.b }}"},
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}
