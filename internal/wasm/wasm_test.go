package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPlugin implements PluginInstance for testing.
type mockPlugin struct {
	mu        sync.Mutex
	calls     []mockCall
	responses map[string]mockResponse
	exports   map[string]bool
	closed    bool
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

func (m *mockPlugin) Close(_ context.Context) error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	return nil
}

func (m *mockPlugin) getCalls(name string) []mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []mockCall
	for _, c := range m.calls {
		if c.Name == name {
			result = append(result, c)
		}
	}
	return result
}

func testLogger() *slog.Logger {
	return slog.Default()
}

// --- Encoding Tests ---

func TestCodec_JSON(t *testing.T) {
	codec, err := NewCodec("json")
	require.NoError(t, err)
	assert.Equal(t, "json", codec.Name())

	data := map[string]any{"key": "value", "num": float64(42)}
	encoded, err := codec.Marshal(data)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, codec.Unmarshal(encoded, &decoded))
	assert.Equal(t, "value", decoded["key"])
	assert.Equal(t, float64(42), decoded["num"])
}

func TestCodec_MessagePack(t *testing.T) {
	codec, err := NewCodec("msgpack")
	require.NoError(t, err)
	assert.Equal(t, "msgpack", codec.Name())

	data := map[string]any{"key": "value", "num": float64(42)}
	encoded, err := codec.Marshal(data)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, codec.Unmarshal(encoded, &decoded))
	assert.Equal(t, "value", decoded["key"])
}

func TestCodec_Default(t *testing.T) {
	codec, err := NewCodec("")
	require.NoError(t, err)
	assert.Equal(t, "json", codec.Name())
}

func TestCodec_Unknown(t *testing.T) {
	_, err := NewCodec("xml")
	assert.Error(t, err)
}

// --- Module Tests ---

func TestModule_Initialize(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	cfg := ModuleConfig{
		Name:     "test",
		TickRate: 10,
		Config:   map[string]any{"max_players": float64(100)},
	}

	m, err := NewModule("test", plugin, cfg, dispatcher, testLogger())
	require.NoError(t, err)

	err = m.Initialize(context.Background())
	require.NoError(t, err)

	calls := plugin.getCalls("initialize")
	require.Len(t, calls, 1)

	// Verify initialize input
	var input InitializeInput
	require.NoError(t, json.Unmarshal(calls[0].Data, &input))
	assert.Equal(t, "json", input.Encoding)
	assert.Equal(t, float64(100), input.Config["max_players"])
}

func TestModule_TickRate_Clamping(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	// Test minimum
	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 0}, dispatcher, testLogger())
	require.NoError(t, err)
	assert.Equal(t, 1, m.tickRate)

	// Test maximum
	m, err = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 200}, dispatcher, testLogger())
	require.NoError(t, err)
	assert.Equal(t, 120, m.tickRate)
}

func TestModule_Stop(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)

	err = m.Initialize(context.Background())
	require.NoError(t, err)

	m.Start()
	time.Sleep(50 * time.Millisecond) // let tick loop start

	err = m.Stop(context.Background())
	require.NoError(t, err)

	calls := plugin.getCalls("shutdown")
	assert.Len(t, calls, 1)
	assert.True(t, plugin.closed)
}

func TestModule_TickLoop(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 20}, dispatcher, testLogger())
	require.NoError(t, err)

	err = m.Initialize(context.Background())
	require.NoError(t, err)

	m.Start()
	time.Sleep(200 * time.Millisecond) // ~4 ticks at 20Hz

	err = m.Stop(context.Background())
	require.NoError(t, err)

	tickCalls := plugin.getCalls("tick")
	assert.GreaterOrEqual(t, len(tickCalls), 2, "should have at least 2 ticks")

	// Verify tick input has dt and timestamp
	if len(tickCalls) > 0 {
		var input TickInput
		require.NoError(t, json.Unmarshal(tickCalls[0].Data, &input))
		assert.Greater(t, input.Timestamp, int64(0))
	}
}

