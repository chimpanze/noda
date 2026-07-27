package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/startup"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validateProject must accept a project that `noda validate` accepts.
func TestValidateProject_AcceptsValidProject(t *testing.T) {
	dir := "../../testdata/valid-project"

	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)
	rc, errs := config.ValidateAll(dir, "", sm)
	require.Empty(t, errs)

	require.NoError(t, validateProject(rc))
}

// testdata/test-cmd-invalid-project is the #444 trap in fixture form: its
// config parses, its workflow test passes, and it cannot boot. Asserting all
// three here is what makes TestTestCmd_FailsOnProjectThatDoesNotValidate
// meaningful — without the middle assertion, that test could pass for the
// boring reason that the fixture's tests were failing anyway.
// The middleware phase has its own heading, and "output unchanged" is the
// contract of the #448 refactor — so the formatter needs pinning, not just
// the bootstrap path above it.
func TestValidateProject_RejectsMiddlewareBuildFailure(t *testing.T) {
	dir := "../../testdata/bad-middleware-project"

	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)
	rc, errs := config.ValidateAll(dir, "", sm)
	require.Empty(t, errs)

	err = validateProject(rc)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "middleware validation failed:\n  "),
		"unexpected heading or indent: %q", err.Error())
	assert.Contains(t, err.Error(), "limiter")
}

func TestValidateProject_RejectsProjectThatCannotBoot(t *testing.T) {
	dir := "../../testdata/test-cmd-invalid-project"

	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)

	rc, errs := config.ValidateAll(dir, "", sm)
	require.Empty(t, errs, "fixture must pass config validation — the gap only exists past that point")

	suites, err := nodatesting.LoadTests(rc)
	require.NoError(t, err)
	require.NotEmpty(t, suites)

	reg, err := buildCoreNodeRegistry()
	require.NoError(t, err)
	ran := 0
	for _, suite := range suites {
		for _, res := range nodatesting.RunTestSuite(suite, rc, reg, sm.ExpressionContext()) {
			ran++
			require.Truef(t, res.Passed,
				"fixture's own tests must pass: suite %q case %q: %s", suite.ID, res.CaseName, res.Error)
		}
	}
	require.Positive(t, ran, "fixture must actually define test cases — an empty tests array would make the assertion above vacuous")

	err = validateProject(rc)
	require.Error(t, err, "unwired outcome output must fail startup validation")
	assert.Contains(t, err.Error(), "outcome output")
	assert.Contains(t, err.Error(), "exists")
}

// Every phase must reach the CLI's output. Deleting any one of these from the
// startup list must redden this test — that is the whole point of the list.
func TestValidateProject_ReportsEveryPhase(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		heading string
		detail  string
	}{
		{"test-cmd-invalid-project", "registries validation failed", "outcome output"},
		{"bad-workflow-graph-project", "workflows validation failed", "cycle detected"},
		{"bad-middleware-project", "middleware validation failed", "limiter"},
		{"bad-schedule-project", "schedules validation failed", "expected exactly 6 fields"},
		{"bad-worker-project", "workers validation failed", "exceeds maximum"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			abs, err := filepath.Abs(filepath.Join("../../testdata", tc.fixture))
			require.NoError(t, err)
			sm, err := config.NewSecretsManager(abs, "")
			require.NoError(t, err)
			rc, cfgErrs := config.ValidateAll(abs, "", sm)
			require.Empty(t, cfgErrs, "fixture must fail a startup phase, not file validation")

			err = validateProject(rc)

			require.Error(t, err, "the %s phase must reach the CLI", tc.heading)
			assert.Contains(t, err.Error(), tc.heading)
			assert.Contains(t, err.Error(), tc.detail)
		})
	}
}

// A failure whose phase this renderer does not know must still be reported.
//
// The previous renderer iterated a hardcoded list of the five phases and
// returned nil when nothing matched, so a sixth phase added to
// internal/startup would have been dropped here and `noda validate` would
// have printed "✓ All config files valid" and exited 0 for a project that
// phase rejects — the exact drift the phase list exists to end. The
// incomplete-artifacts failure, whose phase is deliberately outside
// startup.Phases(), was being dropped that way already.
func TestRenderStartupFailures_NeverDropsAnUnknownPhase(t *testing.T) {
	for _, tc := range []struct {
		name  string
		phase startup.Phase
	}{
		{"a phase added to internal/startup but not to Phases()", startup.Phase("newphase")},
		{"the incomplete-artifacts check, which is not an executed phase", startup.PhaseArtifacts},
		{"no phase set at all", startup.Phase("")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := renderStartupFailures([]startup.Failure{
				{Phase: tc.phase, Err: errors.New("boom: this project cannot boot")},
			})

			require.Error(t, err, "a failure must never render as success")
			assert.Contains(t, err.Error(), "boom: this project cannot boot",
				"the failure's own text must reach the user, whatever its phase")
		})
	}
}

// Every phase in the list keeps its own heading — the catch-all above must not
// swallow the specific ones.
func TestRenderStartupFailures_HeadsEveryListedPhase(t *testing.T) {
	for _, phase := range startup.Phases() {
		t.Run(string(phase), func(t *testing.T) {
			err := renderStartupFailures([]startup.Failure{
				{Phase: phase, Err: errors.New("boom")},
			})
			require.Error(t, err)
			assert.Equal(t, string(phase)+" validation failed:\n  boom", err.Error())
		})
	}
}
