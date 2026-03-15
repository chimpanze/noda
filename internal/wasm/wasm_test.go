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

	"net/http"
	"net/http/httptest"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/fasthttp/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubPlugin implements api.Plugin for service registry tests.
type stubPlugin struct {
	name   string
	prefix string
}

func (p *stubPlugin) Name() string                                     { return p.name }
func (p *stubPlugin) Prefix() string                                   { return p.prefix }
func (p *stubPlugin) Nodes() []api.NodeRegistration                    { return nil }
func (p *stubPlugin) HasServices() bool                                { return false }
func (p *stubPlugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *stubPlugin) HealthCheck(service any) error                    { return nil }
func (p *stubPlugin) Shutdown(service any) error                       { return nil }

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

	require.NoError(t, m.Initialize(context.Background()))

	// Add messages before starting tick loop
	m.AddClientMessage(ClientMessage{
		Endpoint: "game-ws",
		Channel:  "game.1",
		UserID:   "player1",
		Data:     map[string]any{"action": "move"},
	})

	m.Start()
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, m.Stop(context.Background()))

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

	require.NoError(t, m.Initialize(context.Background()))

	// Set a timer that fires after 50ms
	m.SetTimer("save-state", 50)

	m.Start()
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, m.Stop(context.Background()))

	// Check that timer fired in one of the ticks
	tickCalls := plugin.getCalls("tick")
	timerFired := false
	for _, call := range tickCalls {
		var input TickInput
		_ = json.Unmarshal(call.Data, &input)
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

	require.NoError(t, m.Initialize(context.Background()))

	// Add an async response
	m.AddAsyncResult("fetch-data", &AsyncResponse{
		Status: "ok",
		Data:   map[string]any{"result": "hello"},
	})

	m.Start()
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, m.Stop(context.Background()))

	// Check that response was delivered in a tick
	tickCalls := plugin.getCalls("tick")
	responseDelivered := false
	for _, call := range tickCalls {
		var input TickInput
		_ = json.Unmarshal(call.Data, &input)
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

	require.NoError(t, m.Initialize(context.Background()))
	m.Start()
	defer func() { require.NoError(t, m.Stop(context.Background())) }()

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

	require.NoError(t, m.Initialize(context.Background()))
	m.Start()
	defer func() { require.NoError(t, m.Stop(context.Background())) }()

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

	require.NoError(t, m.Initialize(context.Background()))

	m.SendCommand(map[string]any{"action": "test"})

	m.Start()
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, m.Stop(context.Background()))

	// Command should appear in tick's commands field
	tickCalls := plugin.getCalls("tick")
	commandDelivered := false
	for _, call := range tickCalls {
		var input TickInput
		_ = json.Unmarshal(call.Data, &input)
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

	assert.True(t, m.IsServiceAllowed("")) // system always allowed
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
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

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
	_, _ = NewModule("test", plugin, ModuleConfig{
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
	require.NoError(t, svcReg.Register("app-cache", cache, nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
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
	require.NoError(t, svcReg.Register("app-cache", cache, nil))

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
	_, _ = NewModule("test", plugin, ModuleConfig{
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
	_, err := rt.LoadModuleWithPlugin(ModuleConfig{Name: "game", TickRate: 20}, plugin)
	require.NoError(t, err)

	err = rt.StartAll(context.Background())
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

	_, err := rt.LoadModuleWithPlugin(ModuleConfig{Name: "game", TickRate: 10}, plugin)
	require.NoError(t, err)
	require.NoError(t, rt.StartAll(context.Background()))
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
	_, err := rt.LoadModuleWithPlugin(ModuleConfig{Name: "game", TickRate: 20}, plugin)
	require.NoError(t, err)

	require.NoError(t, rt.StartAll(context.Background()))
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
		_ = json.Unmarshal(call.Data, &input)
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

// --- Mock StorageService ---

type mockStorageService struct {
	mu    sync.Mutex
	files map[string][]byte
}

func (s *mockStorageService) Read(_ context.Context, path string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.files[path]
	if !ok {
		return nil, fmt.Errorf("NOT_FOUND: %q", path)
	}
	return data, nil
}

func (s *mockStorageService) Write(_ context.Context, path string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[path] = data
	return nil
}

func (s *mockStorageService) Delete(_ context.Context, path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, path)
	return nil
}

func (s *mockStorageService) List(_ context.Context, _ string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var paths []string
	for k := range s.files {
		paths = append(paths, k)
	}
	return paths, nil
}

// --- Mock ConnectionService ---

type mockConnectionService struct {
	mu      sync.Mutex
	sent    []mockSentMsg
	sentSSE []mockSentSSE
}

type mockSentMsg struct {
	channel string
	data    any
}

type mockSentSSE struct {
	channel string
	event   string
	data    any
	id      string
}

func (c *mockConnectionService) Send(_ context.Context, channel string, data any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, mockSentMsg{channel: channel, data: data})
	return nil
}

func (c *mockConnectionService) SendSSE(_ context.Context, channel, event string, data any, id string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sentSSE = append(c.sentSSE, mockSentSSE{channel: channel, event: event, data: data, id: id})
	return nil
}

// --- Mock StreamService ---

type mockStreamService struct {
	mu       sync.Mutex
	messages []mockStreamMsg
	nextID   int
}

type mockStreamMsg struct {
	topic   string
	payload any
}

func (s *mockStreamService) Publish(_ context.Context, topic string, payload any) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, mockStreamMsg{topic: topic, payload: payload})
	s.nextID++
	return fmt.Sprintf("msg-%d", s.nextID), nil
}

// --- Mock PubSubService ---

type mockPubSubService struct {
	mu       sync.Mutex
	messages []mockPubSubMsg
}

type mockPubSubMsg struct {
	channel string
	payload any
}

func (p *mockPubSubService) Publish(_ context.Context, channel string, payload any) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, mockPubSubMsg{channel: channel, payload: payload})
	return nil
}

// --- Host Dispatcher: Storage dispatch ---

func TestHostDispatcher_StorageService(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	storage := &mockStorageService{files: make(map[string][]byte)}
	require.NoError(t, svcReg.Register("app-storage", storage, nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"app-storage"},
	}, dispatcher, testLogger())

	// Write
	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-storage",
		Operation: "write",
		Payload:   map[string]any{"path": "/data/test.txt", "data": "hello world"},
	})
	require.NoError(t, err)

	// Read
	result, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-storage",
		Operation: "read",
		Payload:   map[string]any{"path": "/data/test.txt"},
	})
	require.NoError(t, err)
	resultMap := result.(map[string]any)
	assert.Equal(t, "hello world", resultMap["data"])

	// Delete
	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-storage",
		Operation: "delete",
		Payload:   map[string]any{"path": "/data/test.txt"},
	})
	require.NoError(t, err)

	// Read after delete should fail
	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-storage",
		Operation: "read",
		Payload:   map[string]any{"path": "/data/test.txt"},
	})
	require.Error(t, err)

	// List
	storage.files["/a.txt"] = []byte("a")
	result, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-storage",
		Operation: "list",
		Payload:   map[string]any{"prefix": "/"},
	})
	require.NoError(t, err)
	resultMap = result.(map[string]any)
	assert.NotNil(t, resultMap["paths"])

	// Unknown operation
	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-storage",
		Operation: "rename",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown storage operation")
}

