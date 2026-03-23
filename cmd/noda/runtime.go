package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/lifecycle"
	nodametrics "github.com/chimpanze/noda/internal/metrics"
	"github.com/chimpanze/noda/internal/pathutil"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/scheduler"
	"github.com/chimpanze/noda/internal/secrets"
	"github.com/chimpanze/noda/internal/server"
	"github.com/chimpanze/noda/internal/trace"
	"github.com/chimpanze/noda/internal/wasm"
	"github.com/chimpanze/noda/internal/worker"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// runtimeContext holds the shared initialized state for both start and dev commands.
type runtimeContext struct {
	RC             *config.ResolvedConfig
	SecretsCtx     map[string]any
	SecretsManager *secrets.Manager
	Bootstrap      *registry.BootstrapResult
	WorkflowCache  *engine.WorkflowCache
	TraceProvider  *trace.Provider
	Plugins        *registry.PluginRegistry
	Logger         *slog.Logger
	ConfigDir      string
}

// initOptions controls behavioral differences between start and dev mode during initialization.
type initOptions struct {
	// ForceTracing enables tracing even when config does not. Dev sets this to true.
	ForceTracing bool
	// TracingFatal makes tracing initialization failure fatal. Dev sets true, start sets false (warn only).
	TracingFatal bool
}

// initRuntime performs the shared initialization sequence: secrets, config validation,
// tracing, plugin bootstrap, workflow cache compilation, and secrets context.
func initRuntime(configDir, envFlag string, opts initOptions) (*runtimeContext, error) {
	logger := slog.Default()

	// Create secrets manager
	sm, err := config.NewSecretsManager(configDir, envFlag)
	if err != nil {
		return nil, fmt.Errorf("loading secrets: %w", err)
	}

	// Load and validate config
	rc, errs := config.ValidateAll(configDir, envFlag, sm)
	if len(errs) > 0 {
		return nil, fmt.Errorf("config validation failed:\n%s", config.FormatErrors(errs))
	}

	// Initialize OTel tracing
	traceCfg := trace.ParseConfig(rc.Root)
	if opts.ForceTracing && !traceCfg.Enabled {
		traceCfg.Enabled = true
	}
	traceProvider, err := trace.NewProvider(context.Background(), traceCfg, logger)
	if err != nil {
		if opts.TracingFatal {
			return nil, fmt.Errorf("initializing tracer: %w", err)
		}
		logger.Warn("tracer initialization failed", "error", err.Error())
	}

	// Bootstrap plugins and services
	plugins := registry.NewPluginRegistry()
	if err := registerCorePlugins(plugins); err != nil {
		return nil, err
	}
	bootstrap, bootstrapErrs := registry.Bootstrap(rc, plugins)
	if len(bootstrapErrs) > 0 {
		var errMsgs []string
		for _, e := range bootstrapErrs {
			errMsgs = append(errMsgs, e.Error())
		}
		return nil, fmt.Errorf("bootstrap failed:\n  %s", strings.Join(errMsgs, "\n  "))
	}

	// Pre-compile all workflows
	workflowCache, err := engine.NewWorkflowCache(rc.Workflows, bootstrap.Nodes)
	if err != nil {
		return nil, fmt.Errorf("compiling workflows: %w", err)
	}

	secretsCtx := sm.ExpressionContext()

	return &runtimeContext{
		RC:             rc,
		SecretsCtx:     secretsCtx,
		SecretsManager: sm,
		Bootstrap:      bootstrap,
		WorkflowCache:  workflowCache,
		TraceProvider:  traceProvider,
		Plugins:        plugins,
		Logger:         logger,
		ConfigDir:      configDir,
	}, nil
}

// initMetrics initializes the metrics provider and returns a server option.
// Returns nil values if metrics are disabled or initialization fails.
func initMetrics(rc *config.ResolvedConfig, logger *slog.Logger) (*nodametrics.Metrics, server.ServerOption) {
	metricsCfg := nodametrics.ParseConfig(rc.Root)
	if !metricsCfg.Enabled {
		return nil, nil
	}

	provider, handler, err := nodametrics.NewProvider()
	if err != nil {
		logger.Warn("metrics initialization failed", "error", err.Error())
		return nil, nil
	}

	meter := provider.Meter("noda")
	m, err := nodametrics.NewMetrics(meter)
	if err != nil {
		logger.Warn("metrics instrument creation failed", "error", err.Error())
		return nil, nil
	}

	return m, server.WithMetrics(m, handler, metricsCfg.Path)
}

