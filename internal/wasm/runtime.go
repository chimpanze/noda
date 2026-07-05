package wasm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	extism "github.com/extism/go-sdk"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
)

// Runtime manages all Wasm module instances.
type Runtime struct {
	mu       sync.RWMutex
	modules  map[string]*Module
	services *registry.ServiceRegistry
	runner   api.WorkflowRunner
	logger   *slog.Logger
}

// NewRuntime creates a new Wasm runtime.
func NewRuntime(services *registry.ServiceRegistry, runner api.WorkflowRunner, logger *slog.Logger) *Runtime {
	return &Runtime{
		modules:  make(map[string]*Module),
		services: services,
		runner:   runner,
		logger:   logger,
	}
}

// defaultMaxModuleSize is the default maximum Wasm module file size (50MB).
const defaultMaxModuleSize int64 = 50 * 1024 * 1024

// defaultMemoryPages caps guest linear memory when MemoryPages is unset.
// 256 pages * 64 KiB = 16 MiB (wazero's default is unbounded up to 4 GiB).
const defaultMemoryPages uint32 = 256

// buildManifest constructs the Extism manifest for a module, applying a
// bounded default guest memory cap and the timeout needed to make context
// cancellation actually terminate a running guest call (see wazero's
// WithCloseOnContextDone).
func buildManifest(cfg ModuleConfig, wasmBytes []byte) extism.Manifest {
	manifest := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
		AllowedHosts: cfg.AllowHTTP, // Extism enforces HTTP host whitelist via its built-in HTTP host function
	}

	pages := cfg.MemoryPages
	if pages == 0 {
		pages = defaultMemoryPages
	}
	manifest.Memory = &extism.ManifestMemory{MaxPages: pages}

	// Set the manifest timeout so extism enables wazero's WithCloseOnContextDone,
	// which makes a context deadline/cancellation actually terminate a running
	// guest call rather than just abandoning it. Use the larger of the tick
	// timeout and the general call timeout so no legitimate call is cut short.
	timeoutMs := cfg.TickTimeout
	if timeoutMs < wasmCallTimeout {
		timeoutMs = wasmCallTimeout
	}
	manifest.Timeout = uint64(timeoutMs / time.Millisecond)

	return manifest
}

// LoadModule loads a Wasm module from config and registers it.
func (r *Runtime) LoadModule(ctx context.Context, cfg ModuleConfig) (*Module, error) {
	// Check file size before loading
	maxSize := cfg.MaxModuleSize
	if maxSize <= 0 {
		maxSize = defaultMaxModuleSize
	}
	info, err := os.Stat(cfg.ModulePath)
	if err != nil {
		return nil, fmt.Errorf("stat wasm file %q: %w", cfg.ModulePath, err)
	}
	if info.Size() > maxSize {
		return nil, fmt.Errorf("wasm module %q exceeds size limit: %d bytes > %d bytes max", cfg.ModulePath, info.Size(), maxSize)
	}

	// Load wasm binary
	wasmBytes, err := os.ReadFile(cfg.ModulePath)
	if err != nil {
		return nil, fmt.Errorf("read wasm file %q: %w", cfg.ModulePath, err)
	}

	return r.loadModuleFromBytes(ctx, cfg, wasmBytes)
}

// loadModuleFromBytes loads a module from raw wasm bytes (used by tests too).
func (r *Runtime) loadModuleFromBytes(ctx context.Context, cfg ModuleConfig, wasmBytes []byte) (*Module, error) {
	// Create Extism manifest
	manifest := buildManifest(cfg, wasmBytes)

	// Create host dispatcher
	dispatcher := NewHostDispatcher(r.services, r.runner, r.logger)

	// Build host functions that delegate to the dispatcher.
	// The dispatcher's module reference is set after NewModule (via SetModule),
	// which is safe because host functions only execute during module exports
	// (initialize, tick, query, command) — all called after module creation.
	hostFns := buildHostFunctions(dispatcher, r.logger)

	// Create Extism plugin with host functions
	pluginCfg := extism.PluginConfig{
		EnableWasi: true,
	}
	plugin, err := extism.NewPlugin(ctx, manifest, pluginCfg, hostFns)
	if err != nil {
		return nil, fmt.Errorf("create extism plugin: %w", err)
	}

	// Wrap in Module
	module, err := NewModule(cfg.Name, plugin, cfg, dispatcher, r.logger)
	if err != nil {
		_ = plugin.Close(ctx)
		return nil, fmt.Errorf("create module: %w", err)
	}

	r.mu.Lock()
	r.modules[cfg.Name] = module
	r.mu.Unlock()

	r.logger.Info("wasm module loaded", "name", cfg.Name, "path", cfg.ModulePath, "size_bytes", len(wasmBytes))

	return module, nil
}

