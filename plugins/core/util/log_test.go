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

func TestLog_AllLevelsOutput(t *testing.T) {
	levels := []struct {
		level  string
		substr string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"error", "ERROR"},
	}
	for _, tc := range levels {
		t.Run(tc.level, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			execCtx := engine.NewExecutionContext(engine.WithLogger(logger))

			executor := newLogExecutor(nil)
			config := map[string]any{
				"level":   tc.level,
				"message": "{{ \"level test\" }}",
			}

			output, _, err := executor.Execute(context.Background(), execCtx, config, nil)
			require.NoError(t, err)
			assert.Equal(t, "success", output)
			assert.Contains(t, buf.String(), "level test")
			assert.Contains(t, buf.String(), "level="+tc.substr)
		})
	}
}

func TestLog_InvalidMessageExpression(t *testing.T) {
	execCtx := engine.NewExecutionContext()

	executor := newLogExecutor(nil)
	config := map[string]any{
		"level":   "info",
		"message": "{{ invalid..expr }}",
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "util.log: message:")
}

func TestLog_InvalidFieldExpression(t *testing.T) {
	execCtx := engine.NewExecutionContext()

	executor := newLogExecutor(nil)
	config := map[string]any{
		"level":   "info",
		"message": "{{ \"hello\" }}",
		"fields": map[string]any{
			"bad_field": "{{ invalid..expr }}",
		},
	}

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "util.log: field \"bad_field\":")
}

func TestLog_NonStringFieldValue(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	execCtx := engine.NewExecutionContext(engine.WithLogger(logger))

	executor := newLogExecutor(nil)
	config := map[string]any{
		"level":   "info",
		"message": "{{ \"with non-string\" }}",
		"fields": map[string]any{
			"count": 42,
		},
	}

	output, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Contains(t, buf.String(), "with non-string")
}

func TestLog_Descriptor(t *testing.T) {
	d := &logDescriptor{}
	assert.Equal(t, "log", d.Name())
	assert.Nil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	require.NotNil(t, schema)
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "level")
	assert.Contains(t, props, "message")
	assert.Contains(t, props, "fields")
}

func TestLog_Outputs(t *testing.T) {
	executor := newLogExecutor(nil)
	assert.Equal(t, []string{"success", "error"}, executor.Outputs())
}

func TestLog_DefaultLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	execCtx := engine.NewExecutionContext(engine.WithLogger(logger))

	executor := newLogExecutor(nil)
	config := map[string]any{
		"level":   "unknown_level",
		"message": "{{ \"fallback\" }}",
	}

	output, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	// Unknown level defaults to Info
	assert.Contains(t, buf.String(), "level=INFO")
}
