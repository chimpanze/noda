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

// createScheduler creates the scheduler runtime without starting it.
// The lifecycle manager handles starting. Returns nil, nil if no schedules are configured.
func createScheduler(rtCtx *runtimeContext) (*scheduler.Runtime, error) {
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
	slog.Info("scheduler configured", "jobs", len(scheduleConfigs))
	return rt, nil
}

// createWasm creates the Wasm runtime and loads all configured modules without starting them.
// The lifecycle manager handles starting. Returns nil, nil if no wasm_runtimes are configured.
func createWasm(rtCtx *runtimeContext) (*wasm.Runtime, error) {
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

	slog.Info("wasm modules loaded", "modules", len(wasmRuntimes))
	return rt, nil
}

// createWorkers creates the worker runtime without starting it.
// The lifecycle manager handles starting. Returns nil, nil if no workers are configured.
func createWorkers(rtCtx *runtimeContext) (*worker.Runtime, error) {
	if len(rtCtx.RC.Workers) == 0 {
		return nil, nil
	}

	workerConfigs := worker.ParseWorkerConfigs(rtCtx.RC.Workers)
	mw := resolveWorkerMiddleware(workerConfigs, 5*time.Minute)
	var tracer oteltrace.Tracer
	if rtCtx.TraceProvider != nil {
		tracer = rtCtx.TraceProvider.Tracer()
	}

	rt := worker.NewRuntime(
		workerConfigs,
		rtCtx.Bootstrap.Services,
		rtCtx.Bootstrap.Nodes,
		rtCtx.RC.Workflows,
		rtCtx.WorkflowCache,
		mw,
		rtCtx.Bootstrap.Compiler,
		tracer,
		rtCtx.Logger,
		rtCtx.SecretsCtx,
	)
	slog.Info("workers configured", "consumers", len(workerConfigs))
	return rt, nil
}

// createServer creates the HTTP server, runs Setup, and registers OpenAPI routes.
// The server is returned ready but not started. Extra options (e.g. WithTraceHub)
// are appended after the base options.
func createServer(rtCtx *runtimeContext, metricsOpt server.ServerOption, extraOpts ...server.ServerOption) (*server.Server, error) {
	serverOpts := []server.ServerOption{
		server.WithLogger(rtCtx.Logger),
		server.WithWorkflowCache(rtCtx.WorkflowCache),
		server.WithCompiler(rtCtx.Bootstrap.Compiler),
		server.WithSecretsContext(rtCtx.SecretsCtx),
	}
	if metricsOpt != nil {
		serverOpts = append(serverOpts, metricsOpt)
	}
	serverOpts = append(serverOpts, extraOpts...)

	srv, err := server.NewServer(rtCtx.RC, rtCtx.Bootstrap.Services, rtCtx.Bootstrap.Nodes, serverOpts...)
	if err != nil {
		return nil, fmt.Errorf("creating server: %w", err)
	}
	if err := srv.Setup(); err != nil {
		return nil, fmt.Errorf("setting up server: %w", err)
	}
	if err := srv.RegisterOpenAPIRoutes(); err != nil {
		rtCtx.Logger.Warn("OpenAPI generation failed", "error", err.Error())
	}
	return srv, nil
}

// lifecycleComponents holds the optional runtimes to register with the lifecycle manager.
type lifecycleComponents struct {
	Server        *server.Server
	WorkerRuntime *worker.Runtime
	Scheduler     *scheduler.Runtime
	WasmRuntime   *wasm.Runtime
	// ExtraComponents are registered after wasm but before conn managers.
	ExtraComponents []lifecycle.Component
}

// setupLifecycle creates a lifecycle manager, installs the signal handler,
// registers all non-nil components in dependency order, and calls StartAll.
// Returns a done channel that is closed after graceful shutdown completes.
func setupLifecycle(rtCtx *runtimeContext, comps lifecycleComponents) (*lifecycle.Lifecycle, <-chan struct{}, error) {
	lc := lifecycle.New(rtCtx.Logger)

	deadline := parseShutdownDeadline(rtCtx.RC, 30*time.Second)
	lc.SetRollbackDeadline(deadline)

	doneCh := make(chan struct{})
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Info("shutting down")
		go func() {
			<-sigCh
			slog.Warn("forced shutdown — received second signal")
			os.Exit(1)
		}()
		lc.StopAll(deadline)
		close(doneCh)
	}()

	// Register components so that shutdown (reverse order) matches the spec:
	// 1. Stop HTTP server  2. Stop workers  3. Stop scheduler  4. Stop Wasm
	// 5. Stop file watcher  6. Close connections  7. Close services  8. Flush telemetry
	if rtCtx.TraceProvider != nil {
		lc.Register(lifecycle.TracerComponent(rtCtx.TraceProvider))
	}
	lc.Register(lifecycle.ServiceRegistryComponent(rtCtx.Bootstrap.Services))
	if comps.Server != nil {
		lc.Register(lifecycle.ConnManagerComponent(comps.Server.ConnManagers()))
	}
	for _, c := range comps.ExtraComponents {
		lc.Register(c)
	}
	if comps.WasmRuntime != nil {
		lc.Register(lifecycle.WasmComponent(comps.WasmRuntime))
	}
	if comps.Scheduler != nil {
		lc.Register(lifecycle.SchedulerComponent(comps.Scheduler))
	}
	if comps.WorkerRuntime != nil {
		lc.Register(lifecycle.WorkerComponent(comps.WorkerRuntime))
	}
	if comps.Server != nil {
		lc.Register(lifecycle.ServerComponent(comps.Server))
	}

	if err := lc.StartAll(context.Background()); err != nil {
		return nil, nil, fmt.Errorf("lifecycle start: %w", err)
	}

	// Verify all services are reachable before marking ready (spec step 12).
	if healthErrs := rtCtx.Bootstrap.Services.HealthCheckAll(); len(healthErrs) > 0 {
		var msgs []string
		for name, err := range healthErrs {
			msgs = append(msgs, fmt.Sprintf("%s: %s", name, err))
		}
		lc.StopAll(parseShutdownDeadline(rtCtx.RC, 30*time.Second))
		close(doneCh)
		return nil, nil, fmt.Errorf("startup health check failed:\n  %s", strings.Join(msgs, "\n  "))
	}

	return lc, doneCh, nil
}
