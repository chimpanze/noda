package util

import (
	"context"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelay_WaitsCorrectDuration(t *testing.T) {
	config := map[string]any{"timeout": "100ms"}
	executor := newDelayExecutor(config)
	execCtx := engine.NewExecutionContext()

	start := time.Now()
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Nil(t, data)
	assert.GreaterOrEqual(t, elapsed, 90*time.Millisecond)
}

func TestDelay_ContextCancellation(t *testing.T) {
	config := map[string]any{"timeout": "10s"}
	executor := newDelayExecutor(config)
	execCtx := engine.NewExecutionContext()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, _, err := executor.Execute(ctx, execCtx, config, nil)
	require.Error(t, err)

	var timeoutErr *api.TimeoutError
	require.ErrorAs(t, err, &timeoutErr)
	assert.Equal(t, "util.delay", timeoutErr.Operation)
}

func TestDelay_DurationParsing(t *testing.T) {
	for _, dur := range []string{"5s", "100ms", "1m"} {
		t.Run(dur, func(t *testing.T) {
			config := map[string]any{"timeout": dur}
			executor := newDelayExecutor(config)
			assert.Equal(t, []string{"success", "error"}, executor.Outputs())
		})
	}
}

func TestDelay_InvalidDuration(t *testing.T) {
	config := map[string]any{"timeout": "invalid"}
	executor := newDelayExecutor(config)
	execCtx := engine.NewExecutionContext()

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid duration")
}
