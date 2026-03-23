package main

import (
	"context"
	"fmt"

	json "github.com/goccy/go-json"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/devmode"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/lifecycle"
	nodamcp "github.com/chimpanze/noda/internal/mcp"
	"github.com/chimpanze/noda/internal/migrate"
	"github.com/chimpanze/noda/internal/pathutil"
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
	coreoidc "github.com/chimpanze/noda/plugins/core/oidc"
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
	livekitplugin "github.com/chimpanze/noda/plugins/livekit"
	pubsubplugin "github.com/chimpanze/noda/plugins/pubsub"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"
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

			// Create secrets manager
			sm, err := config.NewSecretsManager(configDir, envFlag)
			if err != nil {
				return fmt.Errorf("loading secrets: %w", err)
			}

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

			rc, errs := config.ValidateAll(configDir, envFlag, sm)
			if len(errs) > 0 {
				return fmt.Errorf("config validation failed:\n%s", config.FormatErrors(errs))
			}

			// Plugin/service/node startup validation (dry-run: no database connections)
			plugins := registry.NewPluginRegistry()
			if err := registerCorePlugins(plugins); err != nil {
				return err
			}
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

			// Create secrets manager
			sm, err := config.NewSecretsManager(configDir, envFlag)
			if err != nil {
				return fmt.Errorf("loading secrets: %w", err)
			}

			// Load and validate config
			rc, errs := config.ValidateAll(configDir, envFlag, sm)
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
			coreNodeReg, err := buildCoreNodeRegistry()
			if err != nil {
				return err
			}

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

			rtCtx, err := initRuntime(configDir, envFlag, initOptions{})
			if err != nil {
				return err
			}

			_, metricsOpt := initMetrics(rtCtx.RC, rtCtx.Logger)

			// Create and setup server (if requested)
			var srv *server.Server
			if runServer {
				serverOpts := []server.ServerOption{
					server.WithLogger(rtCtx.Logger),
					server.WithWorkflowCache(rtCtx.WorkflowCache),
					server.WithCompiler(rtCtx.Bootstrap.Compiler),
					server.WithSecretsContext(rtCtx.SecretsCtx),
				}
				if metricsOpt != nil {
					serverOpts = append(serverOpts, metricsOpt)
				}
				srv, err = server.NewServer(rtCtx.RC, rtCtx.Bootstrap.Services, rtCtx.Bootstrap.Nodes, serverOpts...)
				if err != nil {
					return fmt.Errorf("creating server: %w", err)
				}
				if err := srv.Setup(); err != nil {
					return fmt.Errorf("setting up server: %w", err)
				}
				if err := srv.RegisterOpenAPIRoutes(); err != nil {
					rtCtx.Logger.Warn("OpenAPI generation failed", "error", err.Error())
				}
			}

			// Start workers if configured and requested
			var workerRuntime *worker.Runtime
			if runWorkers && len(rtCtx.RC.Workers) > 0 {
				workerConfigs := worker.ParseWorkerConfigs(rtCtx.RC.Workers)
				mw := resolveWorkerMiddleware(workerConfigs, 5*time.Minute)
				workerRuntime = worker.NewRuntime(
					workerConfigs,
					rtCtx.Bootstrap.Services,
					rtCtx.Bootstrap.Nodes,
					rtCtx.RC.Workflows,
					rtCtx.WorkflowCache,
					mw,
					rtCtx.Bootstrap.Compiler,
					rtCtx.Logger,
					rtCtx.SecretsCtx,
				)
				if err := workerRuntime.Start(context.Background()); err != nil {
					return fmt.Errorf("starting workers: %w", err)
				}
				slog.Info("workers started", "consumers", len(workerConfigs))
			}

			// Start scheduler (if requested)
			var schedulerRT *scheduler.Runtime
			if runScheduler {
				schedulerRT, err = initScheduler(rtCtx)
				if err != nil {
					return err
				}
			}

			// Start Wasm runtimes (if requested)
			var wasmRT *wasm.Runtime
			if runWasm {
				wasmRT, err = initWasm(rtCtx)
				if err != nil {
					return err
				}
			}

			// Lifecycle
			if _, err := setupLifecycle(rtCtx, lifecycleComponents{
				Server:        srv,
				WorkerRuntime: workerRuntime,
				Scheduler:     schedulerRT,
				WasmRuntime:   wasmRT,
			}); err != nil {
				return err
			}

			// Mark ready
			if srv != nil {
				srv.SetReady()
			}
			slog.Info("noda ready")

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

			rtCtx, err := initRuntime(configDir, envFlag, initOptions{
				ForceTracing: true,
				TracingFatal: true,
			})
			if err != nil {
				return err
			}

			// Create trace event hub for dev mode streaming
			hub := trace.NewEventHub()

			_, metricsOpt := initMetrics(rtCtx.RC, rtCtx.Logger)

			// Create and setup server with dev options
			serverOpts := []server.ServerOption{
				server.WithLogger(rtCtx.Logger),
				server.WithWorkflowCache(rtCtx.WorkflowCache),
				server.WithCompiler(rtCtx.Bootstrap.Compiler),
				server.WithTraceHub(hub),
				server.WithSecretsContext(rtCtx.SecretsCtx),
			}
			if metricsOpt != nil {
				serverOpts = append(serverOpts, metricsOpt)
			}
			srv, err := server.NewServer(rtCtx.RC, rtCtx.Bootstrap.Services, rtCtx.Bootstrap.Nodes, serverOpts...)
			if err != nil {
				return fmt.Errorf("creating server: %w", err)
			}
			if err := srv.Setup(); err != nil {
				return fmt.Errorf("setting up server: %w", err)
			}
			if err := srv.RegisterOpenAPIRoutes(); err != nil {
				rtCtx.Logger.Warn("OpenAPI generation failed", "error", err.Error())
			}

			// Register trace WebSocket endpoint (dev only)
			trace.RegisterTraceWebSocket(srv.App(), hub, rtCtx.Logger)

			// Start scheduler and wasm (always if configured in dev mode)
			schedulerRT, err := initScheduler(rtCtx)
			if err != nil {
				return err
			}
			wasmRT, err := initWasm(rtCtx)
			if err != nil {
				return err
			}

			// Set up hot-reload
			reloader := devmode.NewReloader(configDir, envFlag, rtCtx.RC, hub, rtCtx.Logger)
			reloader.OnReload(func(newRC *config.ResolvedConfig) {
				if err := rtCtx.WorkflowCache.Invalidate(newRC.Workflows, rtCtx.Bootstrap.Nodes); err != nil {
					rtCtx.Logger.Error("workflow cache invalidation failed", "error", err)
				} else {
					rtCtx.Logger.Info("workflow cache invalidated", "workflows", len(newRC.Workflows))
				}
			})

			// Register editor API endpoints (dev mode only)
			root, err := pathutil.NewRoot(configDir)
			if err != nil {
				return fmt.Errorf("resolving config directory: %w", err)
			}
			editorAPI := server.NewEditorAPI(root, envFlag, reloader, rtCtx.Plugins, rtCtx.Bootstrap.Nodes, rtCtx.Bootstrap.Services, rtCtx.Bootstrap.Compiler, rtCtx.SecretsManager)
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
					if !strings.HasPrefix(absPath, editorDist+string(filepath.Separator)) {
						return c.Status(403).SendString("forbidden")
					}
					return c.SendFile(absPath)
				})
			} else {
				srv.RegisterEditorUI()
			}

			// Set up file watcher
			fileWatcher, err := devmode.NewWatcher(reloader.HandleChange, rtCtx.Logger)
			if err != nil {
				return fmt.Errorf("creating file watcher: %w", err)
			}
			if err := fileWatcher.WatchDir(configDir); err != nil {
				rtCtx.Logger.Warn("failed to watch config directory", "error", err.Error())
			}

			// Lifecycle
			slog.Info("watching for changes", "dir", configDir)
			if _, err := setupLifecycle(rtCtx, lifecycleComponents{
				Server:      srv,
				Scheduler:   schedulerRT,
				WasmRuntime: wasmRT,
				ExtraComponents: []lifecycle.Component{
					lifecycle.WatcherComponent(fileWatcher, reloader),
				},
			}); err != nil {
				return err
			}

			srv.SetReady()
			slog.Info("noda ready")
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

			// Create secrets manager
			sm, err := config.NewSecretsManager(configDir, envFlag)
			if err != nil {
				return fmt.Errorf("loading secrets: %w", err)
			}

			// Load and validate config
			rc, errs := config.ValidateAll(configDir, envFlag, sm)
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

	cmd.PersistentFlags().String("service", "db", "database service name from config")

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

	sm, err := config.NewSecretsManager(configDir, envFlag)
	if err != nil {
		return nil, "", nil, fmt.Errorf("loading secrets: %w", err)
	}

	rc, errs := config.ValidateAll(configDir, envFlag, sm)
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
		&coreoidc.Plugin{},
		&livekitplugin.Plugin{},
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

