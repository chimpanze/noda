package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/migrate"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/scheduler"
	"github.com/chimpanze/noda/internal/server"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/util"
	"github.com/chimpanze/noda/plugins/core/workflow"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/chimpanze/noda/plugins/core/event"
	corestorage "github.com/chimpanze/noda/plugins/core/storage"
	"github.com/chimpanze/noda/plugins/core/upload"
	dbplugin "github.com/chimpanze/noda/plugins/db"
	imageplugin "github.com/chimpanze/noda/plugins/image"
	pubsubplugin "github.com/chimpanze/noda/plugins/pubsub"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
	"github.com/spf13/cobra"
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
			Short: "Print Noda version",
			Run: func(_ *cobra.Command, _ []string) {
				fmt.Printf("noda %s\n", Version)
			},
		},
		newValidateCmd(),
		newTestCmd(),
		newStartCmd(),
		newGenerateCmd(),
		newMigrateCmd(),
		newScheduleCmd(),
	)

	placeholders := []struct {
		use   string
		short string
	}{
		{"dev", "Start in development mode with hot reload"},
		{"init", "Initialize a new Noda project"},
		{"plugin", "Manage plugins"},
	}

	for _, p := range placeholders {
		p := p
		rootCmd.AddCommand(&cobra.Command{
			Use:   p.use,
			Short: p.short,
			Run: func(_ *cobra.Command, _ []string) {
				fmt.Printf("%s: not yet implemented\n", p.use)
			},
		})
	}

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

			// Plugin/service/node startup validation
			plugins := registry.NewPluginRegistry()
			_, bootstrapErrs := registry.Bootstrap(rc, plugins)
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			configDir, _ := cmd.Flags().GetString("config")
			envFlag, _ := cmd.Flags().GetString("env")

			// Load and validate config
			rc, errs := config.ValidateAll(configDir, envFlag)
			if len(errs) > 0 {
				fmt.Fprint(os.Stderr, config.FormatErrors(errs))
				os.Exit(1)
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

			// Create and setup server
			srv, err := server.NewServer(rc, bootstrap.Services, bootstrap.Nodes)
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
				fmt.Fprintf(os.Stderr, "Warning: OpenAPI generation failed: %s\n", err)
			}

			// Start scheduler if schedules are configured
			var schedulerRuntime *scheduler.Runtime
			if len(rc.Schedules) > 0 {
				scheduleConfigs := scheduler.ParseScheduleConfigs(rc.Schedules)
				schedulerRuntime = scheduler.NewRuntime(
					scheduleConfigs,
					bootstrap.Services,
					bootstrap.Nodes,
					rc.Workflows,
					nil,
				)
				if err := schedulerRuntime.Start(); err != nil {
					fmt.Fprintf(os.Stderr, "Error starting scheduler: %s\n", err)
					os.Exit(1)
				}
				fmt.Printf("Scheduler started with %d job(s)\n", len(scheduleConfigs))
			}

			// Handle graceful shutdown
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				fmt.Println("\nShutting down...")
				if schedulerRuntime != nil {
					schedulerRuntime.Stop()
				}
				_ = srv.Stop()
			}()

			fmt.Printf("Noda server starting on port %d\n", srv.Port())
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
	cmd.AddCommand(openAPICmd)

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

func buildCoreNodeRegistry() *registry.NodeRegistry {
	nodeReg := registry.NewNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&control.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&transform.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&util.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&workflow.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&response.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&dbplugin.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&cacheplugin.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&event.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&corestorage.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&upload.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&imageplugin.Plugin{})
	return nodeReg
}

func registerCorePlugins(plugins *registry.PluginRegistry) {
	plugins.Register(&control.Plugin{})
	plugins.Register(&transform.Plugin{})
	plugins.Register(&util.Plugin{})
	plugins.Register(&workflow.Plugin{})
	plugins.Register(&response.Plugin{})
	plugins.Register(&dbplugin.Plugin{})
	plugins.Register(&cacheplugin.Plugin{})
	plugins.Register(&streamplugin.Plugin{})
	plugins.Register(&pubsubplugin.Plugin{})
	plugins.Register(&event.Plugin{})
	plugins.Register(&storageplugin.Plugin{})
	plugins.Register(&corestorage.Plugin{})
	plugins.Register(&upload.Plugin{})
	plugins.Register(&imageplugin.Plugin{})
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
