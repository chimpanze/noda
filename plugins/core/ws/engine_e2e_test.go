package ws

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWSSend_Engine(t *testing.T) {
	mgr := connmgr.NewManager()
	var got []byte
	require.NoError(t, mgr.Register(&connmgr.Conn{
		ID:      "c1",
		Channel: "room.1",
		SendFn:  func(d []byte) error { got = d; return nil },
	}))
	svc := connmgr.NewEndpointService(mgr, "ws-test")

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("conns", svc, nil))

	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "ws-wf",
		Nodes: map[string]engine.NodeConfig{
			"send": {
				Type:     "ws.send",
				Config:   map[string]any{"channel": "{{ input.room }}", "data": "{{ input.msg }}"},
				Services: map[string]string{"connections": "conns"},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"room": "room.1", "msg": "hello"}))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))

	out, ok := execCtx.GetOutput("send")
	require.True(t, ok)
	assert.Equal(t, "room.1", out.(map[string]any)["channel"])
	assert.Contains(t, string(got), "hello")
}

func TestWSSend_Engine_MissingChannel(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "ws-test")
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("conns", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "ws-wf-err",
		Nodes: map[string]engine.NodeConfig{
			"send": {Type: "ws.send", Config: map[string]any{"data": "x"}, Services: map[string]string{"connections": "conns"}},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err) // no error edge → workflow fails
}
