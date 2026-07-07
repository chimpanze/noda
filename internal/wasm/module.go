package wasm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
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
	CallWithContext(ctx context.Context, name string, data []byte) (uint32, []byte, error)
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

	// shutdownCtx is a stable context for the module's lifetime, cancelled once
	// by Stop() to unblock any in-flight callWithTimeout calls. Unlike the old
	// lifecycleCtx, it is never reset — callers needing a fresh context (e.g.
	// the shutdown export call in Stop) create their own.
	shutdownCtx      context.Context
	shutdownCancel   context.CancelFunc
	outstandingCalls sync.WaitGroup

	// stopping is set by Stop() before the async-result maps are cleared.
	// AddAsyncResult checks it to drop late-arriving writes silently.
	stopping atomic.Bool

	// failed is set once a guest call errors out (e.g. a hung call that hit
	// its timeout). Once set, the tick loop exits and Query/SendCommand fail
	// fast instead of queuing work a dead loop will never service.
	failed atomic.Bool

	// Outbound gateway connections
	gateway *Gateway

	// Service dispatcher for host calls
	dispatcher *HostDispatcher

	// Query serialization
	queryCh chan queryRequest

	// tickDone is closed when the tick loop goroutine exits
	tickDone chan struct{}
}

type timerEntry struct {
	interval time.Duration
	nextFire time.Time
}

type queryRequest struct {
	// target is the guest export to call. Defaults to "query"; Task 7 will
	// set it per-request to support full command/query routing.
	target string
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

	// Compute default tick timeout: 10x the tick budget
	if cfg.TickTimeout == 0 {
		budget := time.Second / time.Duration(tickRate)
		cfg.TickTimeout = budget * 10
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
		tickDone:      make(chan struct{}),
	}

	m.shutdownCtx, m.shutdownCancel = context.WithCancel(context.Background())

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

	exitCode, _, err := m.callWithTimeout(m.shutdownCtx, "initialize", data, wasmCallTimeout)
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

	go func() { m.tickLoop(); close(m.tickDone) }()
}

// Stop halts the tick loop and calls shutdown. After Stop returns, the
// Module is single-use: the stopping flag is not reset, so any subsequent
// call to AddAsyncResult silently drops. Construct a new Module via
// Runtime.LoadModule to restart.
func (m *Module) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	// Set under mu so SendCommand's mu-held check is race-free: any
	// SendCommand that acquires the lock after this point observes
	// stopping=true and won't outstandingCalls.Add — every Add strictly
	// happens-before the Wait below. AddAsyncResult also keys off this
	// to drop late writes.
	m.stopping.Store(true)
	close(m.stopCh)
	m.mu.Unlock()

	// Cancel the shutdown context to unblock any in-flight callWithTimeout calls
	m.shutdownCancel()

	// Wait for the tick loop goroutine to fully exit before touching the plugin
	<-m.tickDone

	// Call shutdown with timeout to prevent hung exports from blocking lifecycle.
	// Use a fresh context since shutdownCtx is already cancelled above.
	data, _ := m.Codec.Marshal(map[string]any{})
	ctx2, cancel2 := context.WithTimeout(context.Background(), wasmCallTimeout)
	defer cancel2()
	_, _, err := m.callWithTimeout(ctx2, "shutdown", data, wasmCallTimeout)

	// Wait for outstanding async-call goroutines BEFORE clearing the maps
	// they write into. With the Add(1)/Done() wrapping in CallAsync this
	// actually waits for the right things; without it the wait was a no-op.
	// Invariant: no Add can start while this Wait sits at counter zero —
	// SendCommand checks stopping under mu; the hostapi.go CallAsync Add
	// runs inside a guest export, which is serialized before this point
	// (tick loop exited via tickDone; shutdown call above returned); and
	// the nested trigger_workflow Add runs on the CallAsync goroutine,
	// whose own counter entry stays >0 until after the nested Add. A new
	// Add site must preserve one of these properties or the Wait panics.
	done := make(chan struct{})
	go func() {
		m.outstandingCalls.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		// Timed out — at most one goroutine leaks but the maps are about
		// to be cleared so any late write goes into a fresh empty map.
		// The stopping flag (set above) makes AddAsyncResult drop quietly.
	}

	// Clear pending async state
	m.mu.Lock()
	m.pendingLabels = make(map[string]bool)
	m.asyncResults = make(map[string]*AsyncResponse)
	m.mu.Unlock()

	// Close gateway connections
	m.gateway.CloseAll()

	// Close plugin
	_ = m.Plugin.Close(ctx)

	return err
}

