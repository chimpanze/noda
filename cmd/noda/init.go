package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/chimpanze/noda/internal/scaffold"
)

// templates/* excludes dotfiles; list them explicitly.
//
//go:embed templates/* templates/.env.example templates/.mcp.json templates/.claude/settings.json
var templateFS embed.FS

type projectData struct {
	ProjectName string
}

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

func scaffoldProject(name string, force bool) error {
	data := projectData{
		ProjectName: filepath.Base(name),
	}

	if !force {
		var conflicts []string
		err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == "templates" || d.IsDir() {
				return nil
			}
			relPath := strings.TrimPrefix(path, "templates/")
			outPath := filepath.Join(name, relPath)
			if strings.HasSuffix(relPath, ".tmpl") {
				outPath = strings.TrimSuffix(outPath, ".tmpl")
			}
			if _, statErr := os.Stat(outPath); statErr == nil {
				conflicts = append(conflicts, outPath)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("scaffold project: %w", err)
		}
		// .env is generated (not a template file), but must still be conflict-checked.
		if _, statErr := os.Stat(filepath.Join(name, ".env")); statErr == nil {
			conflicts = append(conflicts, filepath.Join(name, ".env"))
		}
		if len(conflicts) > 0 {
			sort.Strings(conflicts)
			return fmt.Errorf("refusing to overwrite existing files (use --force): %s", strings.Join(conflicts, ", "))
		}
	}

	// Also create the migrations directory (empty, not in templates)
	if err := os.MkdirAll(filepath.Join(name, "migrations"), 0755); err != nil {
		return fmt.Errorf("create migrations directory: %w", err)
	}

	var envExampleContent string

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

		if relPath == ".env.example" {
			envExampleContent = string(content)
		}

		return os.WriteFile(outPath, content, 0644)
	})
	if err != nil {
		return fmt.Errorf("scaffold project: %w", err)
	}

	// Generate a real, ready-to-use .env alongside the committable .env.example
	// so a fresh project has a compliant JWT_SECRET (>=32 bytes) from the start (#381).
	if envExampleContent != "" {
		secret, err := scaffold.GenerateJWTSecret()
		if err != nil {
			return fmt.Errorf("scaffold project: %w", err)
		}
		envContent := scaffold.ApplyJWTSecret(envExampleContent, secret)
		if err := os.WriteFile(filepath.Join(name, ".env"), []byte(envContent), 0644); err != nil {
			return fmt.Errorf("write .env: %w", err)
		}
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
