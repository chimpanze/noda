package startup

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/server"
	"github.com/chimpanze/noda/plugins/all"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolve loads a testdata project the way every caller of Run does:
// config.ValidateAll first, then Run on the result.
func resolve(t *testing.T, dir string) (*config.ResolvedConfig, string) {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("../../testdata", dir))
	require.NoError(t, err)

	sm, err := config.NewSecretsManager(abs, "")
	require.NoError(t, err)
	rc, errs := config.ValidateAll(abs, "", sm)
	require.Empty(t, errs, "fixture must pass file-level validation; Run only covers what comes after")
	return rc, filepath.Join(abs, "noda.json")
}

// dryRun runs the full phase list against a fixture with fresh registries,
// the way `noda validate`, `noda test`, and MCP do.
func dryRun(t *testing.T, dir string) []Failure {
	t.Helper()
	rc, rootPath := resolve(t, dir)
	_, failures := Run(context.Background(), Input{
		RC:             rc,
		Plugins:        all.All(),
		RootConfigPath: rootPath,
		DryRun:         true,
	})
	return failures
}

func TestRun_AcceptsValidProject(t *testing.T) {
	assert.Empty(t, dryRun(t, "valid-project"))
}

// Dry-run mode must produce the artifacts later phases and boot depend on.
func TestRun_ProducesArtifacts(t *testing.T) {
	rc, rootPath := resolve(t, "valid-project")

	arts, failures := Run(context.Background(), Input{
		RC: rc, Plugins: all.All(), RootConfigPath: rootPath, DryRun: true,
	})

	require.Empty(t, failures)
	require.NotNil(t, arts)
	assert.NotNil(t, arts.Bootstrap, "cmd/noda takes its registries from here")
	assert.NotNil(t, arts.WorkflowCache, "cmd/noda takes its workflow cache from here")
}

// testdata/test-cmd-invalid-project leaves db.create's "exists" outcome
// output unwired — a registries-phase fault.
func TestRun_ReportsRegistriesFailures(t *testing.T) {
	failures := dryRun(t, "test-cmd-invalid-project")

	require.NotEmpty(t, failures)
	assert.Equal(t, PhaseRegistries, failures[0].Phase)
	assert.Contains(t, errorText(failures), "outcome output")
}

// A failing phase stops the list, matching what `noda validate` has always
// printed. The fixture must fail two phases or this asserts nothing.
func TestRun_StopsAtFirstFailingPhase(t *testing.T) {
	rc, rootPath := resolve(t, "bad-both-phases-project")

	// Guard the guard: prove a middleware fault really is waiting behind the
	// registries one.
	require.NotEmpty(t, serverMiddlewareFaults(t, rc),
		"fixture must have a middleware failure for the short-circuit to hide")

	_, failures := Run(context.Background(), Input{
		RC: rc, Plugins: all.All(), RootConfigPath: rootPath, DryRun: true,
	})

	require.NotEmpty(t, failures)
	for _, f := range failures {
		assert.Equal(t, PhaseRegistries, f.Phase,
			"no phase after the first failing one may run")
	}
}

// cyclicWorkflows returns a workflow map whose graph contains a cycle, keyed
// the way config.ValidateAll keys rc.Workflows: by source file path. Built
// the way internal/engine/compile_error_test.go does — util.log needs
// "level" and "message", and its outputs are "success"/"error", not "next".
func cyclicWorkflows() map[string]map[string]any {
	return map[string]map[string]any{
		"/proj/workflows/loop.json": {
			"id": "loop",
			"nodes": map[string]any{
				"a": map[string]any{"type": "util.log", "config": map[string]any{"level": "info", "message": "a"}},
				"b": map[string]any{"type": "util.log", "config": map[string]any{"level": "info", "message": "b"}},
			},
			"edges": []any{
				map[string]any{"from": "a", "to": "b"},
				map[string]any{"from": "b", "to": "a"},
			},
		},
	}
}

