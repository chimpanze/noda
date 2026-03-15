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

// LoadModule loads a Wasm module from config and registers it.
func (r *Runtime) LoadModule(ctx context.Context, cfg ModuleConfig) (*Module, error) {
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
	manifest := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{Data: wasmBytes},
		},
		AllowedHosts: cfg.AllowHTTP,
	}

	if cfg.MemoryPages > 0 {
		manifest.Memory = &extism.ManifestMemory{
			MaxPages: cfg.MemoryPages,
		}
	}

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
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				logger.Error("noda_call: read input failed", "error", err)
				stack[0] = 0
				return
			}

			var req HostCallRequest
			codec := &jsonCodec{}
			if err := codec.Unmarshal(input, &req); err != nil {
				offset, _ := p.WriteString(fmt.Sprintf(`{"code":"VALIDATION_ERROR","message":"invalid request: %s"}`, err.Error()))
				stack[0] = offset
				return
			}

			result, err := dispatcher.Call(ctx, req)
			if err != nil {
				// Set error via Extism's error mechanism — the PDK reads this via pdk.GetError()
				errMsg, _ := codec.Marshal(map[string]any{
					"code":    "INTERNAL_ERROR",
					"message": err.Error(),
				})
				offset, _ := p.WriteBytes(errMsg) // write error as output so PDK can read it
				stack[0] = offset
				return
			}

			if result == nil {
				stack[0] = 0
				return
			}

			out, err := codec.Marshal(result)
			if err != nil {
				logger.Error("noda_call: marshal response failed", "error", err)
				stack[0] = 0
				return
			}

			offset, err := p.WriteBytes(out)
			if err != nil {
				logger.Error("noda_call: write response failed", "error", err)
				stack[0] = 0
				return
			}
			stack[0] = offset
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
			codec := &jsonCodec{}
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

// StopAll stops all running modules.
func (r *Runtime) StopAll(ctx context.Context) {
	r.mu.Lock()
	modules := make(map[string]*Module, len(r.modules))
	for k, v := range r.modules {
		modules[k] = v
	}
	r.mu.Unlock()

	for name, m := range modules {
		if err := m.Stop(ctx); err != nil {
			r.logger.Error("stop module failed", "name", name, "error", err)
		}
	}
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