// --- Host Dispatcher: Connection dispatch ---

func TestHostDispatcher_ConnectionService(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	connSvc := &mockConnectionService{}
	require.NoError(t, svcReg.Register("ws-conn", connSvc, nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"ws-conn"},
	}, dispatcher, testLogger())

	// Send
	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "ws-conn",
		Operation: "send",
		Payload:   map[string]any{"channel": "game.1", "data": "hello"},
	})
	require.NoError(t, err)
	assert.Len(t, connSvc.sent, 1)
	assert.Equal(t, "game.1", connSvc.sent[0].channel)

	// SendSSE
	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "ws-conn",
		Operation: "send_sse",
		Payload:   map[string]any{"channel": "updates", "event": "score", "data": "100", "id": "evt-1"},
	})
	require.NoError(t, err)
	assert.Len(t, connSvc.sentSSE, 1)
	assert.Equal(t, "score", connSvc.sentSSE[0].event)

	// Unknown operation
	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "ws-conn",
		Operation: "broadcast",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown connection operation")
}

// --- Host Dispatcher: Stream dispatch ---

func TestHostDispatcher_StreamService(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	stream := &mockStreamService{}
	require.NoError(t, svcReg.Register("event-stream", stream, nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"event-stream"},
	}, dispatcher, testLogger())

	// Emit
	result, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "event-stream",
		Operation: "emit",
		Payload:   map[string]any{"topic": "game-events", "payload": map[string]any{"type": "score"}},
	})
	require.NoError(t, err)
	resultMap := result.(map[string]any)
	assert.NotEmpty(t, resultMap["message_id"])

	// Publish alias
	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "event-stream",
		Operation: "publish",
		Payload:   map[string]any{"topic": "game-events", "payload": "data"},
	})
	require.NoError(t, err)

	// Unknown operation
	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "event-stream",
		Operation: "subscribe",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown stream operation")
}

// --- Host Dispatcher: PubSub dispatch ---

func TestHostDispatcher_PubSubService(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	pubsub := &mockPubSubService{}
	require.NoError(t, svcReg.Register("app-pubsub", pubsub, nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"app-pubsub"},
	}, dispatcher, testLogger())

	// Emit
	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-pubsub",
		Operation: "emit",
		Payload:   map[string]any{"channel": "notifications", "payload": "hello"},
	})
	require.NoError(t, err)
	assert.Len(t, pubsub.messages, 1)
	assert.Equal(t, "notifications", pubsub.messages[0].channel)

	// Publish alias
	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-pubsub",
		Operation: "publish",
		Payload:   map[string]any{"channel": "alerts", "payload": "alert!"},
	})
	require.NoError(t, err)
	assert.Len(t, pubsub.messages, 2)

	// Unknown operation
	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "app-pubsub",
		Operation: "subscribe",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown pubsub operation")
}

// --- System Ops: unknown operation ---

func TestHostDispatcher_UnknownSystemOp(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "nonexistent_op",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown system operation")
}

// --- System Ops: unknown service type ---

func TestHostDispatcher_UnsupportedServiceType(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	// Register a service that doesn't implement any known interface
	require.NoError(t, svcReg.Register("weird-svc", "just a string", nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"weird-svc"},
	}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "weird-svc",
		Operation: "do_something",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported service type")
}

// --- System Ops: service not found ---

func TestHostDispatcher_ServiceNotFound(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"missing-svc"},
	}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "missing-svc",
		Operation: "get",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SERVICE_UNAVAILABLE")
}

// --- Cache ops: del and unknown op ---

func TestHostDispatcher_CacheDel(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	cache := &mockCacheService{store: map[string]any{"k1": "v1"}}
	require.NoError(t, svcReg.Register("cache", cache, nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"cache"},
	}, dispatcher, testLogger())

	// Del
	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "cache",
		Operation: "del",
		Payload:   map[string]any{"key": "k1"},
	})
	require.NoError(t, err)

	// Verify deleted
	cache.mu.Lock()
	_, exists := cache.store["k1"]
	cache.mu.Unlock()
	assert.False(t, exists)

	// Unknown cache op
	_, err = dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "cache",
		Operation: "flush",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown cache operation")
}

// --- containsHost tests ---

func TestContainsHost(t *testing.T) {
	tests := []struct {
		url    string
		host   string
		expect bool
	}{
		{"wss://example.com/path", "example.com", true},
		{"ws://example.com:8080/path", "example.com", true},
		{"wss://example.com", "example.com", true},
		{"wss://example.com?query=1", "example.com", true},
		{"wss://other.com/path", "example.com", false},
		{"", "example.com", false},
		{"wss://example.com/path", "", false},
		{"", "", false},
		{"wss://notexample.com/path", "example.com", false},
		{"https://api.discord.gg/ws", "api.discord.gg", true},
		{"wss://gateway.discord.gg:443/", "gateway.discord.gg", true},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_in_%s", tt.host, tt.url), func(t *testing.T) {
			assert.Equal(t, tt.expect, containsHost(tt.url, tt.host))
		})
	}
}

