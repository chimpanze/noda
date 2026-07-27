package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chimpanze/noda/internal/config"
)

// Boot must reject every project a validation surface rejects, and for the
// right reason. Asserting only require.Error would pass even if the phase
// list were bypassed entirely and some unrelated failure — e.g. a service
// dial — happened to occur first; pinning the phase closes that gap. This is
// the invariant the whole phase list exists to hold: if boot accepted one of
// these, `noda validate` would be lying about it.
func TestInitRuntime_RejectsEveryProjectValidateRejects(t *testing.T) {
	for _, tc := range []struct {
		fixture string
		want    string
	}{
		{"bad-workflow-graph-project", "workflows validation failed"},
		{"bad-middleware-project", "middleware validation failed"},
		{"bad-schedule-project", "schedules validation failed"},
		{"bad-worker-project", "workers validation failed"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			abs, err := filepath.Abs(filepath.Join("../../testdata", tc.fixture))
			require.NoError(t, err)

			_, err = initRuntime(abs, "", initOptions{})

			require.Error(t, err, "boot must not accept a project validate rejects")
			require.Contains(t, err.Error(), tc.want)
		})
	}
}

// The artifacts boot runs on must come from the phase list, not be rebuilt
// beside it. This is what stops the list from being deleted phase by phase.
//
// testdata/valid-project declares real postgres/redis services, which
// initRuntime would try to dial (DryRun defaults to false there) — so this
// uses testdata/minimal-project instead, which declares no services at all
// (see its noda.json) and exercises the same code path without a network
// dependency.
func TestInitRuntime_TakesArtifactsFromTheStartupPhases(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("../../testdata", "minimal-project"))
	require.NoError(t, err)

	rtCtx, err := initRuntime(abs, "", initOptions{})
	require.NoError(t, err)

	require.NotNil(t, rtCtx.Bootstrap)
	require.NotNil(t, rtCtx.WorkflowCache)
}

// Dev-mode reload must refuse a config that fails any startup phase, or the
// editor's save reports success for a project that will not boot.
//
// This calls devModeDryRun — the function `noda dev` hands to the reloader —
// and not startup.Run directly. Calling startup.Run is what the previous
// version of this test did, which made it vacuous: reverting devModeDryRun's
// body to registry.DryRun, undoing the dev-mode half of #456 and letting a
// reload accept a cyclic workflow or a five-field cron spec, left the whole
// suite green.
//
// The shape is dev-mode's own: a good project boots and supplies the live
// registries, then a reload delivers a config that fails a phase.
func TestDevModeDryRun_RefusesEveryFailingPhase(t *testing.T) {
	// minimal-project declares no services, so initRuntime needs no network
	// (see TestInitRuntime_TakesArtifactsFromTheStartupPhases).
	bootDir, err := filepath.Abs(filepath.Join("../../testdata", "minimal-project"))
	require.NoError(t, err)
	rtCtx, err := initRuntime(bootDir, "", initOptions{})
	require.NoError(t, err)

	for _, tc := range []struct {
		fixture string
		phase   string
	}{
		{"bad-workflow-graph-project", "cycle detected"},
		{"bad-middleware-project", "limiter"},
		{"bad-schedule-project", "expected exactly 6 fields"},
		{"bad-worker-project", "exceeds maximum"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			abs, err := filepath.Abs(filepath.Join("../../testdata", tc.fixture))
			require.NoError(t, err)
			sm, err := config.NewSecretsManager(abs, "")
			require.NoError(t, err)
			rc, cfgErrs := config.ValidateAll(abs, "", sm)
			require.Empty(t, cfgErrs)

			errs := devModeDryRun(rtCtx, abs)(rc)

			require.NotEmpty(t, errs, "a reload of this project must be refused")
			// Pinning the reason, not just "some error": without it this
			// passes for any phase failing, so dropping four of the five
			// phases would still look fine.
			var joined strings.Builder
			for _, e := range errs {
				joined.WriteString(e.Error())
				joined.WriteString("\n")
			}
			assert.Contains(t, joined.String(), tc.phase)
		})
	}
}

// A reload of a project that boots must be accepted — the guard above would
// otherwise be satisfied by a hook that refuses everything.
func TestDevModeDryRun_AcceptsAReloadableProject(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("../../testdata", "minimal-project"))
	require.NoError(t, err)
	rtCtx, err := initRuntime(abs, "", initOptions{})
	require.NoError(t, err)

	sm, err := config.NewSecretsManager(abs, "")
	require.NoError(t, err)
	rc, cfgErrs := config.ValidateAll(abs, "", sm)
	require.Empty(t, cfgErrs)

	assert.Empty(t, devModeDryRun(rtCtx, abs)(rc))
}
