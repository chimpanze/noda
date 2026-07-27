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
