package sse

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Plugin registration
// ---------------------------------------------------------------------------

func TestPlugin_Name(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "core.sse", p.Name())
}

func TestPlugin_Prefix(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "sse", p.Prefix())
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
	assert.Nil(t, svc)
	assert.NoError(t, err)
}

func TestPlugin_HealthCheck(t *testing.T) {
	p := &Plugin{}
	assert.NoError(t, p.HealthCheck(nil))
}

func TestPlugin_Shutdown(t *testing.T) {
	p := &Plugin{}
	assert.NoError(t, p.Shutdown(nil))
}

// ---------------------------------------------------------------------------
// Descriptor
// ---------------------------------------------------------------------------

func TestSendDescriptor_Name(t *testing.T) {
	d := &sendDescriptor{}
	assert.Equal(t, "send", d.Name())
}

func TestSendDescriptor_ServiceDeps(t *testing.T) {
	d := &sendDescriptor{}
	deps := d.ServiceDeps()
	require.Contains(t, deps, "connections")
	assert.Equal(t, "sse", deps["connections"].Prefix)
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
	assert.Contains(t, props, "event")
	assert.Contains(t, props, "id")

	required, ok := schema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "channel")
	assert.Contains(t, required, "data")
}

// ---------------------------------------------------------------------------
// Executor basics
// ---------------------------------------------------------------------------

func TestSendExecutor_Outputs(t *testing.T) {
	e := newSendExecutor(nil)
	outputs := e.Outputs()
	assert.Equal(t, api.DefaultOutputs(), outputs)
}

// ---------------------------------------------------------------------------
// Execute – happy path
// ---------------------------------------------------------------------------

func TestSseSend(t *testing.T) {
	mgr := connmgr.NewManager()
	var gotEvent, gotData, gotID string

	require.NoError(t, mgr.Register(&connmgr.Conn{
		ID:      "c1",
		Channel: "updates",
		SSEFn: func(event, data, id string) error {
			gotEvent = event
			gotData = data
			gotID = id
			return nil
		},
	}))

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

// ---------------------------------------------------------------------------
// Execute – without optional fields (event and id omitted)
// ---------------------------------------------------------------------------

func TestSseSend_WithoutOptionalFields(t *testing.T) {
	mgr := connmgr.NewManager()
	var gotEvent, gotData, gotID string

	require.NoError(t, mgr.Register(&connmgr.Conn{
		ID:      "c1",
		Channel: "alerts",
		SSEFn: func(event, data, id string) error {
			gotEvent = event
			gotData = data
			gotID = id
			return nil
		},
	}))

	svc := connmgr.NewEndpointService(mgr, "sse-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "alerts",
		"data":    "payload",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, "alerts", result.(map[string]any)["channel"])
	assert.Equal(t, "", gotEvent)
	assert.Equal(t, "payload", gotData)
	assert.Equal(t, "", gotID)
}

// ---------------------------------------------------------------------------
// Execute – error: missing service
// ---------------------------------------------------------------------------

func TestSseSend_MissingService(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newSendExecutor(nil)

	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "ch",
		"data":    "d",
	}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connections")
}

// ---------------------------------------------------------------------------
// Execute – error: missing channel
// ---------------------------------------------------------------------------

func TestSseSend_MissingChannel(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "sse-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"data": "d",
	}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sse.send")
}

// ---------------------------------------------------------------------------
// Execute – error: missing data
// ---------------------------------------------------------------------------

func TestSseSend_MissingData(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "sse-test")
	services := map[string]any{"connections": svc}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"channel": "ch",
	}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sse.send")
}

// ---------------------------------------------------------------------------
// Execute – wildcard channel
// ---------------------------------------------------------------------------

func TestSseSend_Wildcard(t *testing.T) {
	mgr := connmgr.NewManager()
	var count int

	for _, ch := range []string{"room.1", "room.2", "room.3"} {
		require.NoError(t, mgr.Register(&connmgr.Conn{
			ID:      ch,
			Channel: ch,
			SSEFn: func(event, data, id string) error {
				count++
				return nil
			},
		}))
	}

	svc := connmgr.NewEndpointService(mgr, "sse-test")
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
