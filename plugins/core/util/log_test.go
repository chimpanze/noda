package util

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLog_EachLevel(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		t.Run(level, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			execCtx := engine.NewExecutionContext(engine.WithLogger(logger))

			executor := newLogExecutor(nil)
			config := map[string]any{
				"level":   level,
				"message": "{{ \"test message\" }}",
			}

			output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
			require.NoError(t, err)
			assert.Equal(t, "success", output)
			assert.Nil(t, data)
			assert.Contains(t, buf.String(), "test message")
		})
	}
}

func TestLog_MessageInterpolation(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{"user": "Alice"}),
		engine.WithLogger(logger),
	)

	executor := newLogExecutor(nil)
	config := map[string]any{
		"level":   "info",
		"message": "{{ \"User: \" + input.user }}",
	}

	output, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Contains(t, buf.String(), "User: Alice")
}

func TestLog_WithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	execCtx := engine.NewExecutionContext(
		engine.WithInput(map[string]any{"id": 42}),
		engine.WithLogger(logger),
	)

	executor := newLogExecutor(nil)
	config := map[string]any{
		"level":   "info",
		"message": "{{ \"processing\" }}",
		"fields": map[string]any{
			"user_id": "{{ input.id }}",
		},
	}

	output, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Contains(t, buf.String(), "processing")
}

func TestLog_NoFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	execCtx := engine.NewExecutionContext(engine.WithLogger(logger))

	executor := newLogExecutor(nil)
	config := map[string]any{
		"level":   "debug",
		"message": "{{ \"simple\" }}",
	}

	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Nil(t, data)
}
