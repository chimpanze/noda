package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/devmode"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	nodamcp "github.com/chimpanze/noda/internal/mcp"
	"github.com/chimpanze/noda/internal/migrate"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/scheduler"
	"github.com/chimpanze/noda/internal/server"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/chimpanze/noda/internal/trace"
	"github.com/chimpanze/noda/internal/wasm"
	"github.com/chimpanze/noda/internal/worker"
	"github.com/chimpanze/noda/pkg/api"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/event"
	"github.com/chimpanze/noda/plugins/core/response"
	coresse "github.com/chimpanze/noda/plugins/core/sse"
	corestorage "github.com/chimpanze/noda/plugins/core/storage"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/upload"
	"github.com/chimpanze/noda/plugins/core/util"
	corewasm "github.com/chimpanze/noda/plugins/core/wasm"
	"github.com/chimpanze/noda/plugins/core/workflow"
	corews "github.com/chimpanze/noda/plugins/core/ws"
	dbplugin "github.com/chimpanze/noda/plugins/db"
	emailplugin "github.com/chimpanze/noda/plugins/email"
	httpplugin "github.com/chimpanze/noda/plugins/http"
	imageplugin "github.com/chimpanze/noda/plugins/image"
	pubsubplugin "github.com/chimpanze/noda/plugins/pubsub"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
	oteltrace "go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// Build info set at build time via -ldflags.
var (
	Version   = "0.0.1-dev"
	Commit    = ""
	BuildTime = ""
)

