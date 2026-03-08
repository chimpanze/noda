package main

import (
	"fmt"
	"os"

	"github.com/chimpanze/noda/internal/config"
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
	)

	placeholders := []struct {
		use   string
		short string
	}{
		{"dev", "Start in development mode with hot reload"},
		{"start", "Start the production server"},
		{"test", "Run workflow tests"},
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

			fmt.Printf("✓ All config files valid (%d files checked)\n", rc.FileCount)
			return nil
		},
	}

	cmd.Flags().Bool("verbose", false, "show detailed validation info")

	return cmd
}
