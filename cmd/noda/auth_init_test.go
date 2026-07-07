package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/chimpanze/noda/pkg/api"
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
		"workflows/auth.resend-verification.json",
		"routes/auth.register.json",
		"routes/auth.login.json",
		"routes/auth.logout.json",
		"routes/auth.me.json",
		"routes/auth.verify-email.json",
		"routes/auth.request-password-reset.json",
		"routes/auth.reset-password.json",
		"routes/auth.resend-verification.json",
		"tests/test-auth-register.json",
		"tests/test-auth-login.json",
		"tests/test-auth-resend-verification.json",
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
	if err := json.Unmarshal(rb, &root); err != nil {
		t.Fatalf("patched noda.json is not valid JSON: %v", err)
	}
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
	if err := os.MkdirAll(filepath.Join(dir, "workflows"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflows", "auth.login.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

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
	if len(rc.Workflows) < 8 {
		t.Fatalf("expected >=8 workflows, got %d", len(rc.Workflows))
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) && strings.Contains(haystack, needle)
}

// scaffoldAuthProject scaffolds auth into a fresh temp project directory and
// returns the directory. Shared by TestAuthInitScaffold-style tests and the
// runner-based behavior tests below.
func scaffoldAuthProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeMinimalProject(t, dir, true)
	require.NoError(t, runAuthInit(dir))
	return dir
}

// runScaffoldedAuthSuite scaffolds auth into a temp project, then runs one
// scaffolded test suite through the real workflow-test runner and asserts every
// case passes. This exercises the rendered templates end-to-end.
func runScaffoldedAuthSuite(t *testing.T, suiteID string) {
	t.Helper()
	dir := scaffoldAuthProject(t)
	rc, err := loadResolvedConfigForTest(dir)
	require.NoError(t, err)

	suites, err := nodatesting.LoadTests(rc)
	require.NoError(t, err)

	reg, err := buildCoreNodeRegistry()
	require.NoError(t, err)

	var ran bool
	for _, suite := range suites {
		if suite.ID != suiteID {
			continue
		}
		ran = true
		for _, res := range nodatesting.RunTestSuite(suite, rc, reg) {
			assert.Truef(t, res.Passed, "case %q failed: %s", res.CaseName, res.Error)
		}
	}
	require.Truef(t, ran, "suite %q not found among scaffolded tests", suiteID)
}

func TestAuthScaffold_RegisterIsAntiEnumerating(t *testing.T) {
	runScaffoldedAuthSuite(t, "test-auth-register")
}

func TestAuthScaffold_RequestPasswordResetIsConstantTime(t *testing.T) {
	runScaffoldedAuthSuite(t, "test-auth-request-password-reset")
}

func TestAuthScaffold_ResendVerificationIsConstantTime(t *testing.T) {
	runScaffoldedAuthSuite(t, "test-auth-resend-verification")
}

func TestAuthScaffold_ResetPasswordIsAtomic(t *testing.T) {
	runScaffoldedAuthSuite(t, "test-auth-reset-password")
}

// TestScratch_PasswordResetPadExpressionResolvesUnmocked is a one-off proof
// that the pad_* timeout expression in
// auth_templates/workflows/auth.request-password-reset.json.tmpl actually
// resolves against real (unmocked) util.timestamp output. The scaffolded
// suite mocks pad_* directly, so it would pass even if the expression
// referenced a nonexistent field like `nodes.start_ts.value` — this test is
// the only one that exercises the real util.timestamp -> computed
// util.delay timeout -> time.ParseDuration path. It mirrors just the
// "unknown email" timing chain (start_ts -> now_ts_unknown -> pad_unknown ->
// respond_unknown) with a short 50ms deadline (instead of the shipped
// template's 500ms) so it stays fast; the shipped template itself keeps
// T=500ms.
func TestScratch_PasswordResetPadExpressionResolvesUnmocked(t *testing.T) {
	nodeReg, err := buildCoreNodeRegistry()
	require.NoError(t, err)
	svcReg := registry.NewServiceRegistry()

	wf := engine.WorkflowConfig{
		ID: "scratch-pad-unknown",
		Nodes: map[string]engine.NodeConfig{
			"start_ts":       {Type: "util.timestamp", Config: map[string]any{"format": "unix_ms"}},
			"now_ts_unknown": {Type: "util.timestamp", Config: map[string]any{"format": "unix_ms"}},
			"pad_unknown": {Type: "util.delay", Config: map[string]any{
				"timeout": "{{ (nodes.start_ts + 50) > nodes.now_ts_unknown ? (nodes.start_ts + 50 - nodes.now_ts_unknown) : 0 }}ms",
			}},
			"respond_unknown": {Type: "response.json", Config: map[string]any{
				"status": 200,
				"body":   map[string]any{"message": "If that account exists, an email was sent"},
			}},
		},
		Edges: []engine.EdgeConfig{
			{From: "start_ts", To: "now_ts_unknown"},
			{From: "now_ts_unknown", To: "pad_unknown"},
			{From: "pad_unknown", To: "respond_unknown"},
		},
	}

	graph, err := engine.Compile(wf, &engine.DefaultOutputResolver{})
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext()
	start := time.Now()
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	elapsed := time.Since(start)
	require.NoError(t, err, "real util.timestamp -> util.delay pad expression must resolve without error")

	// Duration floor: the pad must actually wait toward the ~50ms deadline. This
	// guards against a regression where the pad expression silently resolves to
	// "0ms" (e.g. swapped operands or a broken ternary) — which would still reach
	// respond_unknown at 200 and pass a status-only assertion.
	assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond,
		"pad delay must approach the 50ms deadline; got %s (expression may have resolved to ~0ms)", elapsed)

	out, ok := execCtx.GetOutput("respond_unknown")
	require.True(t, ok, "respond_unknown must have run")
	resp, ok := out.(*api.HTTPResponse)
	require.True(t, ok, "respond_unknown output must be *api.HTTPResponse, got %T", out)
	assert.Equal(t, 200, resp.Status)
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