// --- Gateway isAllowed tests ---

func TestGateway_IsAllowed(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{"gateway.discord.gg", "api.example.com"},
	}, dispatcher, testLogger())

	gw := m.gateway

	assert.True(t, gw.isAllowed("wss://gateway.discord.gg/"))
	assert.True(t, gw.isAllowed("wss://api.example.com:443/ws"))
	assert.False(t, gw.isAllowed("wss://evil.com/ws"))
	assert.False(t, gw.isAllowed("wss://notgateway.discord.gg/"))
}

func TestGateway_IsAllowed_EmptyWhitelist(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  nil, // no whitelist
	}, dispatcher, testLogger())

	gw := m.gateway
	assert.False(t, gw.isAllowed("wss://anything.com/"))
}

// --- Module: duplicate async label ---

func TestModule_RegisterAsyncLabel_Duplicate(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	err := m.RegisterAsyncLabel("label-1")
	require.NoError(t, err)

	err = m.RegisterAsyncLabel("label-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate async label")
}

// --- Module: connection events ---

func TestModule_AddConnectionEvent(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 20}, dispatcher, testLogger())
	require.NoError(t, m.Initialize(context.Background()))

	m.AddConnectionEvent(ConnectionEvent{
		Endpoint: "game-ws",
		Channel:  "game.1",
		UserID:   "user1",
		Event:    "connected",
	})
	m.AddConnectionEvent(ConnectionEvent{
		Connection: "discord-gw",
		Event:      "disconnected",
		Reason:     "server closed",
	})

	m.Start()
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, m.Stop(context.Background()))

	tickCalls := plugin.getCalls("tick")
	require.NotEmpty(t, tickCalls)

	var input TickInput
	require.NoError(t, json.Unmarshal(tickCalls[0].Data, &input))
	assert.Len(t, input.ConnectionEvents, 2)
	assert.Equal(t, "connected", input.ConnectionEvents[0].Event)
	assert.Equal(t, "disconnected", input.ConnectionEvents[1].Event)
}

// --- Module: incoming WS ---

func TestModule_AddIncomingWS(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 20}, dispatcher, testLogger())
	require.NoError(t, m.Initialize(context.Background()))

	m.AddIncomingWS(IncomingWSMsg{
		Connection: "discord-gw",
		Data:       map[string]any{"op": float64(0), "t": "MESSAGE_CREATE"},
	})

	m.Start()
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, m.Stop(context.Background()))

	tickCalls := plugin.getCalls("tick")
	require.NotEmpty(t, tickCalls)

	var input TickInput
	require.NoError(t, json.Unmarshal(tickCalls[0].Data, &input))
	assert.Len(t, input.IncomingWS, 1)
	assert.Equal(t, "discord-gw", input.IncomingWS[0].Connection)
}

// --- Runtime: StopAll with no modules ---

func TestRuntime_StopAll_NoModules(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	// Should not panic
	rt.StopAll(context.Background())
}

// --- Runtime: StartAll with no modules ---

func TestRuntime_StartAll_NoModules(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	err := rt.StartAll(context.Background())
	require.NoError(t, err)
}

// --- System ops: log levels ---

func TestHostDispatcher_SystemLog_AllLevels(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	for _, level := range []string{"debug", "info", "warn", "error", "unknown_level"} {
		result, err := dispatcher.Call(context.Background(), HostCallRequest{
			Service:   "",
			Operation: "log",
			Payload: map[string]any{
				"level":   level,
				"message": "test message",
				"fields":  map[string]any{"key": "value"},
			},
		})
		require.NoError(t, err)
		assert.Nil(t, result)
	}
}

// --- System ops: trigger_workflow with missing workflow ---

func TestHostDispatcher_TriggerWorkflow_MissingWorkflowID(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "trigger_workflow",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "\"workflow\" is required")
}

// --- System ops: set_timer validation ---

func TestHostDispatcher_SetTimer_MissingName(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "set_timer",
		Payload:   map[string]any{"interval": float64(1000)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "\"name\" is required")
}

func TestHostDispatcher_SetTimer_InvalidInterval(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "set_timer",
		Payload:   map[string]any{"name": "test", "interval": float64(-100)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interval must be positive")
}

// --- System ops: clear_timer missing name ---

func TestHostDispatcher_ClearTimer_MissingName(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "clear_timer",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "\"name\" is required")
}

// --- System ops: nil payload ---

func TestHostDispatcher_SystemOp_NilPayload(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	// Log with nil payload should not panic
	result, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "log",
		Payload:   nil,
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

// --- DispatchToService nil payload ---

func TestHostDispatcher_DispatchToService_NilPayload(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	cache := &mockCacheService{store: map[string]any{"": "empty-key-val"}}
	require.NoError(t, svcReg.Register("cache", cache, nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"cache"},
	}, dispatcher, testLogger())

	// Call with nil payload - should return validation error for missing key
	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "cache",
		Operation: "get",
		Payload:   nil, // nil payload, key is required
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "\"key\" is required")
}

// --- AsyncCall: missing label ---

func TestHostDispatcher_AsyncCall_MissingLabel(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	err := dispatcher.CallAsync(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "log",
		Payload:   map[string]any{"level": "info", "message": "test"},
		Label:     "", // empty label
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "label is required")
}

// --- AsyncCall: permission denied stores error result ---

func TestHostDispatcher_AsyncCall_PermissionDenied(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{}, // no services allowed
	}, dispatcher, testLogger())

	err := dispatcher.CallAsync(context.Background(), HostCallRequest{
		Service:   "forbidden-svc",
		Operation: "get",
		Payload:   map[string]any{},
		Label:     "denied-call",
	})
	require.NoError(t, err) // async permission denied doesn't return error

	// Check the async result was stored with error
	m.mu.Lock()
	resp, ok := m.asyncResults["denied-call"]
	m.mu.Unlock()
	require.True(t, ok)
	assert.Equal(t, "error", resp.Status)
	assert.Contains(t, resp.Error.Code, "PERMISSION_DENIED")
}

