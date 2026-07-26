package validate

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolve loads a testdata project the way every caller of Project does:
// config.ValidateAll first, then Project on the result.
func resolve(t *testing.T, dir string) *config.ResolvedConfig {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("../../testdata", dir))
	require.NoError(t, err)

	sm, err := config.NewSecretsManager(abs, "")
	require.NoError(t, err)
	rc, errs := config.ValidateAll(abs, "", sm)
	require.Empty(t, errs, "fixture must pass file-level validation; Project only runs past that point")
	return rc
}

func TestProject_AcceptsValidProject(t *testing.T) {
	res, err := Project(context.Background(), resolve(t, "valid-project"))
	require.NoError(t, err)
	assert.True(t, res.OK(), "unexpected failures: %v", res.Errors())
}

// The bootstrap phase catches what node config schemas and the service graph
// reject. testdata/test-cmd-invalid-project leaves db.create's "exists"
// outcome output unwired.
func TestProject_ReportsBootstrapFailures(t *testing.T) {
	res, err := Project(context.Background(), resolve(t, "test-cmd-invalid-project"))
	require.NoError(t, err)

	require.False(t, res.OK())
	require.NotEmpty(t, res.Bootstrap)
	assert.Contains(t, errorText(res.Bootstrap), "outcome output")
}

// The middleware phase is the one #448 was about: it is invisible to the
// dry-run bootstrap, so a project can pass every node/service check and still
// be unable to boot.
func TestProject_ReportsMiddlewareFailures(t *testing.T) {
	res, err := Project(context.Background(), resolve(t, "bad-middleware-project"))
	require.NoError(t, err)

	require.False(t, res.OK())
	assert.Empty(t, res.Bootstrap, "fixture is meant to fail only the middleware phase")
	require.NotEmpty(t, res.Middleware)
	assert.Contains(t, errorText(res.Middleware), "limiter")
}

// Bootstrap failures short-circuit the middleware phase, matching what
// `noda validate` has always printed. If this changes, the CLI's output
// changes with it.
func TestProject_BootstrapFailureSkipsMiddlewarePhase(t *testing.T) {
	res, err := Project(context.Background(), resolve(t, "test-cmd-invalid-project"))
	require.NoError(t, err)

	require.NotEmpty(t, res.Bootstrap)
	assert.Empty(t, res.Middleware)
}

func TestResult_OKAndErrorOrdering(t *testing.T) {
	var empty Result
	assert.True(t, empty.OK())
	assert.Empty(t, empty.Errors())

	boot := errors.New("boot")
	mw := errors.New("mw")
	res := Result{Bootstrap: []error{boot}, Middleware: []error{mw}}

	assert.False(t, res.OK())
	// Bootstrap first: it describes the earlier startup phase, so a reader
	// sees failures in the order the runtime would hit them.
	assert.Equal(t, []error{boot, mw}, res.Errors())
}

func errorText(errs []error) string {
	var b strings.Builder
	for _, e := range errs {
		b.WriteString(e.Error())
		b.WriteString("\n")
	}
	return b.String()
}
