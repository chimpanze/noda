package wasm

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	// maxTickRate is the upper bound for Wasm module tick rate in Hz.
	maxTickRate = 120

	// wasmCallTimeout is the maximum time allowed for a single Wasm plugin call
	// (initialize, shutdown, command). Prevents hung modules from blocking the runtime.
	wasmCallTimeout = 30 * time.Second

	// queryChannelBuffer is the buffer size for the query serialization channel.
	queryChannelBuffer = 16
)

// PluginInstance abstracts the Extism plugin for testability.
type PluginInstance interface {
	Call(name string, data []byte) (uint32, []byte, error)
	FunctionExists(name string) bool
	Close(ctx context.Context) error
}

// Module wraps a Wasm plugin instance with runtime state.
type Module struct {
	mu sync.Mutex

	Name   string
	Config ModuleConfig
	Plugin PluginInstance
	Codec  Codec
	Logger *slog.Logger

	// Tick state
	running  bool
	stopCh   chan struct{}
	lastTick time.Time
	tickRate int

	// Event accumulation (protected by mu)
	clientMessages   []ClientMessage
	incomingWS       []IncomingWSMsg
	connectionEvents []ConnectionEvent
	commands         []Command

	// Async responses (protected by mu)
	pendingLabels map[string]bool
	asyncResults  map[string]*AsyncResponse

	// Timers (protected by mu)
	timers map[string]timerEntry

	// Outbound gateway connections
	gateway *Gateway

	// Service dispatcher for host calls
	dispatcher *HostDispatcher

	// Query serialization
	queryCh chan queryRequest
}

type timerEntry struct {
	interval time.Duration
	nextFire time.Time
}

type queryRequest struct {
	data   []byte
	result chan queryResponse
}

type queryResponse struct {
	data []byte
	err  error
}

// NewModule creates a new module with the given plugin and config.
func NewModule(name string, plugin PluginInstance, cfg ModuleConfig, dispatcher *HostDispatcher, logger *slog.Logger) (*Module, error) {
	codec, err := NewCodec(cfg.Encoding)
	if err != nil {
		return nil, err
	}

	tickRate := cfg.TickRate
	if tickRate <= 0 {
		tickRate = 1
	}
	if tickRate > maxTickRate {
		tickRate = maxTickRate
	}

	m := &Module{
		Name:          name,
		Config:        cfg,
		Plugin:        plugin,
		Codec:         codec,
		Logger:        logger,
		tickRate:      tickRate,
		stopCh:        make(chan struct{}),
		pendingLabels: make(map[string]bool),
		asyncResults:  make(map[string]*AsyncResponse),
		timers:        make(map[string]timerEntry),
		queryCh:       make(chan queryRequest, queryChannelBuffer),
	}

	dispatcher.SetModule(m)
	m.dispatcher = dispatcher
	m.gateway = NewGateway(m, logger)

	return m, nil
}

// Initialize calls the module's initialize export.
func (m *Module) Initialize(ctx context.Context) error {
	input := InitializeInput{
		Encoding: m.Codec.Name(),
		Config:   m.Config.Config,
		Services: m.buildServiceManifest(),
	}

	data, err := m.Codec.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal initialize input: %w", err)
	}

	exitCode, _, err := m.callWithTimeout("initialize", data, wasmCallTimeout)
	if err != nil {
		return fmt.Errorf("initialize call failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("initialize returned exit code %d", exitCode)
	}

	return nil
}

// Start begins the tick loop.
func (m *Module) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.lastTick = time.Now()
	m.mu.Unlock()

	go m.tickLoop()
}

// Stop halts the tick loop and calls shutdown.
func (m *Module) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	close(m.stopCh)
	m.mu.Unlock()

	// Call shutdown
	data, _ := m.Codec.Marshal(map[string]any{})
	_, _, err := m.callWithTimeout("shutdown", data, wasmCallTimeout)

	// Close gateway connections
	m.gateway.CloseAll()

	// Close plugin
	_ = m.Plugin.Close(ctx)

	return err
}