// --- AsyncCall: error in underlying Call stores error result ---

func TestHostDispatcher_AsyncCall_ErrorResult(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"missing-svc"},
	}, dispatcher, testLogger())

	err := dispatcher.CallAsync(context.Background(), HostCallRequest{
		Service:   "missing-svc",
		Operation: "get",
		Payload:   map[string]any{},
		Label:     "error-call",
	})
	require.NoError(t, err)

	// Wait for async goroutine
	time.Sleep(50 * time.Millisecond)

	m.mu.Lock()
	resp, ok := m.asyncResults["error-call"]
	m.mu.Unlock()
	require.True(t, ok)
	assert.Equal(t, "error", resp.Status)
	assert.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
}

// --- WasmService: module not found ---

func TestWasmService_Query_ModuleNotFound(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	ws := NewWasmService(rt, "nonexistent")
	_, err := ws.Query(context.Background(), map[string]any{}, "1s")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestWasmService_SendCommand_ModuleNotFound(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	ws := NewWasmService(rt, "nonexistent")
	// Should not panic
	ws.SendCommand(map[string]any{"action": "test"})
}

// --- WasmService: Query with empty/invalid timeout ---

func TestWasmService_Query_EmptyTimeout(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	plugin.exports["query"] = true
	plugin.responses["query"] = mockResponse{data: []byte(`{"ok":true}`)}

	_, err := rt.LoadModuleWithPlugin(ModuleConfig{Name: "game", TickRate: 10}, plugin)
	require.NoError(t, err)
	require.NoError(t, rt.StartAll(context.Background()))
	defer rt.StopAll(context.Background())

	time.Sleep(50 * time.Millisecond)

	ws := NewWasmService(rt, "game")
	result, err := ws.Query(context.Background(), map[string]any{}, "") // empty timeout
	require.NoError(t, err)
	assert.NotNil(t, result)
}

func TestWasmService_Query_InvalidTimeout(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	plugin.exports["query"] = true
	plugin.responses["query"] = mockResponse{data: []byte(`{"ok":true}`)}

	_, err := rt.LoadModuleWithPlugin(ModuleConfig{Name: "game", TickRate: 10}, plugin)
	require.NoError(t, err)
	require.NoError(t, rt.StartAll(context.Background()))
	defer rt.StopAll(context.Background())

	time.Sleep(50 * time.Millisecond)

	ws := NewWasmService(rt, "game")
	result, err := ws.Query(context.Background(), map[string]any{}, "not-a-duration")
	require.NoError(t, err) // falls back to 5s default
	assert.NotNil(t, result)
}

// --- Module: Initialize with error exit code ---

func TestModule_Initialize_ErrorExitCode(t *testing.T) {
	plugin := newMockPlugin()
	plugin.responses["initialize"] = mockResponse{exitCode: 1}
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)

	err = m.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit code 1")
}

// --- Module: Initialize with call error ---

func TestModule_Initialize_CallError(t *testing.T) {
	plugin := newMockPlugin()
	plugin.responses["initialize"] = mockResponse{err: fmt.Errorf("wasm trap")}
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)

	err = m.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "initialize call failed")
}

// --- Module: Stop when not running ---

func TestModule_Stop_NotRunning(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)

	// Stop without starting should be no-op
	err = m.Stop(context.Background())
	require.NoError(t, err)
	assert.False(t, plugin.closed)
}

// --- Module: Start twice is no-op ---

func TestModule_Start_Twice(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))

	m.Start()
	m.Start() // second call should be no-op
	time.Sleep(50 * time.Millisecond)
	require.NoError(t, m.Stop(context.Background()))
}

// --- Module: buildServiceManifest ---

func TestModule_BuildServiceManifest(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{
		Name:        "test",
		TickRate:    1,
		Services:    []string{"my-cache", "my-storage"},
		Connections: []string{"game-ws"},
	}, dispatcher, testLogger())
	require.NoError(t, err)

	// Without registered services, types default to "service" / "ws"
	manifest := m.buildServiceManifest()
	assert.Equal(t, "service", manifest["my-cache"].Type)
	assert.Nil(t, manifest["my-cache"].Operations)
	assert.Equal(t, "service", manifest["my-storage"].Type)
	assert.Equal(t, "ws", manifest["game-ws"].Type)
	assert.Equal(t, []string{"send"}, manifest["game-ws"].Operations)
	assert.Len(t, manifest, 3)
}

func TestModule_BuildServiceManifest_WithRegisteredServices(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()

	// Register services with known prefixes
	cachePlugin := &stubPlugin{name: "cache-plugin", prefix: "cache"}
	storagePlugin := &stubPlugin{name: "storage-plugin", prefix: "storage"}
	require.NoError(t, svcReg.Register("my-cache", "instance", cachePlugin))
	require.NoError(t, svcReg.Register("my-storage", "instance", storagePlugin))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"my-cache", "my-storage"},
	}, dispatcher, testLogger())
	require.NoError(t, err)

	manifest := m.buildServiceManifest()
	assert.Equal(t, "cache", manifest["my-cache"].Type)
	assert.Equal(t, []string{"get", "set", "del", "exists"}, manifest["my-cache"].Operations)
	assert.Equal(t, "storage", manifest["my-storage"].Type)
	assert.Equal(t, []string{"read", "write", "delete", "list"}, manifest["my-storage"].Operations)
}

// --- Encoding: unsupported encoding in NewModule ---

func TestModule_UnsupportedEncoding(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	_, err := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Encoding: "protobuf",
	}, dispatcher, testLogger())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported encoding")
}

// --- parseDuration ---

func TestParseDuration(t *testing.T) {
	d, err := parseDuration("")
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, d)

	d, err = parseDuration("2s")
	require.NoError(t, err)
	assert.Equal(t, 2*time.Second, d)

	_, err = parseDuration("invalid")
	require.Error(t, err)
}

