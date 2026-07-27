package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/chimpanze/noda/internal/scaffold"
)

func newInitCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init [project-name]",
		Short: "Initialize a new Noda project",
		Long:  "Scaffold a new Noda project with config files, Docker Compose, and sample routes/workflows.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			return scaffoldProject(name, force)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing files")
	return cmd
}

// scaffoldProject writes the shared project scaffold (internal/scaffold) into
// name. The templates and the write itself live there rather than here so that
// `noda init` and the MCP noda_scaffold_project tool cannot drift apart (#449).
func scaffoldProject(name string, force bool) error {
	if _, err := scaffold.WriteProject(name, force); err != nil {
		var conflict *scaffold.ConflictError
		if errors.As(err, &conflict) {
			return fmt.Errorf("refusing to overwrite existing files (use --force): %s", strings.Join(conflict.Paths, ", "))
		}
		return fmt.Errorf("scaffold project: %w", err)
	}

	fmt.Printf("✓ Project %q created\n", filepath.Base(name))
	fmt.Println()
	fmt.Println("  Get started:")
	fmt.Printf("    cd %s\n", filepath.Base(name))
	fmt.Println("    docker compose up -d")
	fmt.Println("    noda dev")
	fmt.Println()
	fmt.Println("  (.env was generated with a unique JWT_SECRET; .env.example is the committable template)")
	return nil
}
