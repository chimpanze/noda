package engine

import (
	"bytes"
	"log/slog"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestTraceID_UniquePerExecution(t *testing.T) {
	ids := make(map[string]bool)
	for range 100 {
		ctx := NewExecutionContext()
		id := ctx.Trigger().TraceID
		assert.NotEmpty(t, id)
		assert.False(t, ids[id], "trace IDs should be unique")
		ids[id] = true
	}
}

func TestTraceID_InAllLogs(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx := NewExecutionContext(
		WithWorkflowID("test-wf"),
		WithLogger(logger),
	)

	ctx.Log("info", "test message", nil)
	output := buf.String()

	assert.Contains(t, output, ctx.Trigger().TraceID)
	assert.Contains(t, output, "test-wf")
}

func TestLog_IncludesNodeContext(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx := NewExecutionContext(WithLogger(logger))
	ctx.SetCurrentNode("step-1")
	ctx.Log("debug", "inside node", nil)

	output := buf.String()
	assert.Contains(t, output, "step-1")
}

func TestLog_Levels(t *testing.T) {
	tests := []struct {
		level    string
		expected string
	}{
		{"debug", "DEBUG"},
		{"info", "INFO"},
		{"warn", "WARN"},
		{"error", "ERROR"},
	}

	for _, tt := range tests {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		ctx := NewExecutionContext(WithLogger(logger))
		ctx.Log(tt.level, "test", nil)

		assert.Contains(t, buf.String(), tt.expected)
	}
}

func TestLog_CustomFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx := NewExecutionContext(WithLogger(logger))
	ctx.Log("info", "custom", map[string]any{
		"custom_key": "custom_value",
	})

	output := buf.String()
	assert.Contains(t, output, "custom_key")
	assert.Contains(t, output, "custom_value")
}

func TestWithTrigger_PreservesTraceID(t *testing.T) {
	trigger := api.TriggerData{
		Type:    "http",
		TraceID: "my-trace-id",
	}
	ctx := NewExecutionContext(WithTrigger(trigger))

	assert.Equal(t, "my-trace-id", ctx.Trigger().TraceID)
	assert.Equal(t, "http", ctx.Trigger().Type)
}

func TestWithTrigger_GeneratesTraceIDIfEmpty(t *testing.T) {
	trigger := api.TriggerData{Type: "event"}
	ctx := NewExecutionContext(WithTrigger(trigger))

	assert.NotEmpty(t, ctx.Trigger().TraceID)
	assert.Equal(t, "event", ctx.Trigger().Type)
}