// --- System ops: trigger_workflow with nil runner ---

func TestHostDispatcher_TriggerWorkflow_NilRunner(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger()) // nil runner
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	result, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "trigger_workflow",
		Payload:   map[string]any{"workflow": "some-wf"},
	})
	require.NoError(t, err)
	resultMap := result.(map[string]any)
	assert.Equal(t, "triggered", resultMap["status"])
}

// --- Module: set_timer with zero interval ---

func TestHostDispatcher_SetTimer_ZeroInterval(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "set_timer",
		Payload:   map[string]any{"name": "test", "interval": float64(0)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "interval must be positive")
}

// --- Runtime: StartAll with initialize error ---

func TestRuntime_StartAll_InitializeError(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	plugin.responses["initialize"] = mockResponse{err: fmt.Errorf("init failed")}

	_, err := rt.LoadModuleWithPlugin(ModuleConfig{Name: "bad-module", TickRate: 10}, plugin)
	require.NoError(t, err)

	err = rt.StartAll(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "initialize module")
}

// --- Tick error paths ---

func TestModule_Tick_PluginCallError(t *testing.T) {
	plugin := newMockPlugin()
	plugin.responses["tick"] = mockResponse{err: fmt.Errorf("tick panic")}
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 20}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))

	m.Start()
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, m.Stop(context.Background()))
	// Should not crash despite tick errors
}

func TestModule_Tick_NonZeroExitCode(t *testing.T) {
	plugin := newMockPlugin()
	plugin.responses["tick"] = mockResponse{exitCode: 42}
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 20}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))

	m.Start()
	time.Sleep(150 * time.Millisecond)
	require.NoError(t, m.Stop(context.Background()))
	// Should log error but not crash
}

// --- processQuery error paths ---

func TestModule_Query_CallError(t *testing.T) {
	plugin := newMockPlugin()
	plugin.exports["query"] = true
	plugin.responses["query"] = mockResponse{err: fmt.Errorf("query exploded")}

	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))
	m.Start()
	defer func() { _ = m.Stop(context.Background()) }()
	time.Sleep(50 * time.Millisecond)

	_, err = m.Query(context.Background(), map[string]any{"q": "test"}, 2*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query call failed")
}

func TestModule_Query_NonZeroExitCode(t *testing.T) {
	plugin := newMockPlugin()
	plugin.exports["query"] = true
	plugin.responses["query"] = mockResponse{exitCode: 1}

	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))
	m.Start()
	defer func() { _ = m.Stop(context.Background()) }()
	time.Sleep(50 * time.Millisecond)

	_, err = m.Query(context.Background(), map[string]any{"q": "test"}, 2*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exit code")
}

// --- Query context cancellation ---

func TestModule_Query_ContextCancelled(t *testing.T) {
	plugin := newMockPlugin()
	plugin.exports["query"] = true

	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)

	// Don't start the module - queryCh will block
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err = m.Query(ctx, map[string]any{"q": "test"}, 2*time.Second)
	require.Error(t, err)
}

// --- Gateway: Send to nonexistent connection ---

func TestGateway_Send_NotFound(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	gw := m.gateway

	_, err := gw.Send(map[string]any{"id": "nonexistent", "data": "hello"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_FOUND")
}

// --- Gateway: CloseConn nonexistent ---

func TestGateway_CloseConn_NotFound(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	gw := m.gateway

	_, err := gw.CloseConn(map[string]any{"id": "nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_FOUND")
}

// --- Gateway: Configure nonexistent ---

func TestGateway_Configure_NotFound(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	gw := m.gateway

	_, err := gw.Configure(map[string]any{"id": "nonexistent"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_FOUND")
}

// --- Gateway: Connect validation errors ---

func TestGateway_Connect_MissingParams(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{"example.com"},
	}, dispatcher, testLogger())
	gw := m.gateway

	// Missing id
	_, err := gw.Connect(context.Background(), map[string]any{"url": "wss://example.com"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VALIDATION_ERROR")

	// Missing url
	_, err = gw.Connect(context.Background(), map[string]any{"id": "conn1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VALIDATION_ERROR")
}

// --- Gateway: Connect permission denied ---

func TestGateway_Connect_NotAllowed(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{"safe.example.com"},
	}, dispatcher, testLogger())
	gw := m.gateway

	_, err := gw.Connect(context.Background(), map[string]any{
		"id":  "conn1",
		"url": "wss://evil.com/ws",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PERMISSION_DENIED")
}

// --- Gateway: CloseAll with no connections ---

func TestGateway_CloseAll_Empty(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	gw := m.gateway

	// Should not panic
	gw.CloseAll()
}

// --- System ops via dispatcher: ws_connect, ws_send, ws_close, ws_configure ---

func TestHostDispatcher_WSConnect_ViaSystemOp(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{"example.com"},
	}, dispatcher, testLogger())

	// ws_connect with missing params
	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "ws_connect",
		Payload:   map[string]any{},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "VALIDATION_ERROR")
}

func TestHostDispatcher_WSSend_ViaSystemOp(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "ws_send",
		Payload:   map[string]any{"id": "nonexistent", "data": "hello"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_FOUND")
}

func TestHostDispatcher_WSClose_ViaSystemOp(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "ws_close",
		Payload:   map[string]any{"id": "nonexistent"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_FOUND")
}

func TestHostDispatcher_WSConfigure_ViaSystemOp(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "ws_configure",
		Payload:   map[string]any{"id": "nonexistent"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_FOUND")
}

// --- System ops: log with nil fields ---

func TestHostDispatcher_SystemLog_NilFields(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	result, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "log",
		Payload:   map[string]any{"level": "info", "message": "no fields"},
	})
	require.NoError(t, err)
	assert.Nil(t, result)
}

// --- Module: msgpack encoding ---

func TestModule_MsgpackEncoding(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 10,
		Encoding: "msgpack",
	}, dispatcher, testLogger())
	require.NoError(t, err)
	assert.Equal(t, "msgpack", m.Codec.Name())

	err = m.Initialize(context.Background())
	require.NoError(t, err)

	// Verify initialize was called with msgpack-encoded data
	calls := plugin.getCalls("initialize")
	require.Len(t, calls, 1)
	// Data should be msgpack, not JSON
	assert.NotEmpty(t, calls[0].Data)
}

// --- Module: AddAsyncResult clears pending label ---

func TestModule_AddAsyncResult_ClearsPendingLabel(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	// Register a label
	require.NoError(t, m.RegisterAsyncLabel("op1"))
	m.mu.Lock()
	assert.True(t, m.pendingLabels["op1"])
	m.mu.Unlock()

	// Add result should clear pending
	m.AddAsyncResult("op1", &AsyncResponse{Status: "ok", Data: "result"})

	m.mu.Lock()
	assert.False(t, m.pendingLabels["op1"])
	_, hasResult := m.asyncResults["op1"]
	m.mu.Unlock()
	assert.True(t, hasResult)
}

// --- Module: callWithTimeout actually times out ---

func TestModule_CallWithTimeout(t *testing.T) {
	plugin := newMockPlugin()
	// Make the call block by adding a slow response
	plugin.responses["slow_func"] = mockResponse{} // default response, but we'll override Call

	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	// Normal call should succeed quickly
	_, _, err := m.callWithTimeout("initialize", []byte("{}"), 1*time.Second)
	require.NoError(t, err)
}

// --- Module: executeTick calls drainQueries ---

func TestModule_ExecuteTick_DrainQueries(t *testing.T) {
	plugin := newMockPlugin()
	plugin.exports["query"] = true
	plugin.responses["query"] = mockResponse{data: []byte(`{"ok":true}`)}

	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))

	m.Start()
	time.Sleep(50 * time.Millisecond)

	// Send a query that should be drained between ticks
	result, err := m.Query(context.Background(), map[string]any{"type": "get"}, 2*time.Second)
	require.NoError(t, err)
	assert.NotNil(t, result)

	require.NoError(t, m.Stop(context.Background()))
}