func buildCoreNodeRegistry() (*registry.NodeRegistry, error) {
	nodeReg := registry.NewNodeRegistry()
	for _, p := range corePlugins() {
		if err := nodeReg.RegisterFromPlugin(p); err != nil {
			return nil, fmt.Errorf("register nodes from %q: %w", p.Name(), err)
		}
	}
	return nodeReg, nil
}

func registerCorePlugins(plugins *registry.PluginRegistry) error {
	for _, p := range corePlugins() {
		if err := plugins.Register(p); err != nil {
			return fmt.Errorf("register plugin %q: %w", p.Name(), err)
		}
	}
	for _, p := range serviceOnlyPlugins() {
		if err := plugins.Register(p); err != nil {
			return fmt.Errorf("register plugin %q: %w", p.Name(), err)
		}
	}
	return nil
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

			sm, err := config.NewSecretsManager(configDir, envFlag)
			if err != nil {
				return fmt.Errorf("loading secrets: %w", err)
			}

			rc, errs := config.ValidateAll(configDir, envFlag, sm)
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

// parseShutdownDeadline reads the shutdown_deadline from server config, falling back to defaultVal.
func parseShutdownDeadline(rc *config.ResolvedConfig, defaultVal time.Duration) time.Duration {
	if serverCfg, ok := rc.Root["server"].(map[string]any); ok {
		if d, ok := serverCfg["shutdown_deadline"].(string); ok {
			if parsed, err := time.ParseDuration(d); err == nil {
				return parsed
			}
		}
	}
	return defaultVal
}

// buildWorkflowRunner creates a standalone WorkflowRunner for use outside
// the HTTP server (e.g., by the Wasm runtime).
func buildWorkflowRunner(
	cache *engine.WorkflowCache,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
	compiler *expr.Compiler,
	secretsCtx map[string]any,
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
			engine.WithSecrets(secretsCtx),
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
	if v, ok := m["tick_timeout"].(string); ok {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.TickTimeout = d
		}
	}
	if v, ok := m["allowed_workflows"].([]any); ok {
		for _, s := range v {
			if str, ok := s.(string); ok {
				cfg.AllowedWorkflows = append(cfg.AllowedWorkflows, str)
			}
		}
	}
	if outbound, ok := m["allow_outbound"].(map[string]any); ok {
		if v, ok := outbound["http"].([]any); ok {
			for _, s := range v {
				if str, ok := s.(string); ok {
					cfg.AllowHTTP = append(cfg.AllowHTTP, str)
				}
			}
		}
		if v, ok := outbound["ws"].([]any); ok {
			for _, s := range v {
				if str, ok := s.(string); ok {
					cfg.AllowWS = append(cfg.AllowWS, str)
				}
			}
		}
	}

	return cfg
}

// resolveWorkerMiddleware returns the middleware chain shared by all workers.
// It uses the first worker config that specifies custom middleware; if none do,
// it falls back to DefaultMiddleware. All workers share a single middleware chain —
// per-worker middleware is not currently supported.
func resolveWorkerMiddleware(configs []worker.WorkerConfig, timeout time.Duration) []worker.Middleware {
	for _, wc := range configs {
		if len(wc.Middleware) > 0 {
			return worker.ResolveMiddleware(wc.Middleware, timeout)
		}
	}
	return worker.DefaultMiddleware(timeout)
}
