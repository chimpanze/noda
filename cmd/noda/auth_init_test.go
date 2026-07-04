package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
)

// loadResolvedConfigForTest loads and validates a project directory through
// the same pipeline `noda validate` and the runtime use, then compiles all
// workflows. It's a safety net for auth-scaffold templates: bad edge/output
// syntax, unknown node types, and unknown middleware all surface here even
// though they'd pass the lighter checks in TestAuthInitScaffold.
func loadResolvedConfigForTest(dir string) (*config.ResolvedConfig, error) {
	sm, err := config.NewSecretsManager(dir, "")
	if err != nil {
		return nil, fmt.Errorf("loading secrets: %w", err)
	}
	rc, errs := config.ValidateAll(dir, "", sm)
	if len(errs) > 0 {
		return nil, fmt.Errorf("config validation failed:\n%s", config.FormatErrors(errs))
	}

	plugins := registry.NewPluginRegistry()
	if err := registerCorePlugins(plugins); err != nil {
		return nil, err
	}
	bootstrap, bootstrapErrs := registry.Bootstrap(context.Background(), rc, plugins, registry.BootstrapOptions{DryRun: true})
	if len(bootstrapErrs) > 0 {
		var errMsgs []string
		for _, e := range bootstrapErrs {
			errMsgs = append(errMsgs, e.Error())
		}
		return nil, fmt.Errorf("bootstrap failed:\n  %s", strings.Join(errMsgs, "\n  "))
	}

	if _, err := engine.NewWorkflowCache(rc.Workflows, bootstrap.Nodes); err != nil {
		return nil, fmt.Errorf("compiling workflows: %w", err)
	}

	return rc, nil
}