func main() {
	// Configure log level from LOG_LEVEL env (debug, info, warn, error).
	// Defaults to info if unset.
	if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(lvl)); err == nil {
			slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
		}
	}

	rootCmd := &cobra.Command{
		Use:          "noda",
		Short:        "Noda — configuration-driven API runtime",
		Long:         "Noda is a configuration-driven API runtime for Go. JSON config files define routes, workflows, middleware, auth, services, and real-time connections.",
		Version:      Version,
		SilenceUsage: true,
	}

	rootCmd.PersistentFlags().String("env", "", "runtime environment")
	rootCmd.PersistentFlags().String("config", ".", "path to config directory")

	rootCmd.AddCommand(
		&cobra.Command{
			Use:   "version",
			Short: "Print Noda version and build info",
			Run: func(_ *cobra.Command, _ []string) {
				fmt.Printf("noda %s\n", Version)
				fmt.Printf("go    %s\n", runtime.Version())
				fmt.Printf("os    %s/%s\n", runtime.GOOS, runtime.GOARCH)
				if Commit != "" {
					fmt.Printf("commit %s\n", Commit)
				}
				if BuildTime != "" {
					fmt.Printf("built  %s\n", BuildTime)
				}
			},
		},
		newValidateCmd(),
		newTestCmd(),
		newStartCmd(),
		newGenerateCmd(),
		newMigrateCmd(),
		newScheduleCmd(),
		newDevCmd(),
		newInitCmd(),
		newPluginCmd(),
		newCompletionCmd(),
		newMCPCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration files",
		RunE: func(cmd *cobra.Command, _ []string) error {
			configDir, _ := cmd.Flags().GetString("config")
			envFlag, _ := cmd.Flags().GetString("env")
			verbose, _ := cmd.Flags().GetBool("verbose")

			// Load .env files before config validation
			loadDotEnv(configDir, envFlag, nil)

			if verbose {
				info, err := config.GetValidateInfo(configDir, envFlag)
				if err != nil {
					return fmt.Errorf("getting validation info: %w", err)
				}
				fmt.Printf("Environment: %s\n", info.Environment)
				if info.OverlayFile != "" {
					fmt.Printf("Overlay: %s\n", info.OverlayFile)
				}
				for category, count := range info.FileCounts {
					if count > 0 {
						fmt.Printf("  %s: %d file(s)\n", category, count)
					}
				}
				fmt.Println()
			}

			rc, errs := config.ValidateAll(configDir, envFlag)
			if len(errs) > 0 {
				return fmt.Errorf("config validation failed:\n%s", config.FormatErrors(errs))
			}

			// Plugin/service/node startup validation (dry-run: no database connections)
			plugins := registry.NewPluginRegistry()
			registerCorePlugins(plugins)
			_, bootstrapErrs := registry.Bootstrap(rc, plugins, registry.BootstrapOptions{DryRun: true})
			if len(bootstrapErrs) > 0 {
				var errMsgs []string
				for _, e := range bootstrapErrs {
					errMsgs = append(errMsgs, e.Error())
				}
				return fmt.Errorf("bootstrap failed:\n  %s", strings.Join(errMsgs, "\n  "))
			}

			fmt.Printf("✓ All config files valid (%d files checked)\n", rc.FileCount)
			return nil
		},
	}

	cmd.Flags().Bool("verbose", false, "show detailed validation info")

	return cmd
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Run workflow tests",
		RunE: func(cmd *cobra.Command, _ []string) error {
			configDir, _ := cmd.Flags().GetString("config")
			envFlag, _ := cmd.Flags().GetString("env")
			verbose, _ := cmd.Flags().GetBool("verbose")
			workflowFilter, _ := cmd.Flags().GetString("workflow")

			// Load .env files before config validation
			loadDotEnv(configDir, envFlag, nil)

			// Load and validate config
			rc, errs := config.ValidateAll(configDir, envFlag)
			if len(errs) > 0 {
				return fmt.Errorf("config validation failed:\n%s", config.FormatErrors(errs))
			}

			// Load test suites
			suites, err := nodatesting.LoadTests(rc)
			if err != nil {
				return fmt.Errorf("loading tests: %w", err)
			}

			if len(suites) == 0 {
				fmt.Println("No test files found in tests/")
				return nil
			}

			// Filter by workflow if specified
			if workflowFilter != "" {
				var filtered []nodatesting.TestSuite
				for _, s := range suites {
					if s.Workflow == workflowFilter {
						filtered = append(filtered, s)
					}
				}
				suites = filtered
				if len(suites) == 0 {
					fmt.Printf("No tests found for workflow %q\n", workflowFilter)
					return nil
				}
			}

			// Build core node registry
			coreNodeReg := buildCoreNodeRegistry()

			// Run all suites
			var suiteResults []nodatesting.SuiteResult
			anyFailed := false
			for _, suite := range suites {
				results := nodatesting.RunTestSuite(suite, rc, coreNodeReg)
				suiteResults = append(suiteResults, nodatesting.SuiteResult{
					Suite:   suite,
					Results: results,
				})
				for _, r := range results {
					if !r.Passed {
						anyFailed = true
					}
				}
			}

			// Print results
			fmt.Print(nodatesting.FormatResults(suiteResults, verbose))

			if anyFailed {
				return fmt.Errorf("some tests failed")
			}
			return nil
		},
	}

	cmd.Flags().Bool("verbose", false, "show execution traces for all test cases")
	cmd.Flags().String("workflow", "", "run tests only for specified workflow")

	return cmd
}

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the production server",
		Long:  "Start Noda in production mode. Use flags to select which runtimes to start.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			configDir, _ := cmd.Flags().GetString("config")
			envFlag, _ := cmd.Flags().GetString("env")
			runServer, _ := cmd.Flags().GetBool("server")
			runWorkers, _ := cmd.Flags().GetBool("workers")
			runScheduler, _ := cmd.Flags().GetBool("scheduler")
			runWasm, _ := cmd.Flags().GetBool("wasm")
			runAll, _ := cmd.Flags().GetBool("all")
			logger := slog.Default()

			// Default to --all if no specific flags are set
			if !runServer && !runWorkers && !runScheduler && !runWasm {
				runAll = true
			}
			if runAll {
				runServer = true
				runWorkers = true
				runScheduler = true
				runWasm = true
			}
			// Load .env files before config validation
			loadDotEnv(configDir, envFlag, nil)

			// Load and validate config
			rc, errs := config.ValidateAll(configDir, envFlag)
			if len(errs) > 0 {
				return fmt.Errorf("config validation failed:\n%s", config.FormatErrors(errs))
			}

			// Initialize OTel tracing
			traceCfg := trace.ParseConfig(rc.Root)
			traceProvider, err := trace.NewProvider(context.Background(), traceCfg, logger)
			if err != nil {
				logger.Warn("tracer initialization failed", "error", err.Error())
			}

			// Bootstrap plugins and services
			plugins := registry.NewPluginRegistry()
			registerCorePlugins(plugins)
			bootstrap, bootstrapErrs := registry.Bootstrap(rc, plugins)
			if len(bootstrapErrs) > 0 {
				var errMsgs []string
				for _, e := range bootstrapErrs {
					errMsgs = append(errMsgs, e.Error())
				}
				return fmt.Errorf("bootstrap failed:\n  %s", strings.Join(errMsgs, "\n  "))
			}

			// Pre-compile all workflows once for server, scheduler, and workers
			workflowCache, err := engine.NewWorkflowCache(rc.Workflows, bootstrap.Nodes)
			if err != nil {
				return fmt.Errorf("compiling workflows: %w", err)
			}

			var srv *server.Server
			if runServer {
				srv, err = server.NewServer(rc, bootstrap.Services, bootstrap.Nodes, server.WithLogger(logger), server.WithWorkflowCache(workflowCache), server.WithCompiler(bootstrap.Compiler))
				if err != nil {
					return fmt.Errorf("creating server: %w", err)
				}

				if err := srv.Setup(); err != nil {
					return fmt.Errorf("setting up server: %w", err)
				}

				if err := srv.RegisterOpenAPIRoutes(); err != nil {
					logger.Warn("OpenAPI generation failed", "error", err.Error())
				}

				// Serve embedded editor UI and read-only API (production mode)
				srv.RegisterEditorUI()
				editorAPI := server.NewEditorAPIReadOnly(configDir, envFlag, rc, plugins, bootstrap.Nodes, bootstrap.Services, bootstrap.Compiler)
				editorAPI.Register(srv.App())
			}

			// Start workers if configured and requested
			var workerRuntime *worker.Runtime
			if runWorkers && len(rc.Workers) > 0 {
				workerConfigs := worker.ParseWorkerConfigs(rc.Workers)
				mw := worker.DefaultMiddleware(5 * time.Minute)
				workerRuntime = worker.NewRuntime(
					workerConfigs,
					bootstrap.Services,
					bootstrap.Nodes,
					rc.Workflows,
					workflowCache,
					mw,
					bootstrap.Compiler,
					logger,
				)
				if err := workerRuntime.Start(context.Background()); err != nil {
					return fmt.Errorf("starting workers: %w", err)
				}
				slog.Info("workers started", "consumers", len(workerConfigs))
			}

			// Start scheduler if configured and requested
			var schedulerRuntime *scheduler.Runtime
			if runScheduler && len(rc.Schedules) > 0 {
				scheduleConfigs := scheduler.ParseScheduleConfigs(rc.Schedules)
				var tracer oteltrace.Tracer
				if traceProvider != nil {
					tracer = traceProvider.Tracer()
				}
				schedulerRuntime = scheduler.NewRuntime(
					scheduleConfigs,
					bootstrap.Services,
					bootstrap.Nodes,
					rc.Workflows,
					workflowCache,
					bootstrap.Compiler,
					tracer,
					logger,
				)
				if err := schedulerRuntime.Start(); err != nil {
					return fmt.Errorf("starting scheduler: %w", err)
				}
				slog.Info("scheduler started", "jobs", len(scheduleConfigs))
			}

			// Start Wasm runtimes if configured and requested
			var wasmRuntime *wasm.Runtime
			if runWasm {
				wasmRuntimes, _ := rc.Root["wasm_runtimes"].(map[string]any)
				if len(wasmRuntimes) > 0 {
					workflowRunner := buildWorkflowRunner(workflowCache, bootstrap.Services, bootstrap.Nodes, bootstrap.Compiler)
					wasmRuntime = wasm.NewRuntime(bootstrap.Services, workflowRunner, logger)
					for name, raw := range wasmRuntimes {
						cfg := parseWasmModuleConfig(name, raw)
						// Resolve module path relative to config directory
						if cfg.ModulePath != "" && !filepath.IsAbs(cfg.ModulePath) {
							cfg.ModulePath = filepath.Join(configDir, cfg.ModulePath)
						}
						if _, err := wasmRuntime.LoadModule(context.Background(), cfg); err != nil {
							return fmt.Errorf("loading wasm module %q: %w", name, err)
						}
						// Register WasmService so wasm.send/wasm.query nodes can reference this module
						wasmSvc := wasm.NewWasmService(wasmRuntime, name)
						_ = bootstrap.Services.Register(name, wasmSvc, nil)
					}
					if err := wasmRuntime.StartAll(context.Background()); err != nil {
						return fmt.Errorf("starting wasm runtimes: %w", err)
					}
					slog.Info("wasm runtimes started", "modules", len(wasmRuntimes))
				}
			}

			// Mark ready
			server.SetReady()

			// Handle graceful shutdown
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				slog.Info("shutting down")

				deadline := 30 * time.Second
				if serverCfg, ok := rc.Root["server"].(map[string]any); ok {
					if d, ok := serverCfg["shutdown_deadline"].(string); ok {
						if parsed, err := time.ParseDuration(d); err == nil {
							deadline = parsed
						}
					}
				}

				devmode.ShutdownSequence(logger, deadline, srv, schedulerRuntime, workerRuntime, wasmRuntime, nil, nil, bootstrap.Services, traceProvider)
				os.Exit(0)
			}()

			if srv != nil {
				slog.Info("server starting", "port", srv.Port())
				return srv.Start()
			}

			// No server — block on signal
			slog.Info("started without HTTP server")
			select {}
		},
	}

	cmd.Flags().Bool("server", false, "start HTTP server only")
	cmd.Flags().Bool("workers", false, "start worker runtime only")
	cmd.Flags().Bool("scheduler", false, "start scheduler only")
	cmd.Flags().Bool("wasm", false, "start Wasm runtimes only")
	cmd.Flags().Bool("all", false, "start all runtimes (default)")

	return cmd
}

func newDevCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dev",
		Short: "Start in development mode with hot reload",
		RunE: func(cmd *cobra.Command, _ []string) error {
			configDir, _ := cmd.Flags().GetString("config")
			envFlag, _ := cmd.Flags().GetString("env")
			logger := slog.Default()

			// Load .env files before config validation
			loadDotEnv(configDir, envFlag, logger)

			// Load and validate config
			rc, errs := config.ValidateAll(configDir, envFlag)
			if len(errs) > 0 {
				return fmt.Errorf("config validation failed:\n%s", config.FormatErrors(errs))
			}

			// Initialize OTel tracing
			traceCfg := trace.ParseConfig(rc.Root)
			if !traceCfg.Enabled {
				traceCfg.Enabled = true // always enabled in dev mode
			}
			traceProvider, err := trace.NewProvider(context.Background(), traceCfg, logger)
			if err != nil {
				return fmt.Errorf("initializing tracer: %w", err)
			}

			// Create trace event hub for dev mode streaming
			hub := trace.NewEventHub()

			// Bootstrap plugins and services
			plugins := registry.NewPluginRegistry()
			registerCorePlugins(plugins)
			bootstrap, bootstrapErrs := registry.Bootstrap(rc, plugins)
			if len(bootstrapErrs) > 0 {
				var errMsgs []string
				for _, e := range bootstrapErrs {
					errMsgs = append(errMsgs, e.Error())
				}
				return fmt.Errorf("bootstrap failed:\n  %s", strings.Join(errMsgs, "\n  "))
			}

			// Pre-compile all workflows
			workflowCache, err := engine.NewWorkflowCache(rc.Workflows, bootstrap.Nodes)
			if err != nil {
				return fmt.Errorf("compiling workflows: %w", err)
			}

			// Create and setup server
			srv, err := server.NewServer(rc, bootstrap.Services, bootstrap.Nodes, server.WithLogger(logger), server.WithWorkflowCache(workflowCache), server.WithCompiler(bootstrap.Compiler), server.WithTraceHub(hub))
			if err != nil {
				return fmt.Errorf("creating server: %w", err)
			}

			if err := srv.Setup(); err != nil {
				return fmt.Errorf("setting up server: %w", err)
			}

			// Register OpenAPI routes
			if err := srv.RegisterOpenAPIRoutes(); err != nil {
				logger.Warn("OpenAPI generation failed", "error", err.Error())
			}

			// Register trace WebSocket endpoint (dev only)
			trace.RegisterTraceWebSocket(srv.App(), hub, logger)

			// Start scheduler if configured
			var schedulerRuntime *scheduler.Runtime
			if len(rc.Schedules) > 0 {
				scheduleConfigs := scheduler.ParseScheduleConfigs(rc.Schedules)
				var tracer oteltrace.Tracer
				if traceProvider != nil {
					tracer = traceProvider.Tracer()
				}
				schedulerRuntime = scheduler.NewRuntime(
					scheduleConfigs,
					bootstrap.Services,
					bootstrap.Nodes,
					rc.Workflows,
					workflowCache,
					bootstrap.Compiler,
					tracer,
					logger,
				)
				if err := schedulerRuntime.Start(); err != nil {
					return fmt.Errorf("starting scheduler: %w", err)
				}
				slog.Info("scheduler started", "jobs", len(scheduleConfigs))
			}

			// Set up hot-reload
			reloader := devmode.NewReloader(configDir, envFlag, rc, hub, logger)
			reloader.OnReload(func(newRC *config.ResolvedConfig) {
				logger.Info("config reloaded — new workflows and routes will apply to new requests")
			})

			// Register editor API endpoints (dev mode only)
			editorAPI := server.NewEditorAPI(configDir, envFlag, reloader, plugins, bootstrap.Nodes, bootstrap.Services, bootstrap.Compiler)
			editorAPI.Register(srv.App())

			// Serve editor static files: prefer local dist (for live dev),
			// fall back to embedded assets (for Docker / production builds).
			editorDist := filepath.Join("editor", "dist")
			if info, err := os.Stat(editorDist); err == nil && info.IsDir() {
				srv.App().Get("/editor/*", func(c fiber.Ctx) error {
					file := c.Params("*")
					if file == "" {
						file = "index.html"
					}
					absPath := filepath.Join(editorDist, filepath.Clean(file))
					if !strings.HasPrefix(absPath, editorDist) {
						return c.Status(403).SendString("forbidden")
					}
					return c.SendFile(absPath)
				})
			} else {
				// Use embedded editor assets
				srv.RegisterEditorUI()
			}

			// Set up file watcher
			watcher, err := devmode.NewWatcher(reloader.HandleChange, logger)
			if err != nil {
				return fmt.Errorf("creating file watcher: %w", err)
			}
			if err := watcher.WatchDir(configDir); err != nil {
				logger.Warn("failed to watch config directory", "error", err.Error())
			}
			watcher.Start()
			slog.Info("watching for changes", "dir", configDir)

			// Mark server as ready
			server.SetReady()

			// Handle graceful shutdown
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				slog.Info("shutting down")

				deadline := 30 * time.Second
				if serverCfg, ok := rc.Root["server"].(map[string]any); ok {
					if d, ok := serverCfg["shutdown_deadline"].(string); ok {
						if parsed, err := time.ParseDuration(d); err == nil {
							deadline = parsed
						}
					}
				}

				devmode.ShutdownSequence(logger, deadline, srv, schedulerRuntime, nil, nil, watcher, nil, bootstrap.Services, traceProvider)
				os.Exit(0)
			}()

			slog.Info("dev server starting", "port", srv.Port())
			slog.Info("trace websocket available", "path", "/ws/trace")
			slog.Info("editor available", "path", "/editor/")
			return srv.Start()
		},
	}

	return cmd
}

func newGenerateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate OpenAPI specs or client SDKs",
	}

	openAPICmd := &cobra.Command{
		Use:   "openapi",
		Short: "Generate OpenAPI 3.1 specification",
		RunE: func(cmd *cobra.Command, _ []string) error {
			configDir, _ := cmd.Flags().GetString("config")
			envFlag, _ := cmd.Flags().GetString("env")
			output, _ := cmd.Flags().GetString("output")

			// Load and validate config
			rc, errs := config.ValidateAll(configDir, envFlag)
			if len(errs) > 0 {
				return fmt.Errorf("config validation failed:\n%s", config.FormatErrors(errs))
			}

			doc, err := server.GenerateOpenAPI(rc)
			if err != nil {
				return fmt.Errorf("generating OpenAPI spec: %w", err)
			}

			specBytes, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling spec: %w", err)
			}

			if output != "" {
				if err := os.WriteFile(output, specBytes, 0644); err != nil {
					return fmt.Errorf("writing file: %w", err)
				}
				fmt.Printf("OpenAPI spec written to %s\n", output)
			} else {
				fmt.Println(string(specBytes))
			}

			return nil
		},
	}

	openAPICmd.Flags().String("output", "", "output file path (default: stdout)")

	cmd.AddCommand(openAPICmd)

	return cmd
}

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start MCP server over stdio for AI agent integration",
		Long:  "Start a Model Context Protocol server over stdin/stdout. AI agents (Claude Code, Cursor, etc.) use this to discover node types, get schemas, scaffold projects, and validate configs.",
		RunE: func(_ *cobra.Command, _ []string) error {
			s := nodamcp.NewServer(Version)
			return mcpserver.ServeStdio(s)
		},
	}
}

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
	}

	cmd.PersistentFlags().String("service", "main-db", "database service name from config")

	createCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new migration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir, _ := cmd.Flags().GetString("config")
			migrationsDir := configDir + "/migrations"

			upFile, downFile, err := migrate.Create(migrationsDir, args[0])
			if err != nil {
				return fmt.Errorf("create migration: %w", err)
			}
			fmt.Printf("Created:\n  %s\n  %s\n", upFile, downFile)
			return nil
		},
	}

	upCmd := &cobra.Command{
		Use:   "up",
		Short: "Apply all pending migrations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, configDir, cleanup, err := getDBFromConfig(cmd)
			if err != nil {
				return fmt.Errorf("migrate up: %w", err)
			}
			defer cleanup()

			ran, err := migrate.Up(db, configDir+"/migrations")
			if err != nil {
				return fmt.Errorf("migrate up: %w", err)
			}

			if len(ran) == 0 {
				fmt.Println("No pending migrations")
			} else {
				for _, m := range ran {
					fmt.Printf("  Applied: %s\n", m)
				}
				fmt.Printf("%d migration(s) applied\n", len(ran))
			}
			return nil
		},
	}

	downCmd := &cobra.Command{
		Use:   "down",
		Short: "Roll back the last migration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, configDir, cleanup, err := getDBFromConfig(cmd)
			if err != nil {
				return fmt.Errorf("migrate down: %w", err)
			}
			defer cleanup()

			rolled, err := migrate.Down(db, configDir+"/migrations")
			if err != nil {
				return fmt.Errorf("migrate down: %w", err)
			}
			fmt.Printf("  Rolled back: %s\n", rolled)
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, configDir, cleanup, err := getDBFromConfig(cmd)
			if err != nil {
				return fmt.Errorf("migrate status: %w", err)
			}
			defer cleanup()

			statuses, err := migrate.Status(db, configDir+"/migrations")
			if err != nil {
				return fmt.Errorf("migrate status: %w", err)
			}

			if len(statuses) == 0 {
				fmt.Println("No migrations found")
				return nil
			}

			for _, s := range statuses {
				status := "pending"
				if s.Applied {
					status = "applied"
				}
				fmt.Printf("  [%s] %s_%s\n", status, s.Version, s.Name)
			}
			return nil
		},
	}

	cmd.AddCommand(createCmd, upCmd, downCmd, statusCmd)
	return cmd
}

