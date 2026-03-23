package wasm

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	wasmrt "github.com/chimpanze/noda/internal/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPlugin implements wasmrt.PluginInstance for testing.
type mockPlugin struct {
	mu        sync.Mutex
	calls     []mockCall
	responses map[string]mockResponse
	exports   map[string]bool
}

type mockCall struct {
	Name string
	Data []byte
}

type mockResponse struct {
	exitCode uint32
	data     []byte
	err      error
}

func newMockPlugin() *mockPlugin {
	return &mockPlugin{
		responses: make(map[string]mockResponse),
		exports: map[string]bool{
			"initialize": true,
			"tick":       true,
			"shutdown":   true,
		},
	}
}

func (m *mockPlugin) Call(name string, data []byte) (uint32, []byte, error) {
	m.mu.Lock()
	m.calls = append(m.calls, mockCall{Name: name, Data: data})
	resp, ok := m.responses[name]
	m.mu.Unlock()
	if ok {
		return resp.exitCode, resp.data, resp.err
	}
	return 0, nil, nil
}

func (m *mockPlugin) FunctionExists(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.exports[name]
}

func (m *mockPlugin) Close(_ context.Context) error { return nil }

func TestPlugin(t *testing.T) {
	p := &Plugin{}

	assert.Equal(t, "core.wasm", p.Name())
	assert.Equal(t, "wasm", p.Prefix())
	assert.False(t, p.HasServices())

	nodes := p.Nodes()
	require.Len(t, nodes, 2)

	// Verify send descriptor
	assert.Equal(t, "send", nodes[0].Descriptor.Name())
	sendDeps := nodes[0].Descriptor.ServiceDeps()
	require.Contains(t, sendDeps, "runtime")
	assert.Equal(t, "wasm", sendDeps["runtime"].Prefix)
	assert.True(t, sendDeps["runtime"].Required)
	sendSchema := nodes[0].Descriptor.ConfigSchema()
	assert.Equal(t, "object", sendSchema["type"])
	required, ok := sendSchema["required"].([]any)
	require.True(t, ok)
	assert.Contains(t, required, "data")

	// Verify query descriptor
	assert.Equal(t, "query", nodes[1].Descriptor.Name())
	queryDeps := nodes[1].Descriptor.ServiceDeps()
	require.Contains(t, queryDeps, "runtime")
	assert.Equal(t, "wasm", queryDeps["runtime"].Prefix)
	assert.True(t, queryDeps["runtime"].Required)
	querySchema := nodes[1].Descriptor.ConfigSchema()
	assert.Equal(t, "object", querySchema["type"])
	props, ok := querySchema["properties"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, props, "timeout")

	// Verify factory creates executors with correct outputs
	sendExec := nodes[0].Factory(nil)
	assert.Equal(t, []string{"success", "error"}, sendExec.Outputs())
	queryExec := nodes[1].Factory(nil)
	assert.Equal(t, []string{"success", "error"}, queryExec.Outputs())

	// Service lifecycle methods should be no-ops
	svc, err := p.CreateService(nil)
	assert.NoError(t, err)
	assert.Nil(t, svc)
	assert.NoError(t, p.HealthCheck(nil))
	assert.NoError(t, p.Shutdown(nil))
}

func TestWasmSend_MissingService(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newSendExecutor(nil)

	// Empty services map - no "runtime" key
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"data": "hello",
	}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime")
}

func TestWasmSend_MissingData(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := wasmrt.NewRuntime(svcReg, nil, slog.Default())

	plug := newMockPlugin()
	_, _ = rt.LoadModuleWithPlugin(wasmrt.ModuleConfig{Name: "game", TickRate: 20}, plug)
	_ = rt.StartAll(context.Background())
	defer rt.StopAll(context.Background())

	time.Sleep(50 * time.Millisecond)

	ws := wasmrt.NewWasmService(rt, "game")
	services := map[string]any{"runtime": ws}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newSendExecutor(nil)
	// Config without "data" key
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wasm.send")
}

func TestWasmQuery_MissingService(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	e := newQueryExecutor(nil)

	// Empty services map - no "runtime" key
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"data": map[string]any{"type": "get_state"},
	}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "runtime")
}