func TestModule_ClientMessages(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)

	m.Initialize(context.Background())

	// Add messages before starting tick loop
	m.AddClientMessage(ClientMessage{
		Endpoint: "game-ws",
		Channel:  "game.1",
		UserID:   "player1",
		Data:     map[string]any{"action": "move"},
	})

	m.Start()
	time.Sleep(150 * time.Millisecond)
	m.Stop(context.Background())

	tickCalls := plugin.getCalls("tick")
	require.NotEmpty(t, tickCalls)

	// First tick should contain the client message
	var input TickInput
	require.NoError(t, json.Unmarshal(tickCalls[0].Data, &input))
	assert.Len(t, input.ClientMessages, 1)
	assert.Equal(t, "game.1", input.ClientMessages[0].Channel)
}

func TestModule_Timers(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 20}, dispatcher, testLogger())
	require.NoError(t, err)

	m.Initialize(context.Background())

	// Set a timer that fires after 50ms
	m.SetTimer("save-state", 50)

	m.Start()
	time.Sleep(200 * time.Millisecond)
	m.Stop(context.Background())

	// Check that timer fired in one of the ticks
	tickCalls := plugin.getCalls("tick")
	timerFired := false
	for _, call := range tickCalls {
		var input TickInput
		json.Unmarshal(call.Data, &input)
		for _, t := range input.Timers {
			if t == "save-state" {
				timerFired = true
			}
		}
	}
	assert.True(t, timerFired, "timer should have fired")
}

func TestModule_ClearTimer(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)

	m.SetTimer("test-timer", 5000) // 5 seconds - won't fire
	m.ClearTimer("test-timer")

	m.mu.Lock()
	_, exists := m.timers["test-timer"]
	m.mu.Unlock()
	assert.False(t, exists)
}

func TestModule_AsyncResponse(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 20}, dispatcher, testLogger())
	require.NoError(t, err)

	m.Initialize(context.Background())

	// Add an async response
	m.AddAsyncResult("fetch-data", &AsyncResponse{
		Status: "ok",
		Data:   map[string]any{"result": "hello"},
	})

	m.Start()
	time.Sleep(150 * time.Millisecond)
	m.Stop(context.Background())

	// Check that response was delivered in a tick
	tickCalls := plugin.getCalls("tick")
	responseDelivered := false
	for _, call := range tickCalls {
		var input TickInput
		json.Unmarshal(call.Data, &input)
		if resp, ok := input.Responses["fetch-data"]; ok {
			assert.Equal(t, "ok", resp.Status)
			responseDelivered = true
		}
	}
	assert.True(t, responseDelivered, "async response should be delivered in tick")
}

func TestModule_Query(t *testing.T) {
	plugin := newMockPlugin()
	plugin.exports["query"] = true
	plugin.responses["query"] = mockResponse{
		data: []byte(`{"players":42}`),
	}

	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)

	m.Initialize(context.Background())
	m.Start()
	defer m.Stop(context.Background())

	// Give tick loop time to start
	time.Sleep(50 * time.Millisecond)

	result, err := m.Query(context.Background(), map[string]any{"type": "get_state"}, 2*time.Second)
	require.NoError(t, err)

	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(42), resultMap["players"])
}

func TestModule_SendCommand_WithExport(t *testing.T) {
	plugin := newMockPlugin()
	plugin.exports["command"] = true

	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)

	m.Initialize(context.Background())
	m.Start()
	defer m.Stop(context.Background())

	time.Sleep(50 * time.Millisecond)

	m.SendCommand(map[string]any{"action": "broadcast"})
	time.Sleep(200 * time.Millisecond)

	// command export should have been called (via queryCh)
	// Note: since "command" exists but "query" doesn't, processQuery uses "command"
	calls := plugin.getCalls("command")
	assert.NotEmpty(t, calls)
}