// The literal motivating bug for this package: a cycle in a workflow graph
// used to pass `noda validate` and kill boot, because the phase that would
// catch it did not exist. This pins that runWorkflows both runs and is
// attributed to the right phase and file.
func TestRun_ReportsWorkflowCycleFailure(t *testing.T) {
	rc := &config.ResolvedConfig{Workflows: cyclicWorkflows()}

	_, failures := Run(context.Background(), Input{
		RC:      rc,
		Plugins: all.All(),
		DryRun:  true,
	})

	require.Len(t, failures, 1)
	assert.Equal(t, PhaseWorkflows, failures[0].Phase)
	assert.Equal(t, []string{"/proj/workflows/loop.json"}, failures[0].Files)
	assert.Contains(t, failures[0].Err.Error(), "cycle detected")
}

// attributeRegistries points a ServiceConfigError at the root config, where
// services are declared, so a caller filtering by file needs no special case
// for registries failures that are project-wide rather than per-workflow.
func TestAttributeRegistries_ServiceConfigErrorAttributedToRootConfig(t *testing.T) {
	svcErr := &registry.ServiceConfigError{Service: "db", Plugin: "postgres", Err: errors.New("boom")}

	failures := attributeRegistries(Input{RootConfigPath: "/abs/noda.json"}, []error{svcErr})

	require.Len(t, failures, 1)
	assert.Equal(t, PhaseRegistries, failures[0].Phase)
	assert.Equal(t, []string{"/abs/noda.json"}, failures[0].Files)
}

// An empty RootConfigPath must leave Files empty, not [""]: a later caller
// filtering with slices.Contains(f.Files, absPath) would otherwise behave
// oddly against a slice holding one empty string.
func TestAttributeRegistries_EmptyRootConfigPathLeavesFilesEmpty(t *testing.T) {
	svcErr := &registry.ServiceConfigError{Service: "db", Plugin: "postgres", Err: errors.New("boom")}

	failures := attributeRegistries(Input{RootConfigPath: ""}, []error{svcErr})

	require.Len(t, failures, 1)
	assert.Empty(t, failures[0].Files)
}

// Node-type, config-schema, service-slot, edge-output, and expression checks
// in the registries phase are not individually typed the way
// ServiceConfigError is — attributeRegistries decodes the `workflow %q, ...`
// convention they all share against rc.Workflows' keys instead. Without this,
// the editor's per-file scoping (internal/editor/validation.go) would go
// blind to the most common registries-phase fault: a node's own config
// failing its plugin's schema.
func TestAttributeRegistries_WorkflowScopedErrorAttributedToItsFile(t *testing.T) {
	rc := &config.ResolvedConfig{Workflows: map[string]map[string]any{
		"/proj/workflows/hello.json": {},
		"/proj/workflows/other.json": {},
	}}
	err := errors.New(`workflow "/proj/workflows/hello.json", node "fail" (response.error): missing required config field "code"`)

	failures := attributeRegistries(Input{RC: rc}, []error{err})

	require.Len(t, failures, 1)
	assert.Equal(t, []string{"/proj/workflows/hello.json"}, failures[0].Files)
}

// An error naming no workflow file (or an RC with none registered) must leave
// Files empty rather than guessing.
func TestAttributeRegistries_UnmatchedErrorLeavesFilesEmpty(t *testing.T) {
	rc := &config.ResolvedConfig{Workflows: map[string]map[string]any{
		"/proj/workflows/hello.json": {},
	}}
	err := errors.New("register plugin \"mock\": already registered")

	failures := attributeRegistries(Input{RC: rc}, []error{err})

	require.Len(t, failures, 1)
	assert.Empty(t, failures[0].Files)
}

func TestOfPhase_SelectsOnePhase(t *testing.T) {
	failures := []Failure{
		{Phase: PhaseRegistries, Err: assertErr("a")},
		{Phase: PhaseMiddleware, Err: assertErr("b")},
	}

	assert.Len(t, OfPhase(failures, PhaseRegistries), 1)
	assert.Len(t, OfPhase(failures, PhaseMiddleware), 1)
	assert.Empty(t, OfPhase(failures, PhaseWorkflows))
	assert.Len(t, Errors(failures), 2)
}

func errorText(failures []Failure) string {
	var b strings.Builder
	for _, f := range failures {
		b.WriteString(f.Err.Error())
		b.WriteString("\n")
	}
	return b.String()
}

