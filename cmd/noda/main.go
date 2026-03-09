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
	"syscall"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/devmode"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/migrate"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/scheduler"
	"github.com/chimpanze/noda/internal/server"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/chimpanze/noda/internal/trace"
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
	"github.com/spf13/cobra"
	oteltrace "go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// Version is set at build time via -ldflags.
var Version = "0.0.1-dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     "noda",
		Short:   "Noda — configuration-driven API runtime",
		Long:    "Noda is a configuration-driven API runtime for Go. JSON config files define routes, workflows, middleware, auth, services, and real-time connections.",
		Version: Version,
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

			if verbose {
				info, err := config.GetValidateInfo(configDir, envFlag)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %s\n", err)
					os.Exit(1)
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
				fmt.Fprint(os.Stderr, config.FormatErrors(errs))
				os.Exit(1)
			}

			// Plugin/service/node startup validation (dry-run: no database connections)
			plugins := registry.NewPluginRegistry()
			registerCorePlugins(plugins)
			_, bootstrapErrs := registry.Bootstrap(rc, plugins, registry.BootstrapOptions{DryRun: true})
			if len(bootstrapErrs) > 0 {
				for _, e := range bootstrapErrs {
					fmt.Fprintf(os.Stderr, "  ✗ %s\n", e)
				}
				os.Exit(1)
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

			// Load and validate config
			rc, errs := config.ValidateAll(configDir, envFlag)
			if len(errs) > 0 {
				fmt.Fprint(os.Stderr, config.FormatErrors(errs))
				os.Exit(1)
			}

			// Load test suites
			suites, err := nodatesting.LoadTests(rc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading tests: %s\n", err)
				os.Exit(1)
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
				os.Exit(1)
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
			runScheduler, _ := cmd.Flags().GetBool("scheduler")
			runAll, _ := cmd.Flags().GetBool("all")
			logger := slog.Default()

			// Default to --all if no specific flags are set
			if !runServer && !runScheduler {
				runAll = true
			}
			if runAll {
				runServer = true
				runScheduler = true
			}

			// Load and validate config
			rc, errs := config.ValidateAll(configDir, envFlag)
			if len(errs) > 0 {
				fmt.Fprint(os.Stderr, config.FormatErrors(errs))
				os.Exit(1)
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
				for _, e := range bootstrapErrs {
					fmt.Fprintf(os.Stderr, "  ✗ %s\n", e)
				}
				os.Exit(1)
			}

			// Pre-compile all workflows once for server, scheduler, and workers
			workflowCache, err := engine.NewWorkflowCache(rc.Workflows, bootstrap.Nodes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error compiling workflows: %s\n", err)
				os.Exit(1)
			}

			var srv *server.Server
			if runServer {
				srv, err = server.NewServer(rc, bootstrap.Services, bootstrap.Nodes, server.WithLogger(logger), server.WithWorkflowCache(workflowCache), server.WithCompiler(bootstrap.Compiler))
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error creating server: %s\n", err)
					os.Exit(1)
				}

				if err := srv.Setup(); err != nil {
					fmt.Fprintf(os.Stderr, "Error setting up server: %s\n", err)
					os.Exit(1)
				}

				if err := srv.RegisterOpenAPIRoutes(); err != nil {
					logger.Warn("OpenAPI generation failed", "error", err.Error())
				}

				// Serve embedded editor UI (production mode)
				srv.RegisterEditorUI()
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
					fmt.Fprintf(os.Stderr, "Error starting scheduler: %s\n", err)
					os.Exit(1)
				}
				fmt.Printf("Scheduler started with %d job(s)\n", len(scheduleConfigs))
			}

			// Mark ready
			server.SetReady()

			// Handle graceful shutdown
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				fmt.Println("\nShutting down...")

				deadline := 30 * time.Second
				if serverCfg, ok := rc.Root["server"].(map[string]any); ok {
					if d, ok := serverCfg["shutdown_deadline"].(string); ok {
						if parsed, err := time.ParseDuration(d); err == nil {
							deadline = parsed
						}
					}
				}

				devmode.ShutdownSequence(logger, deadline, srv, schedulerRuntime, nil, nil, traceProvider)
				os.Exit(0)
			}()

			if srv != nil {
				fmt.Printf("Noda server starting on port %d\n", srv.Port())
				return srv.Start()
			}

			// No server — block on signal
			fmt.Println("Noda started (no HTTP server)")
			select {}
		},
	}

	cmd.Flags().Bool("server", false, "start HTTP server only")
	cmd.Flags().Bool("scheduler", false, "start scheduler only")
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

			// Load and validate config
			rc, errs := config.ValidateAll(configDir, envFlag)
			if len(errs) > 0 {
				fmt.Fprint(os.Stderr, config.FormatErrors(errs))
				os.Exit(1)
			}

			// Initialize OTel tracing
			traceCfg := trace.ParseConfig(rc.Root)
			if !traceCfg.Enabled {
				traceCfg.Enabled = true // always enabled in dev mode
			}
			traceProvider, err := trace.NewProvider(context.Background(), traceCfg, logger)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error initializing tracer: %s\n", err)
				os.Exit(1)
			}

			// Create trace event hub for dev mode streaming
			hub := trace.NewEventHub()

			// Bootstrap plugins and services
			plugins := registry.NewPluginRegistry()
			registerCorePlugins(plugins)
			bootstrap, bootstrapErrs := registry.Bootstrap(rc, plugins)
			if len(bootstrapErrs) > 0 {
				for _, e := range bootstrapErrs {
					fmt.Fprintf(os.Stderr, "  ✗ %s\n", e)
				}
				os.Exit(1)
			}

			// Pre-compile all workflows
			workflowCache, err := engine.NewWorkflowCache(rc.Workflows, bootstrap.Nodes)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error compiling workflows: %s\n", err)
				os.Exit(1)
			}

			// Create and setup server
			srv, err := server.NewServer(rc, bootstrap.Services, bootstrap.Nodes, server.WithLogger(logger), server.WithWorkflowCache(workflowCache), server.WithCompiler(bootstrap.Compiler))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating server: %s\n", err)
				os.Exit(1)
			}

			if err := srv.Setup(); err != nil {
				fmt.Fprintf(os.Stderr, "Error setting up server: %s\n", err)
				os.Exit(1)
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
					fmt.Fprintf(os.Stderr, "Error starting scheduler: %s\n", err)
					os.Exit(1)
				}
				fmt.Printf("Scheduler started with %d job(s)\n", len(scheduleConfigs))
			}

			// Set up hot-reload
			reloader := devmode.NewReloader(configDir, envFlag, rc, hub, logger)
			reloader.OnReload(func(newRC *config.ResolvedConfig) {
				logger.Info("config reloaded — new workflows and routes will apply to new requests")
			})

			// Register editor API endpoints (dev mode only)
			editorAPI := server.NewEditorAPI(configDir, envFlag, reloader, plugins, bootstrap.Nodes, bootstrap.Services)
			editorAPI.Register(srv.App())

			// Serve editor static files (or placeholder if not built yet)
			editorDist := filepath.Join("editor", "dist")
			if info, err := os.Stat(editorDist); err == nil && info.IsDir() {
				srv.App().Get("/editor/*", func(c fiber.Ctx) error {
					file := c.Params("*")
					if file == "" {
						file = "index.html"
					}
					return c.SendFile(filepath.Join(editorDist, file))
				})
			} else {
				srv.App().Get("/editor", func(c fiber.Ctx) error {
					return c.SendString("Noda Visual Editor — run 'npm run build' in editor/ to enable")
				})
				srv.App().Get("/editor/*", func(c fiber.Ctx) error {
					return c.SendString("Noda Visual Editor — run 'npm run build' in editor/ to enable")
				})
			}

			// Set up file watcher
			watcher, err := devmode.NewWatcher(reloader.HandleChange, logger)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error creating file watcher: %s\n", err)
				os.Exit(1)
			}
			if err := watcher.WatchDir(configDir); err != nil {
				logger.Warn("failed to watch config directory", "error", err.Error())
			}
			watcher.Start()
			fmt.Printf("Watching %s for changes\n", configDir)

			// Mark server as ready
			server.SetReady()

			// Handle graceful shutdown
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				fmt.Println("\nShutting down...")

				deadline := 30 * time.Second
				if serverCfg, ok := rc.Root["server"].(map[string]any); ok {
					if d, ok := serverCfg["shutdown_deadline"].(string); ok {
						if parsed, err := time.ParseDuration(d); err == nil {
							deadline = parsed
						}
					}
				}

				devmode.ShutdownSequence(logger, deadline, srv, schedulerRuntime, nil, watcher, traceProvider)
				os.Exit(0)
			}()

			fmt.Printf("Noda dev server starting on port %d\n", srv.Port())
			fmt.Println("Trace WebSocket available at /ws/trace")
			fmt.Println("Editor placeholder at /editor")
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
				fmt.Fprint(os.Stderr, config.FormatErrors(errs))
				os.Exit(1)
			}

			doc, err := server.GenerateOpenAPI(rc)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error generating OpenAPI spec: %s\n", err)
				os.Exit(1)
			}

			specBytes, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error marshaling spec: %s\n", err)
				os.Exit(1)
			}

			if output != "" {
				if err := os.WriteFile(output, specBytes, 0644); err != nil {
					fmt.Fprintf(os.Stderr, "Error writing file: %s\n", err)
					os.Exit(1)
				}
				fmt.Printf("OpenAPI spec written to %s\n", output)
			} else {
				fmt.Println(string(specBytes))
			}

			return nil
		},
	}

	openAPICmd.Flags().String("output", "", "output file path (default: stdout)")

	mcpCmd := &cobra.Command{
		Use:   "mcp",
		Short: "Generate MCP server definition (stub)",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println("MCP server generation is not yet implemented.")
			fmt.Println("This will generate an MCP-compatible server definition from your Noda config.")
		},
	}

	cmd.AddCommand(openAPICmd, mcpCmd)

	return cmd
}

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run database migrations",
	}

	cmd.Flags().String("service", "main-db", "database service name from config")

	createCmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new migration",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir, _ := cmd.Flags().GetString("config")
			migrationsDir := configDir + "/migrations"

			upFile, downFile, err := migrate.Create(migrationsDir, args[0])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
				os.Exit(1)
			}
			fmt.Printf("Created:\n  %s\n  %s\n", upFile, downFile)
			return nil
		},
	}

	upCmd := &cobra.Command{
		Use:   "up",
		Short: "Apply all pending migrations",
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, configDir, err := getDBFromConfig(cmd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
				os.Exit(1)
			}

			ran, err := migrate.Up(db, configDir+"/migrations")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
				os.Exit(1)
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
			db, configDir, err := getDBFromConfig(cmd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
				os.Exit(1)
			}

			rolled, err := migrate.Down(db, configDir+"/migrations")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
				os.Exit(1)
			}
			fmt.Printf("  Rolled back: %s\n", rolled)
			return nil
		},
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show migration status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, configDir, err := getDBFromConfig(cmd)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
				os.Exit(1)
			}

			statuses, err := migrate.Status(db, configDir+"/migrations")
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err)
				os.Exit(1)
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