// Query calls the module's query export synchronously, serialized with ticks.
func (m *Module) Query(ctx context.Context, queryData any, timeout time.Duration) (any, error) {
	data, err := m.Codec.Marshal(queryData)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	req := queryRequest{
		data:   data,
		result: make(chan queryResponse, 1),
	}

	select {
	case m.queryCh <- req:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp := <-req.result:
		if resp.err != nil {
			return nil, resp.err
		}
		var result any
		if err := m.Codec.Unmarshal(resp.data, &result); err != nil {
			return nil, fmt.Errorf("unmarshal query response: %w", err)
		}
		return result, nil
	case <-timer.C:
		return nil, fmt.Errorf("query timeout after %s", timeout)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SendCommand delivers a command to the module.
func (m *Module) SendCommand(data any) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If module exports "command", call it directly (between ticks — queued via queryCh)
	if m.Plugin.FunctionExists("command") {
		cmdData, err := m.Codec.Marshal(data)
		if err != nil {
			m.Logger.Error("marshal command failed", "module", m.Name, "error", err)
			return
		}
		// Queue as a query-like request to serialize with ticks (with timeout)
		go func() {
			req := queryRequest{data: cmdData, result: make(chan queryResponse, 1)}
			select {
			case m.queryCh <- req:
				<-req.result // wait for completion
			case <-time.After(wasmCallTimeout):
				m.Logger.Error("command queue timed out", "module", m.Name)
			}
		}()
		return
	}

	// Buffer for next tick
	m.commands = append(m.commands, Command{Source: "workflow", Data: data})
}

// AddClientMessage adds a message from a connected client.
func (m *Module) AddClientMessage(msg ClientMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clientMessages = append(m.clientMessages, msg)
}

// AddIncomingWS adds a message from an outbound WebSocket.
func (m *Module) AddIncomingWS(msg IncomingWSMsg) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.incomingWS = append(m.incomingWS, msg)
}

// AddConnectionEvent adds a connection lifecycle event.
func (m *Module) AddConnectionEvent(evt ConnectionEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectionEvents = append(m.connectionEvents, evt)
}

// SetTimer sets a named timer with the given interval.
func (m *Module) SetTimer(name string, intervalMs int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timers[name] = timerEntry{
		interval: time.Duration(intervalMs) * time.Millisecond,
		nextFire: time.Now().Add(time.Duration(intervalMs) * time.Millisecond),
	}
}

// ClearTimer removes a named timer.
func (m *Module) ClearTimer(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.timers, name)
}

// AddAsyncResult stores the result of an async call for the next tick.
func (m *Module) AddAsyncResult(label string, result *AsyncResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.asyncResults[label] = result
	delete(m.pendingLabels, label)
}

// RegisterAsyncLabel marks a label as pending. Returns error if duplicate.
func (m *Module) RegisterAsyncLabel(label string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pendingLabels[label] {
		return fmt.Errorf("duplicate async label: %q", label)
	}
	m.pendingLabels[label] = true
	return nil
}

// IsServiceAllowed checks if the module can access a service.
func (m *Module) IsServiceAllowed(service string) bool {
	if service == "" {
		return true // system operations always allowed
	}
	for _, s := range m.Config.Services {
		if s == service {
			return true
		}
	}
	for _, c := range m.Config.Connections {
		if c == service {
			return true
		}
	}
	return false
}

// callWithTimeout calls a plugin function with a timeout. Returns an error if
// the call doesn't complete within the given duration.
func (m *Module) callWithTimeout(name string, data []byte, timeout time.Duration) (uint32, []byte, error) {
	type callResult struct {
		exitCode uint32
		output   []byte
		err      error
	}
	ch := make(chan callResult, 1)
	go func() {
		exitCode, output, err := m.Plugin.Call(name, data)
		ch <- callResult{exitCode, output, err}
	}()
	select {
	case r := <-ch:
		return r.exitCode, r.output, r.err
	case <-time.After(timeout):
		return 0, nil, fmt.Errorf("%s call timed out after %s", name, timeout)
	}
}

func (m *Module) buildServiceManifest() map[string]ServiceManifest {
	manifest := make(map[string]ServiceManifest)
	for _, s := range m.Config.Services {
		manifest[s] = ServiceManifest{Type: "service"}
	}
	for _, c := range m.Config.Connections {
		manifest[c] = ServiceManifest{Type: "connection"}
	}
	return manifest
}
