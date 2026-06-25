package sse

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSESend_Engine(t *testing.T) {
	mgr := connmgr.NewManager()
	var gotEvent, gotData string
	require.NoError(t, mgr.Register(&connmgr.Conn{
		ID:      "c1",
		Channel: "feed.1",
		SSEFn:   func(event, data, id string) error { gotEvent, gotData = event, data; return nil },
	}))
	svc := connmgr.NewEndpointService(mgr, "sse-test")

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("conns", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "sse-wf",
		Nodes: map[string]engine.NodeConfig{
			"send": {
				Type:     "sse.send",
				Config:   map[string]any{"channel": "{{ input.ch }}", "event": "update", "data": "{{ input.msg }}"},
				Services: map[string]string{"connections": "conns"},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"ch": "feed.1", "msg": "tick"}))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))

	out, ok := execCtx.GetOutput("send")
	require.True(t, ok)
	assert.Equal(t, "feed.1", out.(map[string]any)["channel"])
	assert.Equal(t, "update", gotEvent)
	assert.Contains(t, gotData, "tick")
}