// Query calls the module's query export synchronously, serialized with ticks.
func (m *Module) Query(ctx context.Context, queryData any, timeout time.Duration) (any, error) {
	if m.failed.Load() {
		return nil, fmt.Errorf("module %q has failed", m.Name)
	}

	data, err := m.Codec.Marshal(queryData)
	if err != nil {
		return nil, fmt.Errorf("marshal query: %w", err)
	}

	req := queryRequest{
		target: "query",
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
	if m.failed.Load() {
		m.Logger.Error("dropping command on failed module", "module", m.Name)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Once Stop has begun (stopping set under this same mutex), adding to
	// outstandingCalls would race Stop's Wait-at-zero — WaitGroup misuse.
	// Buffering would be equally pointless: no tick will drain it.
	if m.stopping.Load() {
		m.Logger.Warn("dropping command on stopped module", "module", m.Name)
		return
	}

	// If module exports "command", call it directly (between ticks — queued via queryCh)
	if m.Plugin.FunctionExists("command") {
		cmdData, err := m.Codec.Marshal(data)
		if err != nil {
			m.Logger.Error("marshal command failed", "module", m.Name, "error", err)
			return
		}
		// Queue as a query-like request to serialize with ticks (with timeout)
		m.outstandingCalls.Add(1)
		go func() {
			defer m.outstandingCalls.Done()
			req := queryRequest{target: "command", data: cmdData, result: make(chan queryResponse, 1)}
			select {
			case m.queryCh <- req:
				select {
				case <-req.result:
					// processed successfully
				case <-time.After(wasmCallTimeout):
					m.Logger.Error("command result timed out", "module", m.Name)
				case <-m.stopCh:
					// module stopping, abandon
				}
			case <-time.After(wasmCallTimeout):
				m.Logger.Error("command queue timed out", "module", m.Name)
			case <-m.stopCh:
				// module stopping, abandon
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
// Drops the write silently (debug log) if the module is in the post-Stop
// state — protects against the rare race where an async goroutine wakes
// after Stop's outstandingCalls wait has timed out.
func (m *Module) AddAsyncResult(label string, result *AsyncResponse) {
	if m.stopping.Load() {
		slog.Debug("dropping async result on stopped module", "module", m.Name, "label", label)
		return
	}
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

// IsWorkflowAllowed checks if the module can trigger a workflow.
// Returns false if AllowedWorkflows is empty (deny by default).
func (m *Module) IsWorkflowAllowed(workflowID string) bool {
	for _, w := range m.Config.AllowedWorkflows {
		if w == workflowID {
			return true
		}
	}
	return false
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

// errGuestInterrupted is a sentinel wrapped into the error returned by
// callWithTimeout when the guest call was interrupted (its context deadline
// expired or the parent shutdown context was cancelled). In that case wazero
// has actually terminated the guest instance, so every subsequent call would
// also fail — callers must stop and mark the module failed. Any other
// non-nil error from CallWithContext (a wasm trap, or a guest that returned
// noda.Fail/noda.FailMsg) leaves the instance alive and recoverable, so
// callers should log it and keep going instead of tearing down the module.
var errGuestInterrupted = errors.New("guest call interrupted")

// callWithTimeout calls a guest export synchronously with a per-call deadline.
// It runs inline on the caller's goroutine (the tick loop during running),
// so only one goroutine ever touches the plugin. With extism manifest.Timeout
// set, a context deadline actually terminates the guest.
func (m *Module) callWithTimeout(parent context.Context, name string, data []byte, timeout time.Duration) (uint32, []byte, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	exitCode, out, err := m.Plugin.CallWithContext(ctx, name, data)
	if err != nil && ctx.Err() != nil {
		// The call's own deadline expired, or the parent shutdown context was
		// cancelled mid-call — either way the guest instance is now closed.
		return exitCode, out, fmt.Errorf("%s call interrupted (%v): %w: %w", name, ctx.Err(), err, errGuestInterrupted)
	}
	return exitCode, out, err
}

// markFailed marks the module as failed, causing the tick loop to exit and
// subsequent Query/SendCommand calls to fail fast. Idempotent — only the
// first call logs.
func (m *Module) markFailed(reason string) {
	if m.failed.CompareAndSwap(false, true) {
		m.Logger.Error("wasm module failed; stopping tick loop", "module", m.Name, "reason", reason)
	}
}

func (m *Module) buildServiceManifest() map[string]ServiceManifest {
	manifest := make(map[string]ServiceManifest)
	for _, s := range m.Config.Services {
		svcType := "service"
		var ops []string
		if prefix, ok := m.dispatcher.services.GetPrefix(s); ok {
			svcType = prefix
			ops = operationsForPrefix(prefix)
		}
		manifest[s] = ServiceManifest{Type: svcType, Operations: ops}
	}
	for _, c := range m.Config.Connections {
		cType := "ws"
		if prefix, ok := m.dispatcher.services.GetPrefix(c); ok {
			cType = prefix
		}
		manifest[c] = ServiceManifest{Type: cType, Operations: operationsForPrefix(cType)}
	}
	return manifest
}

// operationsForPrefix returns the supported operations for a service prefix.
func operationsForPrefix(prefix string) []string {
	switch prefix {
	case "storage":
		return []string{"read", "write", "delete", "list"}
	case "cache":
		return []string{"get", "set", "del", "exists"}
	case "stream":
		return []string{"publish"}
	case "pubsub":
		return []string{"publish"}
	case "ws", "sse":
		return []string{"send"}
	default:
		return nil
	}
}