func TestWasmQuery_MissingData(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := wasmrt.NewRuntime(svcReg, nil, slog.Default())

	plug := newMockPlugin()
	plug.exports["query"] = true
	_, _ = rt.LoadModuleWithPlugin(wasmrt.ModuleConfig{Name: "game", TickRate: 10}, plug)
	_ = rt.StartAll(context.Background())
	defer rt.StopAll(context.Background())

	time.Sleep(50 * time.Millisecond)

	ws := wasmrt.NewWasmService(rt, "game")
	services := map[string]any{"runtime": ws}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newQueryExecutor(nil)
	// Config without "data" key
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{}, services)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wasm.query")
}

func TestWasmQuery_DefaultTimeout(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := wasmrt.NewRuntime(svcReg, nil, slog.Default())

	plug := newMockPlugin()
	plug.exports["query"] = true
	plug.responses["query"] = mockResponse{
		data: []byte(`{"status":"ok"}`),
	}
	_, _ = rt.LoadModuleWithPlugin(wasmrt.ModuleConfig{Name: "game", TickRate: 10}, plug)
	_ = rt.StartAll(context.Background())
	defer rt.StopAll(context.Background())

	time.Sleep(50 * time.Millisecond)

	ws := wasmrt.NewWasmService(rt, "game")
	services := map[string]any{"runtime": ws}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newQueryExecutor(nil)
	// No "timeout" in config - should use default "5s"
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"data": map[string]any{"type": "get_status"},
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ok", resultMap["status"])
}

func TestWasmSend(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := wasmrt.NewRuntime(svcReg, nil, slog.Default())

	plugin := newMockPlugin()
	_, _ = rt.LoadModuleWithPlugin(wasmrt.ModuleConfig{Name: "game", TickRate: 20}, plugin)
	_ = rt.StartAll(context.Background())
	defer rt.StopAll(context.Background())

	time.Sleep(50 * time.Millisecond)

	ws := wasmrt.NewWasmService(rt, "game")
	services := map[string]any{"runtime": ws}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"msg": "hello",
	}))

	e := newSendExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"data": "{{ input.msg }}",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.NotNil(t, result)

	// Wait for command to be delivered in tick
	time.Sleep(200 * time.Millisecond)

	// Verify command appeared in tick input
	plugin.mu.Lock()
	var foundCommand bool
	for _, c := range plugin.calls {
		if c.Name == "tick" {
			var input wasmrt.TickInput
			_ = json.Unmarshal(c.Data, &input)
			if len(input.Commands) > 0 {
				foundCommand = true
			}
		}
	}
	plugin.mu.Unlock()
	assert.True(t, foundCommand, "command should be in tick")
}

func TestWasmQuery(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := wasmrt.NewRuntime(svcReg, nil, slog.Default())

	plugin := newMockPlugin()
	plugin.exports["query"] = true
	plugin.responses["query"] = mockResponse{
		data: []byte(`{"players":42}`),
	}
	_, _ = rt.LoadModuleWithPlugin(wasmrt.ModuleConfig{Name: "game", TickRate: 10}, plugin)
	_ = rt.StartAll(context.Background())
	defer rt.StopAll(context.Background())

	time.Sleep(50 * time.Millisecond)

	ws := wasmrt.NewWasmService(rt, "game")
	services := map[string]any{"runtime": ws}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newQueryExecutor(nil)
	output, result, err := e.Execute(context.Background(), execCtx, map[string]any{
		"data":    map[string]any{"type": "get_leaderboard"},
		"timeout": "2s",
	}, services)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(42), resultMap["players"])
}

func TestWasmQuery_Timeout(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := wasmrt.NewRuntime(svcReg, nil, slog.Default())

	plugin := newMockPlugin()
	plugin.exports["query"] = true
	// Simulate slow query - response takes too long
	plugin.responses["query"] = mockResponse{
		data: nil,
		err:  nil,
	}
	_, _ = rt.LoadModuleWithPlugin(wasmrt.ModuleConfig{Name: "game", TickRate: 1}, plugin) // slow tick rate
	_ = rt.StartAll(context.Background())
	defer rt.StopAll(context.Background())

	// Don't wait for tick to start - query should timeout
	ws := wasmrt.NewWasmService(rt, "game")
	services := map[string]any{"runtime": ws}
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))

	e := newQueryExecutor(nil)
	_, _, err := e.Execute(context.Background(), execCtx, map[string]any{
		"data":    map[string]any{"type": "get_state"},
		"timeout": "100ms",
	}, services)
	require.Error(t, err)
}