func getDBFromConfig(cmd *cobra.Command) (*gorm.DB, string, func(), error) {
	configDir, _ := cmd.Flags().GetString("config")
	envFlag, _ := cmd.Flags().GetString("env")
	serviceName, _ := cmd.Flags().GetString("service")

	loadDotEnv(configDir, envFlag, nil)

	rc, errs := config.ValidateAll(configDir, envFlag)
	if len(errs) > 0 {
		return nil, "", nil, fmt.Errorf("config validation failed:\n%s", config.FormatErrors(errs))
	}

	// Create the database service from config
	servicesConfig, ok := rc.Root["services"].(map[string]any)
	if !ok {
		return nil, "", nil, fmt.Errorf("no services configured")
	}

	svcConfig, ok := servicesConfig[serviceName].(map[string]any)
	if !ok {
		return nil, "", nil, fmt.Errorf("service %q not found in config", serviceName)
	}

	innerConfig, _ := svcConfig["config"].(map[string]any)
	if innerConfig == nil {
		innerConfig = svcConfig
	}

	plugin := &dbplugin.Plugin{}
	svc, err := plugin.CreateService(innerConfig)
	if err != nil {
		return nil, "", nil, fmt.Errorf("create database service: %w", err)
	}

	db, ok := svc.(*gorm.DB)
	if !ok {
		return nil, "", nil, fmt.Errorf("service %q is not a database", serviceName)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, "", nil, fmt.Errorf("get underlying sql.DB: %w", err)
	}
	cleanup := func() { _ = sqlDB.Close() }

	return db, configDir, cleanup, nil
}

