package util

import (
	"context"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimestamp_ISO8601(t *testing.T) {
	executor := newTimestampExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{"format": "iso8601"}
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	ts, err := time.Parse(time.RFC3339, data.(string))
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now().UTC(), ts, 2*time.Second)
}

func TestTimestamp_Unix(t *testing.T) {
	executor := newTimestampExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{"format": "unix"}
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	unix := data.(int64)
	now := time.Now().Unix()
	assert.InDelta(t, now, unix, 2)
}

func TestTimestamp_UnixMs(t *testing.T) {
	executor := newTimestampExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{"format": "unix_ms"}
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	unixMs := data.(int64)
	nowMs := time.Now().UnixMilli()
	assert.InDelta(t, nowMs, unixMs, 2000)
}

func TestTimestamp_DefaultFormat(t *testing.T) {
	executor := newTimestampExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{}
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	// Default should be iso8601
	_, err = time.Parse(time.RFC3339, data.(string))
	require.NoError(t, err)
}

func TestTimestamp_UnknownFormat(t *testing.T) {
	executor := newTimestampExecutor(nil)
	execCtx := engine.NewExecutionContext()

	config := map[string]any{"format": "rfc2822"}
	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format")
	assert.Contains(t, err.Error(), "rfc2822")
}

func TestTimestamp_Descriptor(t *testing.T) {
	d := &timestampDescriptor{}
	assert.Equal(t, "timestamp", d.Name())
	assert.Nil(t, d.ServiceDeps())

	schema := d.ConfigSchema()
	require.NotNil(t, schema)
	props := schema["properties"].(map[string]any)
	formatProp := props["format"].(map[string]any)
	assert.Equal(t, "string", formatProp["type"])
	assert.Equal(t, "iso8601", formatProp["default"])
}

func TestTimestamp_Outputs(t *testing.T) {
	executor := newTimestampExecutor(nil)
	assert.Equal(t, []string{"success", "error"}, executor.Outputs())
}

func TestTimestamp_NilConfig(t *testing.T) {
	executor := newTimestampExecutor(nil)
	execCtx := engine.NewExecutionContext()

	// nil config map - format will be empty string, should default to iso8601
	config := map[string]any(nil)
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	_, err = time.Parse(time.RFC3339, data.(string))
	require.NoError(t, err)
}
