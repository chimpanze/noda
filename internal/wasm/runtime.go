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
)

// Runtime manages all Wasm module instances.
type Runtime struct {
	mu       sync.RWMutex
	modules  map[string]*Module
	services *registry.ServiceRegistry
	runner   WorkflowRunner
	logger   *slog.Logger
}

// NewRuntime creates a new Wasm runtime.
func NewRuntime(services *registry.ServiceRegistry, runner WorkflowRunner, logger *slog.Logger) *Runtime {
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

	// Create Extism plugin
	pluginCfg := extism.PluginConfig{
		EnableWasi: true,
	}
	plugin, err := extism.NewPlugin(ctx, manifest, pluginCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("create extism plugin: %w", err)
	}

	// Wrap in Module
	module, err := NewModule(cfg.Name, plugin, cfg, dispatcher, r.logger)
	if err != nil {
		plugin.Close(ctx)
		return nil, fmt.Errorf("create module: %w", err)
	}

	r.mu.Lock()
	r.modules[cfg.Name] = module
	r.mu.Unlock()

	return module, nil
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