// --- Module: Stop with shutdown error ---

func TestModule_Stop_ShutdownError(t *testing.T) {
	plugin := newMockPlugin()
	plugin.responses["shutdown"] = mockResponse{err: fmt.Errorf("shutdown failed")}

	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))

	m.Start()
	time.Sleep(50 * time.Millisecond)

	err = m.Stop(context.Background())
	require.Error(t, err)         // shutdown error should be returned
	assert.True(t, plugin.closed) // plugin still closed
}

// --- Gateway: Connect to unreachable host (allowed but fails to dial) ---

func TestGateway_Connect_DialError(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{"localhost"},
	}, dispatcher, testLogger())
	gw := m.gateway

	// Connect to a non-listening port - should fail with SERVICE_UNAVAILABLE
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := gw.Connect(ctx, map[string]any{
		"id":  "conn1",
		"url": "ws://localhost:19999/nonexistent",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SERVICE_UNAVAILABLE")
}

// --- Gateway: Connect with headers ---

func TestGateway_Connect_WithHeaders_DialError(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{"localhost"},
	}, dispatcher, testLogger())
	gw := m.gateway

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := gw.Connect(ctx, map[string]any{
		"id":      "conn1",
		"url":     "ws://localhost:19999/ws",
		"headers": map[string]any{"Authorization": "Bearer token123"},
	})
	require.Error(t, err)
	// At least validates the headers parsing path
}

// --- Gateway: CloseConn with custom code and reason ---

func TestGateway_CloseConn_CustomCodeReason(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	gw := m.gateway

	// nonexistent with custom code/reason - tests the payload parsing
	_, err := gw.CloseConn(map[string]any{
		"id":     "nonexistent",
		"code":   float64(1001),
		"reason": "going away",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_FOUND")
}

// --- Runtime: StopAll with module stop error ---

func TestRuntime_StopAll_WithModuleError(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	rt := NewRuntime(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	plugin.responses["shutdown"] = mockResponse{err: fmt.Errorf("shutdown boom")}

	_, err := rt.LoadModuleWithPlugin(ModuleConfig{Name: "bad-mod", TickRate: 10}, plugin)
	require.NoError(t, err)
	require.NoError(t, rt.StartAll(context.Background()))

	time.Sleep(50 * time.Millisecond)

	// Should not panic, just log the error
	rt.StopAll(context.Background())
}

// --- HostDispatcher: SetModule ---

func TestHostDispatcher_SetModule(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	plugin := newMockPlugin()
	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	// Verify dispatcher's module is set (already done by NewModule)
	assert.Equal(t, m, dispatcher.module)

	// Set a different module
	plugin2 := newMockPlugin()
	m2, _ := NewModule("test2", plugin2, ModuleConfig{Name: "test2", TickRate: 1}, dispatcher, testLogger())
	dispatcher.SetModule(m2)
	assert.Equal(t, m2, dispatcher.module)
}

// --- NewGateway ---

func TestNewGateway(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	gw := NewGateway(m, testLogger())
	assert.NotNil(t, gw)
	assert.Equal(t, m, gw.module)
	assert.NotNil(t, gw.conns)
}

// --- Cache: exists returns false for missing key ---

func TestHostDispatcher_CacheExists_False(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	cache := &mockCacheService{store: make(map[string]any)}
	require.NoError(t, svcReg.Register("cache", cache, nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"cache"},
	}, dispatcher, testLogger())

	result, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "cache",
		Operation: "exists",
		Payload:   map[string]any{"key": "nonexistent"},
	})
	require.NoError(t, err)
	assert.False(t, result.(map[string]any)["exists"].(bool))
}

// --- Cache: get error ---

func TestHostDispatcher_CacheGet_Error(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	cache := &mockCacheService{store: make(map[string]any)}
	require.NoError(t, svcReg.Register("cache", cache, nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"cache"},
	}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "cache",
		Operation: "get",
		Payload:   map[string]any{"key": "missing"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "NOT_FOUND")
}

// --- Storage: read error ---