func TestModule_SendCommand_Buffered(t *testing.T) {
	plugin := newMockPlugin()
	// No "command" export - should buffer for tick

	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 20}, dispatcher, testLogger())
	require.NoError(t, err)

	m.Initialize(context.Background())

	m.SendCommand(map[string]any{"action": "test"})

	m.Start()
	time.Sleep(150 * time.Millisecond)
	m.Stop(context.Background())

	// Command should appear in tick's commands field
	tickCalls := plugin.getCalls("tick")
	commandDelivered := false
	for _, call := range tickCalls {
		var input TickInput
		json.Unmarshal(call.Data, &input)
		if len(input.Commands) > 0 {
			commandDelivered = true
			assert.Equal(t, "workflow", input.Commands[0].Source)
		}
	}
	assert.True(t, commandDelivered, "command should be delivered in tick")
}

func TestModule_IsServiceAllowed(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	cfg := ModuleConfig{
		Name:        "test",
		TickRate:    10,
		Services:    []string{"app-cache", "game-storage"},
		Connections: []string{"game-ws"},
	}

	m, err := NewModule("test", plugin, cfg, dispatcher, testLogger())
	require.NoError(t, err)

	assert.True(t, m.IsServiceAllowed(""))           // system always allowed
	assert.True(t, m.IsServiceAllowed("app-cache"))
	assert.True(t, m.IsServiceAllowed("game-storage"))
	assert.True(t, m.IsServiceAllowed("game-ws"))
	assert.False(t, m.IsServiceAllowed("secret-db"))
}

// --- HostDispatcher Tests ---

func TestHostDispatcher_SystemLog(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	_ = m

	result, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "log",
		Payload:   map[string]any{"level": "info", "message": "hello from wasm"},
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestHostDispatcher_SystemTimer(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "set_timer",
		Payload:   map[string]any{"name": "save", "interval": float64(5000)},
	})
	require.NoError(t, err)

	m.mu.Lock()
	_, exists := m.timers["save"]
	m.mu.Unlock()
	assert.True(t, exists)

	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "clear_timer",
		Payload:   map[string]any{"name": "save"},
	})
	require.NoError(t, err)

	m.mu.Lock()
	_, exists = m.timers["save"]
	m.mu.Unlock()
	assert.False(t, exists)
}

func TestHostDispatcher_TriggerWorkflow(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	var triggered atomic.Bool

	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		triggered.Store(true)
		assert.Equal(t, "ban-user", wfID)
		return nil
	}

	dispatcher := NewHostDispatcher(svcReg, runner, testLogger())
	plugin := newMockPlugin()
	NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "trigger_workflow",
		Payload:   map[string]any{"workflow": "ban-user", "input": map[string]any{"user_id": "123"}},
	})
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond) // async workflow
	assert.True(t, triggered.Load())
}

func TestHostDispatcher_PermissionDenied(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()

	// Module only allows "app-cache"
	NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"app-cache"},
	}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "secret-db",
		Operation: "read",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PERMISSION_DENIED")
}

func TestHostDispatcher_CacheService(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	cache := &mockCacheService{store: make(map[string]any)}
	svcReg.Register("app-cache", cache, nil)

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"app-cache"},
	}, dispatcher, testLogger())

	// Set
	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-cache",
		Operation: "set",
		Payload:   map[string]any{"key": "score", "value": float64(100), "ttl": float64(60)},
	})
	require.NoError(t, err)

	// Get
	result, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-cache",
		Operation: "get",
		Payload:   map[string]any{"key": "score"},
	})
	require.NoError(t, err)
	resultMap := result.(map[string]any)
	assert.Equal(t, float64(100), resultMap["value"])

	// Exists
	result, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-cache",
		Operation: "exists",
		Payload:   map[string]any{"key": "score"},
	})
	require.NoError(t, err)
	assert.True(t, result.(map[string]any)["exists"].(bool))
}