// corePlugins returns all built-in plugins. Used by both buildCoreNodeRegistry
// (for the test runner, which only needs nodes) and registerCorePlugins
// (for the full runtime, which also needs service-only plugins).
func corePlugins() []api.Plugin {
	return []api.Plugin{
		&control.Plugin{},
		&transform.Plugin{},
		&util.Plugin{},
		&workflow.Plugin{},
		&response.Plugin{},
		&dbplugin.Plugin{},
		&cacheplugin.Plugin{},
		&event.Plugin{},
		&corestorage.Plugin{},
		&upload.Plugin{},
		&imageplugin.Plugin{},
		&httpplugin.Plugin{},
		&emailplugin.Plugin{},
		&corews.Plugin{},
		&coresse.Plugin{},
		&corewasm.Plugin{},
	}
}

// serviceOnlyPlugins returns plugins that provide services but no nodes
// used by workflows (stream, pubsub, storage). These are registered in the
// full runtime but not needed for the test runner's node registry.
func serviceOnlyPlugins() []api.Plugin {
	return []api.Plugin{
		&streamplugin.Plugin{},
		&pubsubplugin.Plugin{},
		&storageplugin.Plugin{},
	}
}

func buildCoreNodeRegistry() *registry.NodeRegistry {
	nodeReg := registry.NewNodeRegistry()
	for _, p := range corePlugins() {
		_ = nodeReg.RegisterFromPlugin(p)
	}
	return nodeReg
}

