package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
)

//go:embed auth_templates
var authTemplateFS embed.FS

func newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication scaffolding",
	}
	var dir string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold auth flows (routes, workflows, migrations, tests) into this project",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runAuthInit(dir)
		},
	}
	initCmd.Flags().StringVar(&dir, "dir", ".", "project directory (must contain noda.json)")
	cmd.AddCommand(initCmd)
	return cmd
}

type authScaffoldData struct {
	DBService    string
	EmailService string
}

func runAuthInit(dir string) error {
	rootPath := filepath.Join(dir, "noda.json")
	rootBytes, err := os.ReadFile(rootPath)
	if err != nil {
		return fmt.Errorf("auth init: read noda.json: %w", err)
	}
	var root map[string]any
	if err := json.Unmarshal(rootBytes, &root); err != nil {
		return fmt.Errorf("auth init: parse noda.json: %w", err)
	}

	services, _ := root["services"].(map[string]any)
	dbName, driver := findServiceByPlugin(services, "db")
	if dbName == "" {
		return fmt.Errorf("auth init: no database service (plugin \"db\") found in noda.json — add one first")
	}
	if driver == "" {
		driver = "postgres"
	}
	emailName, _ := findServiceByPlugin(services, "email")
	if emailName == "" {
		emailName = "email"
		fmt.Fprintln(os.Stderr, "warning: no email service configured — verify-email and password-reset flows need a service named \"email\" (or edit the generated workflows)")
	}
	if _, exists := services["auth"]; exists {
		return fmt.Errorf("auth init: services.auth already exists in noda.json")
	}

	data := authScaffoldData{DBService: dbName, EmailService: emailName}
	timestamp := time.Now().UTC().Format("20060102150405")

	// Build the full output set in memory first (all-or-nothing).
	outputs := map[string][]byte{}

	upSQL, err := authTemplateFS.ReadFile("auth_templates/migrations/" + driver + ".up.sql")
	if err != nil {
		return fmt.Errorf("auth init: unsupported db driver %q", driver)
	}
	downSQL, _ := authTemplateFS.ReadFile("auth_templates/migrations/" + driver + ".down.sql")
	outputs[filepath.Join("migrations", timestamp+"_auth_tables.up.sql")] = upSQL
	outputs[filepath.Join("migrations", timestamp+"_auth_tables.down.sql")] = downSQL

	err = fs.WalkDir(authTemplateFS, "auth_templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasPrefix(path, "auth_templates/migrations/") {
			return err
		}
		rel := strings.TrimPrefix(path, "auth_templates/")
		content, err := authTemplateFS.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.HasSuffix(rel, ".tmpl") {
			tpl, err := template.New(rel).Delims("[[", "]]").Parse(string(content))
			if err != nil {
				return fmt.Errorf("template %s: %w", rel, err)
			}
			var buf strings.Builder
			if err := tpl.Execute(&buf, data); err != nil {
				return fmt.Errorf("template %s: %w", rel, err)
			}
			content = []byte(buf.String())
			rel = strings.TrimSuffix(rel, ".tmpl")
		}
		outputs[rel] = content
		return nil
	})
	if err != nil {
		return fmt.Errorf("auth init: %w", err)
	}

	// Collision check before writing anything.
	var collisions []string
	for rel := range outputs {
		if _, err := os.Stat(filepath.Join(dir, rel)); err == nil {
			collisions = append(collisions, rel)
		}
	}
	if len(collisions) > 0 {
		return fmt.Errorf("auth init: refusing to overwrite existing files:\n  %s", strings.Join(collisions, "\n  "))
	}

	for rel, content := range outputs {
		target := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("auth init: %w", err)
		}
		if err := os.WriteFile(target, content, 0644); err != nil {
			return fmt.Errorf("auth init: %w", err)
		}
	}

	// Patch noda.json: services.auth + middleware preset.
	services["auth"] = map[string]any{
		"plugin": "auth",
		"config": map[string]any{"database": dbName},
	}
	root["services"] = services
	presets, _ := root["middleware_presets"].(map[string]any)
	if presets == nil {
		presets = map[string]any{}
	}
	if _, exists := presets["authenticated_session"]; !exists {
		presets["authenticated_session"] = []any{"auth.session"}
	}
	root["middleware_presets"] = presets
	patched, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("auth init: %w", err)
	}
	if err := os.WriteFile(rootPath, append(patched, '\n'), 0644); err != nil {
		return fmt.Errorf("auth init: %w", err)
	}

	fmt.Printf("Scaffolded auth: %d files + noda.json updated.\nNext steps:\n  1. noda migrate up\n  2. Open the auth-* workflows in the editor and customize\n  3. noda test\n", len(outputs))
	return nil
}

func findServiceByPlugin(services map[string]any, pluginName string) (name, driver string) {
	for n, v := range services {
		svc, ok := v.(map[string]any)
		if !ok || svc["plugin"] != pluginName {
			continue
		}
		cfg, _ := svc["config"].(map[string]any)
		d, _ := cfg["driver"].(string)
		return n, d
	}
	return "", ""
}