func TestHostDispatcher_AsyncCall(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	cache := &mockCacheService{store: map[string]any{"key1": "val1"}}
	svcReg.Register("app-cache", cache, nil)

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"app-cache"},
	}, dispatcher, testLogger())

	err := dispatcher.CallAsync(context.Background(), HostCallRequest{
		Service:   "app-cache",
		Operation: "get",
		Payload:   map[string]any{"key": "key1"},
		Label:     "fetch-key",
	})
	require.NoError(t, err)

	// Wait for async result
	time.Sleep(50 * time.Millisecond)

	m.mu.Lock()
	resp, ok := m.asyncResults["fetch-key"]
	m.mu.Unlock()
	require.True(t, ok)
	assert.Equal(t, "ok", resp.Status)
}

func TestHostDispatcher_AsyncDuplicateLabel(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"app-cache"},
	}, dispatcher, testLogger())

	// First call succeeds
	err := dispatcher.CallAsync(context.Background(), HostCallRequest{
		Service: "", Operation: "log",
		Payload: map[string]any{"level": "info", "message": "test"},
		Label:   "log1",
	})
	require.NoError(t, err)

	// Duplicate label fails
	err = dispatcher.CallAsync(context.Background(), HostCallRequest{
		Service: "", Operation: "log",
		Payload: map[string]any{"level": "info", "message": "test2"},
		Label:   "log1",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VALIDATION_ERROR")
}

// --- Runtime Tests ---

func TestRuntime_LoadAndGetModule(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	cfg := ModuleConfig{Name: "game", TickRate: 20}

	m, err := rt.LoadModuleWithPlugin(cfg, plugin)
	require.NoError(t, err)
	assert.NotNil(t, m)

	found, ok := rt.GetModule("game")
	assert.True(t, ok)
	assert.Equal(t, m, found)

	_, ok = rt.GetModule("nonexistent")
	assert.False(t, ok)
}

func TestRuntime_StartAndStopAll(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	rt.LoadModuleWithPlugin(ModuleConfig{Name: "game", TickRate: 20}, plugin)

	err := rt.StartAll(context.Background())
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	rt.StopAll(context.Background())

	assert.NotEmpty(t, plugin.getCalls("initialize"))
	assert.NotEmpty(t, plugin.getCalls("tick"))
	assert.NotEmpty(t, plugin.getCalls("shutdown"))
}

// --- WasmService Tests ---

func TestWasmService_Query(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	plugin.exports["query"] = true
	plugin.responses["query"] = mockResponse{data: []byte(`{"leaderboard":[1,2,3]}`)}

	rt.LoadModuleWithPlugin(ModuleConfig{Name: "game", TickRate: 10}, plugin)
	rt.StartAll(context.Background())
	defer rt.StopAll(context.Background())

	time.Sleep(50 * time.Millisecond)

	ws := NewWasmService(rt, "game")
	result, err := ws.Query(context.Background(), map[string]any{"type": "get_leaderboard"}, "2s")
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWasmService_SendCommand(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	rt.LoadModuleWithPlugin(ModuleConfig{Name: "game", TickRate: 20}, plugin)

	rt.StartAll(context.Background())
	defer rt.StopAll(context.Background())

	time.Sleep(50 * time.Millisecond)

	ws := NewWasmService(rt, "game")
	ws.SendCommand(map[string]any{"type": "broadcast", "message": "hello"})

	time.Sleep(200 * time.Millisecond)

	// Command should appear in a tick
	tickCalls := plugin.getCalls("tick")
	found := false
	for _, call := range tickCalls {
		var input TickInput
		json.Unmarshal(call.Data, &input)
		if len(input.Commands) > 0 {
			found = true
		}
	}
	assert.True(t, found, "command should be in tick input")
}

// --- Mock Services ---

type mockCacheService struct {
	mu    sync.Mutex
	store map[string]any
}

func (m *mockCacheService) Get(_ context.Context, key string) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.store[key]
	if !ok {
		return nil, fmt.Errorf("NOT_FOUND: key %q", key)
	}
	return v, nil
}

func (m *mockCacheService) Set(_ context.Context, key string, value any, _ int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = value
	return nil
}

func (m *mockCacheService) Del(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	return nil
}

func (m *mockCacheService) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.store[key]
	return ok, nil
}
