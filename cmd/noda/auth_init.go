package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
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
	dbNames, driverByName := findServicesByPlugin(services, "db")
	if len(dbNames) == 0 {
		return fmt.Errorf("auth init: no database service (plugin \"db\") found in noda.json — add one first")
	}
	if len(dbNames) > 1 {
		return fmt.Errorf("auth init: multiple database services (plugin \"db\") found: %s — rename or remove services so only one remains, then re-run", strings.Join(dbNames, ", "))
	}
	dbName := dbNames[0]
	driver := driverByName[dbName]
	if driver == "" {
		driver = "postgres"
	}
	emailNames, _ := findServicesByPlugin(services, "email")
	var emailName string
	switch len(emailNames) {
	case 0:
		emailName = "email"
		fmt.Fprintln(os.Stderr, "warning: no email service configured — verify-email and password-reset flows need a service named \"email\" (or edit the generated workflows)")
	case 1:
		emailName = emailNames[0]
	default:
		emailName = emailNames[0]
		fmt.Fprintf(os.Stderr, "warning: multiple email services found (%s) — using %q\n", strings.Join(emailNames, ", "), emailName)
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

	// The scaffolded login/register/request-password-reset/reset-password
	// routes reference the "limiter" middleware by name; it must have a
	// config with an explicit "max" (there is no default — the server
	// refuses to start otherwise). Only add it if the project hasn't
	// already configured its own limiter.
	middlewareCfg, _ := root["middleware"].(map[string]any)
	if middlewareCfg == nil {
		middlewareCfg = map[string]any{}
	}
	if _, exists := middlewareCfg["limiter"]; !exists {
		middlewareCfg["limiter"] = map[string]any{"max": 20, "expiration": "1m"}
	}
	root["middleware"] = middlewareCfg

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

// findServicesByPlugin returns the sorted names of every service configured
// with the given plugin, plus a name->driver lookup (driver is read from
// config.driver, empty string if absent). Sorting makes the scaffold
// deterministic: iterating a map directly (the previous behavior) meant
// that, with more than one matching service, which one got picked was
// random from run to run.
func findServicesByPlugin(services map[string]any, pluginName string) (names []string, driverByName map[string]string) {
	driverByName = map[string]string{}
	for n, v := range services {
		svc, ok := v.(map[string]any)
		if !ok || svc["plugin"] != pluginName {
			continue
		}
		cfg, _ := svc["config"].(map[string]any)
		d, _ := cfg["driver"].(string)
		names = append(names, n)
		driverByName[n] = d
	}
	sort.Strings(names)
	return names, driverByName
}
