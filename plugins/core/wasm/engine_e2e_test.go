package wasm

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	wasmrt "github.com/chimpanze/noda/internal/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWasmService(t *testing.T) (*wasmrt.WasmService, *mockPlugin) {
	t.Helper()
	svcReg := registry.NewServiceRegistry()
	rt := wasmrt.NewRuntime(svcReg, nil, slog.Default())
	plug := newMockPlugin()
	_, _ = rt.LoadModuleWithPlugin(wasmrt.ModuleConfig{Name: "game", TickRate: 20}, plug)
	require.NoError(t, rt.StartAll(context.Background()))
	t.Cleanup(func() { _ = rt.StopAll(context.Background()) })
	time.Sleep(50 * time.Millisecond)
	return wasmrt.NewWasmService(rt, "game"), plug
}

func runWasmNode(t *testing.T, nodeType string, config map[string]any, svc *wasmrt.WasmService) (any, error) {
	t.Helper()
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("wasmsvc", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "wasm-wf",
		Nodes: map[string]engine.NodeConfig{
			"n": {Type: nodeType, Config: config, Services: map[string]string{"runtime": "wasmsvc"}},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	if err := engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg); err != nil {
		return nil, err
	}
	out, _ := execCtx.GetOutput("n")
	return out, nil
}

func TestWasmSend_Engine(t *testing.T) {
	svc, plug := setupWasmService(t)
	out, err := runWasmNode(t, "wasm.send", map[string]any{"data": map[string]any{"cmd": "move"}}, svc)
	require.NoError(t, err)
	assert.Equal(t, true, out.(map[string]any)["sent"])
	// The command reached the module on its tick loop.
	require.Eventually(t, func() bool {
		plug.mu.Lock()
		defer plug.mu.Unlock()
		return len(plug.calls) > 0
	}, time.Second, 10*time.Millisecond)
}

func TestWasmQuery_Engine(t *testing.T) {
	svc, plug := setupWasmService(t)
	plug.mu.Lock()
	plug.responses["query"] = mockResponse{exitCode: 0, data: []byte(`{"state":"ok"}`)}
	plug.exports["query"] = true
	plug.mu.Unlock()

	out, err := runWasmNode(t, "wasm.query", map[string]any{"data": map[string]any{"type": "get_state"}}, svc)
	require.NoError(t, err)
	require.NotNil(t, out)
	resultMap, ok := out.(map[string]any)
	require.True(t, ok, "query result should be a map[string]any")
	assert.Equal(t, "ok", resultMap["state"], "queried data should flow back to node output")
}