func getDBFromConfig(cmd *cobra.Command) (*gorm.DB, string, error) {
	configDir, _ := cmd.Flags().GetString("config")
	envFlag, _ := cmd.Flags().GetString("env")
	serviceName, _ := cmd.Flags().GetString("service")

	rc, errs := config.ValidateAll(configDir, envFlag)
	if len(errs) > 0 {
		return nil, "", fmt.Errorf("config validation failed")
	}

	// Create the database service from config
	servicesConfig, ok := rc.Root["services"].(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("no services configured")
	}

	svcConfig, ok := servicesConfig[serviceName].(map[string]any)
	if !ok {
		return nil, "", fmt.Errorf("service %q not found in config", serviceName)
	}

	plugin := &dbplugin.Plugin{}
	svc, err := plugin.CreateService(svcConfig)
	if err != nil {
		return nil, "", fmt.Errorf("create database service: %w", err)
	}

	db, ok := svc.(*gorm.DB)
	if !ok {
		return nil, "", fmt.Errorf("service %q is not a database", serviceName)
	}

	return db, configDir, nil
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
		plugins.Register(p)
	}
	for _, p := range serviceOnlyPlugins() {
		plugins.Register(p)
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
				fmt.Fprint(os.Stderr, config.FormatErrors(errs))
				os.Exit(1)
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
