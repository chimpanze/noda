package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/startup"
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
func TestDevModeDryRun_RefusesEveryFailingPhase(t *testing.T) {
	for _, fixture := range []string{
		"bad-workflow-graph-project",
		"bad-middleware-project",
		"bad-schedule-project",
		"bad-worker-project",
	} {
		t.Run(fixture, func(t *testing.T) {
			abs, err := filepath.Abs(filepath.Join("../../testdata", fixture))
			require.NoError(t, err)
			sm, err := config.NewSecretsManager(abs, "")
			require.NoError(t, err)
			rc, cfgErrs := config.ValidateAll(abs, "", sm)
			require.Empty(t, cfgErrs)

			// The same call the dev-mode hook makes, with fresh registries
			// standing in for the running server's.
			_, failures := startup.Run(context.Background(), startup.Input{
				RC:             rc,
				Plugins:        allPlugins(),
				RootConfigPath: filepath.Join(abs, "noda.json"),
				DryRun:         true,
			})

			assert.NotEmpty(t, startup.Errors(failures),
				"a reload of this project must be refused")
		})
	}
}
