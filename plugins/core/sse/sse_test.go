package sse

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSseSend(t *testing.T) {
	mgr := connmgr.NewManager()
	var gotEvent, gotData, gotID string

	mgr.Register(&connmgr.Conn{
		ID:      "c1",
		Channel: "updates",
		SSEFn: func(event, data, id string) error {
			gotEvent = event
			gotData = data
			gotID = id
			return nil
		},
	})

	svc := connmgr.NewEndpointService(mgr, "sse-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"ch": "updates",
	}))

	e := newSendExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "{{ input.ch }}",
		"data":    "hello",
		"event":   "message",
		"id":      "1",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "updates", result.(map[string]any)["channel"])
	assert.Equal(t, "message", gotEvent)
	assert.Equal(t, "hello", gotData)
	assert.Equal(t, "1", gotID)
}

func TestSseSend_NoClients(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "sse-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "empty",
		"data":    "msg",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}
