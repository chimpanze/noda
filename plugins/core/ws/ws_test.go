package ws

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Plugin registration tests ---

func TestPlugin_Name(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "ws", p.Name())
}

func TestPlugin_Prefix(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "ws", p.Prefix())
}

func TestPlugin_HasServices(t *testing.T) {
	p := &Plugin{}
	assert.False(t, p.HasServices())
}

func TestPlugin_Nodes(t *testing.T) {
	p := &Plugin{}
	nodes := p.Nodes()
	require.Len(t, nodes, 1)
	assert.Equal(t, "send", nodes[0].Descriptor.Name())
	assert.NotNil(t, nodes[0].Factory)
}

func TestPlugin_CreateService(t *testing.T) {
	p := &Plugin{}
	svc, err := p.CreateService(nil)
	assert.NoError(t, err)
	assert.Nil(t, svc)
}

func TestPlugin_HealthCheck(t *testing.T) {
	p := &Plugin{}
	assert.NoError(t, p.HealthCheck(nil))
}

func TestPlugin_Shutdown(t *testing.T) {
	p := &Plugin{}
	assert.NoError(t, p.Shutdown(nil))
}

// --- Descriptor tests ---

func TestSendDescriptor_Name(t *testing.T) {
	d := &sendDescriptor{}
	assert.Equal(t, "send", d.Name())
}

func TestSendDescriptor_ServiceDeps(t *testing.T) {
	d := &sendDescriptor{}
	deps := d.ServiceDeps()
	require.Contains(t, deps, "connections")
	assert.Equal(t, "ws", deps["connections"].Prefix)
	assert.True(t, deps["connections"].Required)
}

func TestSendDescriptor_ConfigSchema(t *testing.T) {
	d := &sendDescriptor{}
	schema := d.ConfigSchema()
	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "channel")
	assert.Contains(t, props, "data")

	required, ok := schema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "channel")
	assert.Contains(t, required, "data")
}

// --- Executor tests ---

func TestSendExecutor_Outputs(t *testing.T) {
	e := newSendExecutor(nil)
	outputs := e.Outputs()
	assert.Contains(t, outputs, "success")
	assert.Contains(t, outputs, "error")
}

func TestSendExecutor_Factory(t *testing.T) {
	e := newSendExecutor(map[string]any{"key": "value"})
	assert.NotNil(t, e)
}

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

// --- Error path tests ---

func TestWsSend_MissingService(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newSendExecutor(nil)

	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "room.1",
		"data":    "hello",
	}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connections")
}

func TestWsSend_WrongServiceType(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newSendExecutor(nil)

	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "room.1",
		"data":    "hello",
	}, map[string]any{"connections": "not-a-service"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connections")
}

func TestWsSend_MissingChannel(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "ws-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"data": "hello",
	}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ws.send")
	assert.Contains(t, err.Error(), "channel")
}

func TestWsSend_MissingData(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "ws-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "room.1",
	}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ws.send")
	assert.Contains(t, err.Error(), "data")
}

func TestWsSend_MapData_JSONSerialized(t *testing.T) {
	mgr := connmgr.NewManager()
	var received []byte

	mgr.Register(&connmgr.Conn{
		ID:      "c1",
		Channel: "room.1",
		SendFn:  func(data []byte) error { received = data; return nil },
	})

	svc := connmgr.NewEndpointService(mgr, "ws-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "room.1",
		"data": map[string]any{
			"type":    "message",
			"content": "hello world",
		},
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "room.1", result.(map[string]any)["channel"])

	// The map data should be JSON-serialized by the connmgr
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(received, &parsed))
	assert.Equal(t, "message", parsed["type"])
	assert.Equal(t, "hello world", parsed["content"])
}

func TestWsSend_ChannelNonString(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "ws-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": 12345,
		"data":    "hello",
	}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ws.send")
}
