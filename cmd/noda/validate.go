package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/startup"
	"github.com/chimpanze/noda/plugins/all"
)

// validateProject runs the shared startup phases (internal/startup) and
// renders the failing one as the single error `noda validate` and `noda test`
// print.
//
// The formatting lives here rather than in internal/startup because it is a
// CLI concern: the MCP tool consumes the same failures and emits structured
// JSON, and the editor attributes them to files. The checks themselves must
// not be duplicated — one implementation is what stops these surfaces
// drifting, which they did four times (#442, #444, #448, #456).
func validateProject(rc *config.ResolvedConfig) error {
	_, failures := startup.Run(context.Background(), startup.Input{
		RC:      rc,
		Plugins: all.All(),
		DryRun:  true,
	})
	return renderStartupFailures(failures)
}

// renderStartupFailures renders a phase run's failures as one CLI error, or nil
// if there were none.
//
// It is separate from validateProject only so a test can hand it a failure
// whose phase no renderer knows — the case that must not be silently dropped
// and cannot be produced by running a real project.
func renderStartupFailures(failures []startup.Failure) error {
	if len(failures) == 0 {
		return nil
	}

	// Run stops at the first failing phase, so at most one of these fires.
	// startup.Phases() is iterated rather than a literal listed here: a literal
	// is how this command silently ignores a phase added later, which is the
	// drift the phase list exists to end.
	for _, phase := range startup.Phases() {
		if msgs := joinErrors(startup.OfPhase(failures, phase)); msgs != "" {
			return fmt.Errorf("%s validation failed:\n  %s", phase, msgs)
		}
	}

	// A failure whose phase is not in Phases() — startup.PhaseArtifacts, or a
	// phase added to the package but not to the list — must still be reported.
	// Returning nil here would make `noda validate` print "✓ All config files
	// valid" and exit 0 for a project that cannot boot, which is precisely the
	// bug the phase list exists to prevent; failing loudly under a generic
	// heading is always better than that.
	return fmt.Errorf("startup validation failed:\n  %s", joinErrors(startup.Errors(failures)))
}

// joinErrors renders errors one per line, indented to sit under the heading
// its caller prints. Returns "" for an empty slice so callers can test the
// result directly.
func joinErrors(errs []error) string {
	if len(errs) == 0 {
		return ""
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return strings.Join(msgs, "\n  ")
}