func registerCorePlugins(plugins *registry.PluginRegistry) {
	for _, p := range corePlugins() {
		_ = plugins.Register(p)
	}
	for _, p := range serviceOnlyPlugins() {
		_ = plugins.Register(p)
	}
}

func newScheduleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schedule",
		Short: "Manage scheduled jobs",
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show schedule status (list, last run, next run)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			configDir, _ := cmd.Flags().GetString("config")
			envFlag, _ := cmd.Flags().GetString("env")

			rc, errs := config.ValidateAll(configDir, envFlag)
			if len(errs) > 0 {
				return fmt.Errorf("config validation failed:\n%s", config.FormatErrors(errs))
			}

			if len(rc.Schedules) == 0 {
				fmt.Println("No schedules configured.")
				return nil
			}

			scheduleConfigs := scheduler.ParseScheduleConfigs(rc.Schedules)
			fmt.Printf("%-20s  %-25s  %-10s  %s\n", "ID", "CRON", "TIMEZONE", "WORKFLOW")
			fmt.Println("-------------------------------------------------------------------------------------")
			for _, sc := range scheduleConfigs {
				tz := sc.Timezone
				if tz == "" {
					tz = "UTC"
				}
				fmt.Printf("%-20s  %-25s  %-10s  %s\n", sc.ID, sc.Cron, tz, sc.WorkflowID)
			}
			return nil
		},
	}

	cmd.AddCommand(statusCmd)
	return cmd
}

