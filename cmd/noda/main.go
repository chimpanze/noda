package main

import (
	"fmt"
	"os"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/util"
	"github.com/chimpanze/noda/plugins/core/workflow"
	"github.com/spf13/cobra"
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
	)

	placeholders := []struct {
		use   string
		short string
	}{
		{"dev", "Start in development mode with hot reload"},
		{"start", "Start the production server"},
		{"generate", "Generate OpenAPI specs or client SDKs"},
		{"migrate", "Run database migrations"},
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
			// Note: real plugins will be registered here in later milestones
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

func buildCoreNodeRegistry() *registry.NodeRegistry {
	nodeReg := registry.NewNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&control.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&transform.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&util.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&workflow.Plugin{})
	return nodeReg
}