// initScheduler creates and starts the scheduler runtime.
// Returns nil, nil if no schedules are configured.
func initScheduler(rtCtx *runtimeContext) (*scheduler.Runtime, error) {
	if len(rtCtx.RC.Schedules) == 0 {
		return nil, nil
	}

	scheduleConfigs := scheduler.ParseScheduleConfigs(rtCtx.RC.Schedules)
	var tracer oteltrace.Tracer
	if rtCtx.TraceProvider != nil {
		tracer = rtCtx.TraceProvider.Tracer()
	}

	rt := scheduler.NewRuntime(
		scheduleConfigs,
		rtCtx.Bootstrap.Services,
		rtCtx.Bootstrap.Nodes,
		rtCtx.RC.Workflows,
		rtCtx.WorkflowCache,
		rtCtx.Bootstrap.Compiler,
		tracer,
		rtCtx.Logger,
		rtCtx.SecretsCtx,
	)
	if err := rt.Start(); err != nil {
		return nil, fmt.Errorf("starting scheduler: %w", err)
	}
	slog.Info("scheduler started", "jobs", len(scheduleConfigs))
	return rt, nil
}

// initWasm creates and starts the Wasm runtime with all configured modules.
// Returns nil, nil if no wasm_runtimes are configured.
func initWasm(rtCtx *runtimeContext) (*wasm.Runtime, error) {
	wasmRuntimes, _ := rtCtx.RC.Root["wasm_runtimes"].(map[string]any)
	if len(wasmRuntimes) == 0 {
		return nil, nil
	}

	workflowRunner := buildWorkflowRunner(
		rtCtx.WorkflowCache, rtCtx.Bootstrap.Services, rtCtx.Bootstrap.Nodes,
		rtCtx.Bootstrap.Compiler, rtCtx.SecretsCtx,
	)
	rt := wasm.NewRuntime(rtCtx.Bootstrap.Services, workflowRunner, rtCtx.Logger)

	wasmRoot, err := pathutil.NewRoot(rtCtx.ConfigDir)
	if err != nil {
		return nil, fmt.Errorf("resolving config directory for wasm: %w", err)
	}

	for name, raw := range wasmRuntimes {
		cfg := parseWasmModuleConfig(name, raw)
		if cfg.ModulePath != "" && !filepath.IsAbs(cfg.ModulePath) {
			resolved, err := wasmRoot.Resolve(cfg.ModulePath)
			if err != nil {
				return nil, fmt.Errorf("wasm module %q: path outside config directory: %w", name, err)
			}
			cfg.ModulePath = resolved
		}
		if _, err := rt.LoadModule(context.Background(), cfg); err != nil {
			return nil, fmt.Errorf("loading wasm module %q: %w", name, err)
		}
		wasmSvc := wasm.NewWasmService(rt, name)
		if err := rtCtx.Bootstrap.Services.Register(name, wasmSvc, nil); err != nil {
			rtCtx.Logger.Warn("wasm service registration failed", "name", name, "error", err)
		}
	}

	if err := rt.StartAll(context.Background()); err != nil {
		return nil, fmt.Errorf("starting wasm runtimes: %w", err)
	}
	slog.Info("wasm runtimes started", "modules", len(wasmRuntimes))
	return rt, nil
}

// lifecycleComponents holds the optional runtimes to register with the lifecycle manager.
type lifecycleComponents struct {
	Server        *server.Server
	WorkerRuntime *worker.Runtime // nil in dev mode
	Scheduler     *scheduler.Runtime
	WasmRuntime   *wasm.Runtime
	// ExtraComponents are registered after wasm but before conn managers.
	ExtraComponents []lifecycle.Component
}

// setupLifecycle creates a lifecycle manager, installs the signal handler,
// registers all non-nil components in dependency order, and calls StartAll.
func setupLifecycle(rtCtx *runtimeContext, comps lifecycleComponents) (*lifecycle.Lifecycle, error) {
	lc := lifecycle.New(rtCtx.Logger)

	deadline := parseShutdownDeadline(rtCtx.RC, 30*time.Second)
	lc.SetRollbackDeadline(deadline)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("shutting down")
		lc.StopAll(deadline)
		os.Exit(0)
	}()

	// Register components in dependency order (shutdown is reverse).
	if comps.Server != nil {
		lc.Register(lifecycle.ServerComponent(comps.Server))
	}
	if comps.WorkerRuntime != nil {
		lc.Register(lifecycle.WorkerComponent(comps.WorkerRuntime))
	}
	if comps.Scheduler != nil {
		lc.Register(lifecycle.SchedulerComponent(comps.Scheduler))
	}
	if comps.WasmRuntime != nil {
		lc.Register(lifecycle.WasmComponent(comps.WasmRuntime))
	}
	for _, c := range comps.ExtraComponents {
		lc.Register(c)
	}
	if comps.Server != nil {
		lc.Register(lifecycle.ConnManagerComponent(comps.Server.ConnManagers()))
	}
	lc.Register(lifecycle.ServiceRegistryComponent(rtCtx.Bootstrap.Services))
	if rtCtx.TraceProvider != nil {
		lc.Register(lifecycle.TracerComponent(rtCtx.TraceProvider))
	}

	if err := lc.StartAll(context.Background()); err != nil {
		return nil, fmt.Errorf("lifecycle start: %w", err)
	}

	return lc, nil
}