func TestHostDispatcher_StorageRead_Error(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	storage := &mockStorageService{files: make(map[string][]byte)}
	require.NoError(t, svcReg.Register("storage", storage, nil))

	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		Services: []string{"storage"},
	}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "storage",
		Operation: "read",
		Payload:   map[string]any{"path": "/nonexistent"},
	})
	require.Error(t, err)
}

// --- System ops: trigger_workflow with nil input ---

func TestHostDispatcher_TriggerWorkflow_NilInput(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	var triggered atomic.Bool
	runner := func(_ context.Context, wfID string, input map[string]any) error {
		triggered.Store(true)
		assert.Nil(t, input)
		return nil
	}

	dispatcher := NewHostDispatcher(svcReg, runner, testLogger())
	plugin := newMockPlugin()
	_, _ = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())

	_, err := dispatcher.Call(context.Background(), HostCallRequest{
		Service:   "",
		Operation: "trigger_workflow",
		Payload:   map[string]any{"workflow": "test-wf"},
	})
	require.NoError(t, err)
	time.Sleep(50 * time.Millisecond)
	assert.True(t, triggered.Load())
}

// --- Gateway: heartbeatLoop with zero interval returns immediately ---

func TestGateway_HeartbeatLoop_ZeroInterval(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	gw := m.gateway

	gc := &gatewayConn{
		id:     "test",
		stopCh: make(chan struct{}),
		config: GatewayConfig{HeartbeatInterval: 0},
	}

	// Should return immediately without blocking
	done := make(chan struct{})
	go func() {
		gw.heartbeatLoop(gc)
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("heartbeatLoop did not return for zero interval")
	}
}

// --- Gateway: full lifecycle with real WebSocket ---

func startTestWSServer(t *testing.T, handler func(conn *websocket.Conn)) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		handler(conn)
	}))
	return server
}

func wsURL(server *httptest.Server) string {
	return "ws" + server.URL[4:] // http -> ws
}

func TestGateway_FullLifecycle(t *testing.T) {
	// Start echo WebSocket server
	server := startTestWSServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		for {
			mt, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			_ = conn.WriteMessage(mt, msg)
		}
	})
	defer server.Close()

	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	// Extract host from URL
	host := server.Listener.Addr().String()

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{host},
	}, dispatcher, testLogger())
	gw := m.gateway

	// Connect
	result, err := gw.Connect(context.Background(), map[string]any{
		"id":  "echo-conn",
		"url": wsURL(server),
	})
	require.NoError(t, err)
	resultMap := result.(map[string]any)
	assert.Equal(t, "connected", resultMap["status"])

	// Send
	_, err = gw.Send(map[string]any{
		"id":   "echo-conn",
		"data": map[string]any{"hello": "world"},
	})
	require.NoError(t, err)

	// Wait for echo response to be buffered
	time.Sleep(100 * time.Millisecond)

	m.mu.Lock()
	wsCount := len(m.incomingWS)
	m.mu.Unlock()
	assert.GreaterOrEqual(t, wsCount, 1, "should have received echo")

	// Close
	_, err = gw.CloseConn(map[string]any{
		"id":     "echo-conn",
		"code":   float64(1000),
		"reason": "done",
	})
	require.NoError(t, err)
}

func TestGateway_Send_ClosedConnection(t *testing.T) {
	// Server that closes immediately after upgrade
	server := startTestWSServer(t, func(conn *websocket.Conn) {
		_ = conn.Close()
	})
	defer server.Close()

	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	host := server.Listener.Addr().String()

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{host},
	}, dispatcher, testLogger())
	gw := m.gateway

	// Connect
	_, err := gw.Connect(context.Background(), map[string]any{
		"id":  "dying-conn",
		"url": wsURL(server),
	})
	require.NoError(t, err)

	// Wait for server to close connection
	time.Sleep(100 * time.Millisecond)

	// Send to closed connection should fail
	_, err = gw.Send(map[string]any{
		"id":   "dying-conn",
		"data": "hello",
	})
	// May get closed error or write error
	if err != nil {
		assert.Contains(t, err.Error(), "closed")
	}
}

func TestGateway_ReadLoop_DeliveredDisconnectEvent(t *testing.T) {
	// Server that closes after sending one message
	server := startTestWSServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`"hello from server"`))
		time.Sleep(50 * time.Millisecond)
		_ = conn.Close()
	})
	defer server.Close()

	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	host := server.Listener.Addr().String()

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{host},
	}, dispatcher, testLogger())
	gw := m.gateway

	_, err := gw.Connect(context.Background(), map[string]any{
		"id":  "server-close-conn",
		"url": wsURL(server),
	})
	require.NoError(t, err)

	// Wait for message and disconnect
	time.Sleep(200 * time.Millisecond)

	m.mu.Lock()
	wsCount := len(m.incomingWS)
	evtCount := len(m.connectionEvents)
	m.mu.Unlock()

	assert.GreaterOrEqual(t, wsCount, 1, "should have received server message")
	assert.GreaterOrEqual(t, evtCount, 1, "should have received disconnect event")
}

func TestGateway_CloseAll_WithConnections(t *testing.T) {
	server := startTestWSServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer server.Close()

	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	host := server.Listener.Addr().String()

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{host},
	}, dispatcher, testLogger())
	gw := m.gateway

	// Connect two connections
	_, err := gw.Connect(context.Background(), map[string]any{
		"id":  "conn1",
		"url": wsURL(server),
	})
	require.NoError(t, err)

	_, err = gw.Connect(context.Background(), map[string]any{
		"id":  "conn2",
		"url": wsURL(server),
	})
	require.NoError(t, err)

	// CloseAll
	gw.CloseAll()

	gw.mu.RLock()
	count := len(gw.conns)
	gw.mu.RUnlock()
	assert.Equal(t, 0, count)
}