// loadDotEnv loads .env files from the config directory and working directory.
// Logs which files were loaded for transparency.
func loadDotEnv(configDir, envFlag string, logger *slog.Logger) {
	loaded := config.LoadDotEnv(configDir, envFlag)
	for _, f := range loaded {
		if logger != nil {
			logger.Info("loaded environment file", "path", f)
		} else {
			fmt.Printf("Loaded environment from %s\n", f)
		}
	}
}

// buildWorkflowRunner creates a standalone WorkflowRunner for use outside
// the HTTP server (e.g., by the Wasm runtime).
func buildWorkflowRunner(
	cache *engine.WorkflowCache,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
	compiler *expr.Compiler,
) api.WorkflowRunner {
	return func(ctx context.Context, workflowID string, input map[string]any) error {
		graph, ok := cache.Get(workflowID)
		if !ok {
			return fmt.Errorf("workflow %q not found", workflowID)
		}
		execCtx := engine.NewExecutionContext(
			engine.WithInput(input),
			engine.WithTrigger(api.TriggerData{
				Type:    "wasm",
				TraceID: uuid.New().String(),
			}),
			engine.WithWorkflowID(workflowID),
			engine.WithCompiler(compiler),
		)
		return engine.ExecuteGraph(ctx, graph, execCtx, services, nodes)
	}
}

// parseWasmModuleConfig converts a raw config map into a wasm.ModuleConfig.
func parseWasmModuleConfig(name string, raw any) wasm.ModuleConfig {
	cfg := wasm.ModuleConfig{Name: name}
	m, ok := raw.(map[string]any)
	if !ok {
		return cfg
	}

	if v, ok := m["module"].(string); ok {
		cfg.ModulePath = v
	}
	if v, ok := m["tick_rate"].(float64); ok {
		cfg.TickRate = int(v)
	}
	if v, ok := m["encoding"].(string); ok {
		cfg.Encoding = v
	}
	if v, ok := m["config"].(map[string]any); ok {
		cfg.Config = v
	}
	if v, ok := m["memory_pages"].(float64); ok {
		cfg.MemoryPages = uint32(v)
	}
	if v, ok := m["services"].([]any); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				cfg.Services = append(cfg.Services, str)
			}
		}
	}
	if v, ok := m["connections"].([]any); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				cfg.Connections = append(cfg.Connections, str)
			}
		}
	}
	if v, ok := m["allow_http"].([]any); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				cfg.AllowHTTP = append(cfg.AllowHTTP, str)
			}
		}
	}
	if v, ok := m["allow_ws"].([]any); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				cfg.AllowWS = append(cfg.AllowWS, str)
			}
		}
	}

	return cfg
}
