package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

// templates/* excludes dotfiles; list them explicitly.
//go:embed templates/* templates/.env.example templates/.claude/settings.json
var templateFS embed.FS

type projectData struct {
	ProjectName string
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [project-name]",
		Short: "Initialize a new Noda project",
		Long:  "Scaffold a new Noda project with config files, Docker Compose, and sample routes/workflows.",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]
			return scaffoldProject(name)
		},
	}
	return cmd
}

func scaffoldProject(name string) error {
	data := projectData{
		ProjectName: filepath.Base(name),
	}

	// Also create the migrations directory (empty, not in templates)
	if err := os.MkdirAll(filepath.Join(name, "migrations"), 0755); err != nil {
		return fmt.Errorf("create migrations directory: %w", err)
	}

	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip root "templates" entry
		if path == "templates" {
			return nil
		}

		// Strip the "templates/" prefix to get the relative output path
		relPath := strings.TrimPrefix(path, "templates/")

		outPath := filepath.Join(name, relPath)

		if d.IsDir() {
			return os.MkdirAll(outPath, 0755)
		}

		content, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read template %s: %w", path, err)
		}

		// Process .tmpl files with text/template
		if strings.HasSuffix(relPath, ".tmpl") {
			outPath = strings.TrimSuffix(outPath, ".tmpl")
			tmpl, err := template.New(filepath.Base(path)).Delims("[[", "]]").Parse(string(content))
			if err != nil {
				return fmt.Errorf("parse template %s: %w", path, err)
			}
			var buf strings.Builder
			if err := tmpl.Execute(&buf, data); err != nil {
				return fmt.Errorf("execute template %s: %w", path, err)
			}
			content = []byte(buf.String())
		}

		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("create directory for %s: %w", outPath, err)
		}

		return os.WriteFile(outPath, content, 0644)
	})
	if err != nil {
		return fmt.Errorf("scaffold project: %w", err)
	}

	fmt.Printf("✓ Project %q created\n", filepath.Base(name))
	fmt.Println()
	fmt.Println("  Get started:")
	fmt.Printf("    cd %s\n", filepath.Base(name))
	fmt.Println("    cp .env.example .env")
	fmt.Println("    docker compose up -d")
	fmt.Println("    noda dev")
	fmt.Println()
	return nil
}