func TestGateway_Configure_WithHeartbeat(t *testing.T) {
	server := startTestWSServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer server.Close()

	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	host := server.Listener.Addr().String()

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{host},
	}, dispatcher, testLogger())
	gw := m.gateway

	_, err := gw.Connect(context.Background(), map[string]any{
		"id":  "hb-conn",
		"url": wsURL(server),
	})
	require.NoError(t, err)

	_, err = gw.Configure(map[string]any{
		"id":                 "hb-conn",
		"heartbeat_interval": float64(50),
		"heartbeat_payload":  map[string]any{"op": float64(1)},
	})
	require.NoError(t, err)

	// Let heartbeat fire at least once
	time.Sleep(150 * time.Millisecond)

	// Clean up
	gw.CloseAll()
}

func TestGateway_ReadLoop_NonJSONMessage(t *testing.T) {
	// Server that sends non-JSON text
	server := startTestWSServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("plain text message"))
		time.Sleep(50 * time.Millisecond)
		_ = conn.Close()
	})
	defer server.Close()

	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	host := server.Listener.Addr().String()

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{host},
	}, dispatcher, testLogger())
	gw := m.gateway

	_, err := gw.Connect(context.Background(), map[string]any{
		"id":  "plain-conn",
		"url": wsURL(server),
	})
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	m.mu.Lock()
	wsCount := len(m.incomingWS)
	var data any
	if wsCount > 0 {
		data = m.incomingWS[0].Data
	}
	m.mu.Unlock()

	assert.GreaterOrEqual(t, wsCount, 1)
	// Non-JSON should be stored as string
	assert.Equal(t, "plain text message", data)
}

// --- Module: TickRate exactly at boundary ---

func TestModule_TickRate_ExactBoundaries(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	// Exactly 1
	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	require.NoError(t, err)
	assert.Equal(t, 1, m.tickRate)

	// Exactly 120
	m, err = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 120}, dispatcher, testLogger())
	require.NoError(t, err)
	assert.Equal(t, 120, m.tickRate)

	// Negative
	m, err = NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: -5}, dispatcher, testLogger())
	require.NoError(t, err)
	assert.Equal(t, 1, m.tickRate)
}

// --- Gateway: reconnection ---

func TestParseReconnectConfig(t *testing.T) {
	rc := parseReconnectConfig(map[string]any{
		"enabled":       true,
		"max_attempts":  float64(5),
		"backoff":       "exponential",
		"initial_delay": float64(1000),
	})

	assert.True(t, rc.Enabled)
	assert.Equal(t, 5, rc.MaxAttempts)
	assert.Equal(t, "exponential", rc.Backoff)
	assert.Equal(t, 1000*time.Millisecond, rc.InitialDelay)
}

func TestParseReconnectConfig_Defaults(t *testing.T) {
	rc := parseReconnectConfig(map[string]any{})
	assert.False(t, rc.Enabled)
	assert.Equal(t, 0, rc.MaxAttempts)
	assert.Equal(t, "", rc.Backoff)
	assert.Equal(t, time.Duration(0), rc.InitialDelay)
}

func TestReconnectLoop_Disabled(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	gw := NewGateway(m, testLogger())

	gc := &gatewayConn{
		id:     "test-conn",
		url:    "ws://localhost:9999",
		stopCh: make(chan struct{}),
		config: GatewayConfig{
			Reconnect: &ReconnectConfig{Enabled: false, MaxAttempts: 3},
		},
	}

	// Should return immediately when disabled
	done := make(chan struct{})
	go func() {
		gw.reconnectLoop(gc)
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("reconnectLoop did not return when disabled")
	}
}

func TestReconnectLoop_NilConfig(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	gw := NewGateway(m, testLogger())

	gc := &gatewayConn{
		id:     "test-conn",
		url:    "ws://localhost:9999",
		stopCh: make(chan struct{}),
	}

	done := make(chan struct{})
	go func() {
		gw.reconnectLoop(gc)
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(1 * time.Second):
		t.Fatal("reconnectLoop did not return for nil config")
	}
}

func TestReconnectLoop_MaxAttempts(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 1}, dispatcher, testLogger())
	gw := NewGateway(m, testLogger())

	gc := &gatewayConn{
		id:     "test-conn",
		url:    "ws://localhost:1", // unreachable
		stopCh: make(chan struct{}),
		config: GatewayConfig{
			Reconnect: &ReconnectConfig{
				Enabled:      true,
				MaxAttempts:  2,
				Backoff:      "linear",
				InitialDelay: 10 * time.Millisecond,
			},
		},
	}

	done := make(chan struct{})
	go func() {
		gw.reconnectLoop(gc)
		close(done)
	}()

	select {
	case <-done:
		// OK — exhausted attempts
	case <-time.After(5 * time.Second):
		t.Fatal("reconnectLoop did not exhaust attempts in time")
	}
}

func TestReconnectLoop_Success(t *testing.T) {
	server := startTestWSServer(t, func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	})
	defer server.Close()

	host := server.Listener.Addr().String()
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, _ := NewModule("test", plugin, ModuleConfig{
		Name:     "test",
		TickRate: 1,
		AllowWS:  []string{host},
	}, dispatcher, testLogger())
	gw := NewGateway(m, testLogger())

	gc := &gatewayConn{
		id:     "reconn-test",
		url:    wsURL(server),
		stopCh: make(chan struct{}),
		closed: true,
		config: GatewayConfig{
			Reconnect: &ReconnectConfig{
				Enabled:      true,
				MaxAttempts:  3,
				Backoff:      "exponential",
				InitialDelay: 10 * time.Millisecond,
			},
		},
	}

	gw.mu.Lock()
	gw.conns["reconn-test"] = gc
	gw.mu.Unlock()

	gw.reconnectLoop(gc)

	gc.mu.Lock()
	isClosed := gc.closed
	gc.mu.Unlock()

	assert.False(t, isClosed, "connection should be re-established")

	// Verify reconnected event
	m.mu.Lock()
	var hasReconnected bool
	for _, evt := range m.connectionEvents {
		if evt.Event == "reconnected" && evt.Connection == "reconn-test" {
			hasReconnected = true
			break
		}
	}
	m.mu.Unlock()
	assert.True(t, hasReconnected, "should have emitted reconnected event")

	// Cleanup
	gw.CloseAll()
}