func writeMinimalProject(t *testing.T, dir string, withEmail bool) {
	t.Helper()
	services := map[string]any{
		"main-db": map[string]any{"plugin": "db", "config": map[string]any{"driver": "sqlite", "path": "data/app.db"}},
	}
	if withEmail {
		services["mailer"] = map[string]any{"plugin": "email", "config": map[string]any{"host": "localhost", "port": 1025}}
	}
	root := map[string]any{"services": services}
	b, _ := json.MarshalIndent(root, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "noda.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestAuthInitScaffold(t *testing.T) {
	dir := t.TempDir()
	writeMinimalProject(t, dir, true)

	if err := runAuthInit(dir); err != nil {
		t.Fatal(err)
	}

	// files exist
	for _, glob := range []string{
		"migrations/*_auth_tables.up.sql",
		"migrations/*_auth_tables.down.sql",
		"workflows/auth.register.json",
		"workflows/auth.login.json",
		"workflows/auth.logout.json",
		"workflows/auth.me.json",
		"workflows/auth.verify-email.json",
		"workflows/auth.request-password-reset.json",
		"workflows/auth.reset-password.json",
		"routes/auth.register.json",
		"routes/auth.login.json",
		"routes/auth.logout.json",
		"routes/auth.me.json",
		"routes/auth.verify-email.json",
		"routes/auth.request-password-reset.json",
		"routes/auth.reset-password.json",
		"tests/test-auth-register.json",
		"tests/test-auth-login.json",
	} {
		m, _ := filepath.Glob(filepath.Join(dir, glob))
		if len(m) != 1 {
			t.Fatalf("missing scaffolded file: %s", glob)
		}
	}

	// db service name substituted into workflows
	b, _ := os.ReadFile(filepath.Join(dir, "workflows", "auth.login.json"))
	var wf map[string]any
	if err := json.Unmarshal(b, &wf); err != nil {
		t.Fatalf("scaffolded workflow is not valid JSON: %v", err)
	}
	if !containsString(string(b), `"main-db"`) {
		t.Fatal("db service name not substituted")
	}

	// noda.json patched
	rb, _ := os.ReadFile(filepath.Join(dir, "noda.json"))
	var root map[string]any
	json.Unmarshal(rb, &root)
	services := root["services"].(map[string]any)
	authSvc, ok := services["auth"].(map[string]any)
	if !ok || authSvc["plugin"] != "auth" {
		t.Fatal("auth service not added to noda.json")
	}
	if authSvc["config"].(map[string]any)["database"] != "main-db" {
		t.Fatal("auth service database not set")
	}

	// sqlite dialect migration (project driver is sqlite)
	m, _ := filepath.Glob(filepath.Join(dir, "migrations", "*_auth_tables.up.sql"))
	sql, _ := os.ReadFile(m[0])
	if containsString(string(sql), "JSONB") || containsString(string(sql), "TIMESTAMPTZ") {
		t.Fatal("sqlite project must get sqlite-dialect migration")
	}
}

func TestAuthInitCollisionAborts(t *testing.T) {
	dir := t.TempDir()
	writeMinimalProject(t, dir, true)
	os.MkdirAll(filepath.Join(dir, "workflows"), 0755)
	os.WriteFile(filepath.Join(dir, "workflows", "auth.login.json"), []byte("{}"), 0644)

	if err := runAuthInit(dir); err == nil {
		t.Fatal("collision must abort")
	}
	// all-or-nothing: nothing else written
	if _, err := os.Stat(filepath.Join(dir, "workflows", "auth.register.json")); err == nil {
		t.Fatal("must not write any file when aborting")
	}
}

func TestAuthInitWithoutEmailServiceWarnsButSucceeds(t *testing.T) {
	dir := t.TempDir()
	writeMinimalProject(t, dir, false)
	if err := runAuthInit(dir); err != nil {
		t.Fatalf("must succeed without email service: %v", err)
	}
}

func TestAuthInitOutputValidates(t *testing.T) {
	dir := t.TempDir()
	writeMinimalProject(t, dir, true)
	if err := runAuthInit(dir); err != nil {
		t.Fatal(err)
	}
	// Load through the real config pipeline — catches schema violations,
	// unknown node types, bad edges, and unknown middleware.
	rc, err := loadResolvedConfigForTest(dir)
	if err != nil {
		t.Fatalf("scaffolded project fails config load: %v", err)
	}
	if len(rc.Workflows) < 7 {
		t.Fatalf("expected >=7 workflows, got %d", len(rc.Workflows))
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && strings.Contains(haystack, needle)
}

// TestAuthInitRegisterRouteEnforcesPasswordLength proves scaffolded projects
// reject short/long passwords with a 400 at body-schema validation, before
// the workflow (and, for reset-password, the token-consuming node) ever
// runs. Without minLength/maxLength on the password property, a bad
// password reaches the workflow and surfaces as an unhandled `error` output
// (500), and for reset-password the token is burned before the password is
// even checked.
func TestAuthInitRegisterRouteEnforcesPasswordLength(t *testing.T) {
	dir := t.TempDir()
	writeMinimalProject(t, dir, true)
	if err := runAuthInit(dir); err != nil {
		t.Fatal(err)
	}

	for _, route := range []string{"auth.register.json", "auth.reset-password.json"} {
		b, err := os.ReadFile(filepath.Join(dir, "routes", route))
		if err != nil {
			t.Fatalf("read %s: %v", route, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(b, &doc); err != nil {
			t.Fatalf("%s: not valid JSON: %v", route, err)
		}
		props := doc["body"].(map[string]any)["schema"].(map[string]any)["properties"].(map[string]any)
		pw, ok := props["password"].(map[string]any)
		if !ok {
			t.Fatalf("%s: password property missing", route)
		}
		minLen, ok := pw["minLength"].(float64)
		if !ok || minLen != 8 {
			t.Fatalf("%s: password schema must have minLength 8, got %v", route, pw["minLength"])
		}
		if _, ok := pw["maxLength"]; !ok {
			t.Fatalf("%s: password schema must have maxLength", route)
		}
	}
}

// TestAuthInitMultipleDBServicesErrors proves the scaffold refuses to guess
// which database service to wire the auth plugin to when more than one
// exists, instead of nondeterministically picking one via map iteration.
func TestAuthInitMultipleDBServicesErrors(t *testing.T) {
	dir := t.TempDir()
	writeMinimalProject(t, dir, true)

	rb, err := os.ReadFile(filepath.Join(dir, "noda.json"))
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(rb, &root); err != nil {
		t.Fatal(err)
	}
	services := root["services"].(map[string]any)
	services["second-db"] = map[string]any{"plugin": "db", "config": map[string]any{"driver": "postgres"}}
	b, _ := json.MarshalIndent(root, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "noda.json"), b, 0644); err != nil {
		t.Fatal(err)
	}

	err = runAuthInit(dir)
	if err == nil {
		t.Fatal("expected error when multiple db services exist")
	}
	if !containsString(err.Error(), "main-db") || !containsString(err.Error(), "second-db") {
		t.Fatalf("error must list both candidate service names, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "workflows", "auth.register.json")); statErr == nil {
		t.Fatal("must not write any file when db service is ambiguous")
	}
}
