package ws

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWsSend(t *testing.T) {
	mgr := connmgr.NewManager()
	var received []byte

	mgr.Register(&connmgr.Conn{
		ID:      "c1",
		Channel: "room.42",
		SendFn:  func(data []byte) error { received = data; return nil },
	})

	svc := connmgr.NewEndpointService(mgr, "ws-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"room": "room.42",
		"msg":  "hello",
	}))

	e := newSendExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "{{ input.room }}",
		"data":    "{{ input.msg }}",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "room.42", result.(map[string]any)["channel"])
	assert.Equal(t, []byte("hello"), received)
}

func TestWsSend_NoClients(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "ws-test")
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

func TestWsSend_Wildcard(t *testing.T) {
	mgr := connmgr.NewManager()
	var count int

	for _, ch := range []string{"room.1", "room.2", "room.3"} {
		mgr.Register(&connmgr.Conn{
			ID:      ch,
			Channel: ch,
			SendFn:  func(data []byte) error { count++; return nil },
		})
	}

	svc := connmgr.NewEndpointService(mgr, "ws-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	output, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "room.*",
		"data":    "broadcast",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, 3, count)
}
