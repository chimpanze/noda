package main

import (
	"fmt"
	"os"

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

	rootCmd.PersistentFlags().String("env", "development", "runtime environment")
	rootCmd.PersistentFlags().String("config", ".", "path to config directory")

	rootCmd.AddCommand(
		&cobra.Command{
			Use:   "version",
			Short: "Print Noda version",
			Run: func(_ *cobra.Command, _ []string) {
				fmt.Printf("noda %s\n", Version)
			},
		},
	)

	placeholders := []struct {
		use   string
		short string
	}{
		{"validate", "Validate configuration files"},
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