// buildHostFunctions creates the noda_call and noda_call_async Extism host functions.
func buildHostFunctions(dispatcher *HostDispatcher, logger *slog.Logger) []extism.HostFunction {
	// noda_call: synchronous host call.
	// Input (PTR): JSON-encoded HostCallRequest {service, operation, payload}
	// Output (PTR): JSON-encoded response data (or empty on void operations)
	// On error: sets Extism error string.
	nodaCall := extism.NewHostFunctionWithStack(
		"noda_call",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			codec := dispatcher.module.Codec
			writeEnvelope := func(env map[string]any) {
				out, mErr := codec.Marshal(env)
				if mErr != nil {
					stack[0] = 0
					return
				}
				off, wErr := p.WriteBytes(out)
				if wErr != nil {
					stack[0] = 0
					return
				}
				stack[0] = off
			}
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				writeEnvelope(map[string]any{"ok": false, "error": map[string]any{"code": "INTERNAL_ERROR", "message": "read input: " + err.Error()}})
				return
			}
			var req HostCallRequest
			if err := codec.Unmarshal(input, &req); err != nil {
				writeEnvelope(map[string]any{"ok": false, "error": map[string]any{"code": "VALIDATION_ERROR", "message": "invalid request: " + err.Error()}})
				return
			}
			result, err := dispatcher.Call(ctx, req)
			if err != nil {
				writeEnvelope(map[string]any{"ok": false, "error": map[string]any{"code": classifyError(err), "message": err.Error()}})
				return
			}
			if result == nil {
				stack[0] = 0 // void success
				return
			}
			writeEnvelope(map[string]any{"ok": true, "data": result})
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)

	// noda_call_async: asynchronous host call.
	// Input (PTR): JSON-encoded HostCallRequest {service, operation, payload, label}
	// No output — result delivered in next tick's responses field.
	nodaCallAsync := extism.NewHostFunctionWithStack(
		"noda_call_async",
		func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				logger.Error("noda_call_async: read input failed", "error", err)
				return
			}

			var req HostCallRequest
			codec := dispatcher.module.Codec
			if err := codec.Unmarshal(input, &req); err != nil {
				logger.Error("noda_call_async: invalid request", "error", err)
				return
			}

			if err := dispatcher.CallAsync(ctx, req); err != nil {
				logger.Error("noda_call_async: dispatch failed", "error", err, "label", req.Label)
			}
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{},
	)

	return []extism.HostFunction{nodaCall, nodaCallAsync}
}

// LoadModuleWithPlugin loads a module using a pre-created PluginInstance (for testing).
func (r *Runtime) LoadModuleWithPlugin(cfg ModuleConfig, plugin PluginInstance) (*Module, error) {
	dispatcher := NewHostDispatcher(r.services, r.runner, r.logger)

	module, err := NewModule(cfg.Name, plugin, cfg, dispatcher, r.logger)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.modules[cfg.Name] = module
	r.mu.Unlock()

	return module, nil
}

// GetModule returns a module by name.
func (r *Runtime) GetModule(name string) (*Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.modules[name]
	return m, ok
}

// StartAll initializes and starts all loaded modules.
func (r *Runtime) StartAll(ctx context.Context) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for name, m := range r.modules {
		if err := m.Initialize(ctx); err != nil {
			return fmt.Errorf("initialize module %q: %w", name, err)
		}
		r.logger.Info("wasm module initialized", "name", name)
		m.Start()
		r.logger.Info("wasm module started", "name", name, "tick_rate", m.tickRate)
	}
	return nil
}

// StopAll stops all running modules and returns the first error encountered.
func (r *Runtime) StopAll(ctx context.Context) error {
	r.mu.Lock()
	modules := make(map[string]*Module, len(r.modules))
	for k, v := range r.modules {
		modules[k] = v
	}
	r.mu.Unlock()

	var firstErr error
	for name, m := range modules {
		if err := m.Stop(ctx); err != nil {
			r.logger.Error("stop module failed", "name", name, "error", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("stopping module %q: %w", name, err)
			}
		}
	}
	return firstErr
}

// WasmService provides the API for workflow nodes (wasm.send, wasm.query) to interact with modules.
type WasmService struct {
	runtime *Runtime
	module  string // module name
}

// NewWasmService creates a service wrapper for a specific module.
func NewWasmService(runtime *Runtime, moduleName string) *WasmService {
	return &WasmService{
		runtime: runtime,
		module:  moduleName,
	}
}

// Query calls the module's query export.
func (s *WasmService) Query(ctx context.Context, data any, timeout string) (any, error) {
	m, ok := s.runtime.GetModule(s.module)
	if !ok {
		return nil, fmt.Errorf("module %q not found", s.module)
	}

	dur, err := parseDuration(timeout)
	if err != nil {
		dur = 5 * time.Second
	}

	return m.Query(ctx, data, dur)
}

// SendCommand sends a command to the module.
func (s *WasmService) SendCommand(data any) {
	m, ok := s.runtime.GetModule(s.module)
	if !ok {
		return
	}
	m.SendCommand(data)
}

func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 5 * time.Second, nil
	}
	return time.ParseDuration(s)
}