func assertErr(msg string) error { return errors.New(msg) }

// serverMiddlewareFaults calls the middleware phase's underlying check
// directly, so a test can prove a fixture has a fault the phase list would
// hide behind an earlier failure.
func serverMiddlewareFaults(t *testing.T, rc *config.ResolvedConfig) []error {
	t.Helper()
	return server.ValidateMiddlewareBuilds(rc)
}

// The workflow phase. A cycle passes config.ValidateAll and the registries
// phase, and killed boot at engine.NewWorkflowCache.
func TestRun_ReportsWorkflowGraphFailures(t *testing.T) {
	failures := dryRun(t, "bad-workflow-graph-project")

	require.Len(t, failures, 1)
	assert.Equal(t, PhaseWorkflows, failures[0].Phase)
	assert.Contains(t, failures[0].Err.Error(), "cycle detected")
	assert.Equal(t,
		[]string{filepath.Join(fixtureDir(t, "bad-workflow-graph-project"), "workflows", "hello.json")},
		failures[0].Files,
		"the editor marks the workflow file this cycle is in")
}

// The middleware phase — issue #456's subject, and #448's before it.
func TestRun_ReportsMiddlewareFailures(t *testing.T) {
	failures := dryRun(t, "bad-middleware-project")

	require.Len(t, failures, 1)
	assert.Equal(t, PhaseMiddleware, failures[0].Phase)
	assert.Contains(t, failures[0].Err.Error(), "limiter")

	dir := fixtureDir(t, "bad-middleware-project")
	assert.ElementsMatch(t,
		[]string{
			filepath.Join(dir, "routes", "hello.json"),
			filepath.Join(dir, "noda.json"),
		},
		failures[0].Files,
		"both the route referencing the middleware and the file defining it")
}

// The schedules phase. A five-field cron spec passed `noda validate` and
// failed at lifecycle.StartAll, after services were dialed.
func TestRun_ReportsScheduleFailures(t *testing.T) {
	failures := dryRun(t, "bad-schedule-project")

	require.Len(t, failures, 1)
	assert.Equal(t, PhaseSchedules, failures[0].Phase)
	assert.Contains(t, failures[0].Err.Error(), "expected exactly 6 fields")
	assert.Equal(t,
		[]string{filepath.Join(fixtureDir(t, "bad-schedule-project"), "schedules", "cleanup.json")},
		failures[0].Files)
}

// The workers phase. The worker schema puts no bound on concurrency.
func TestRun_ReportsWorkerFailures(t *testing.T) {
	failures := dryRun(t, "bad-worker-project")

	require.Len(t, failures, 1)
	assert.Equal(t, PhaseWorkers, failures[0].Phase)
	assert.Contains(t, failures[0].Err.Error(), "exceeds maximum")
	assert.Equal(t, "/concurrency", failures[0].JSONPath)
	assert.Equal(t,
		[]string{filepath.Join(fixtureDir(t, "bad-worker-project"), "workers", "ingest.json")},
		failures[0].Files)
}

// Guard the guards. Each fixture must fail ONLY its own phase — otherwise the
// tests above prove nothing about the phase they name. Three vacuous guards
// have shipped in this repo; this is what catches the fourth.
func TestFixtures_FailExactlyOnePhaseEach(t *testing.T) {
	for fixture, want := range map[string]Phase{
		"bad-workflow-graph-project": PhaseWorkflows,
		"bad-middleware-project":     PhaseMiddleware,
		"bad-schedule-project":       PhaseSchedules,
		"bad-worker-project":         PhaseWorkers,
	} {
		t.Run(fixture, func(t *testing.T) {
			failures := dryRun(t, fixture)
			require.NotEmpty(t, failures, "fixture must fail its phase")
			for _, f := range failures {
				assert.Equal(t, want, f.Phase,
					"fixture must fail only %s, so a guard naming it proves that phase ran", want)
			}
		})
	}
}

func fixtureDir(t *testing.T, dir string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("../../testdata", dir))
	require.NoError(t, err)
	return abs
}
