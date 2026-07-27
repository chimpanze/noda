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

	// Run stops at the first failing phase, so at most one of these fires.
	for _, phase := range []startup.Phase{
		startup.PhaseRegistries,
		startup.PhaseWorkflows,
		startup.PhaseMiddleware,
		startup.PhaseSchedules,
		startup.PhaseWorkers,
	} {
		if msgs := joinErrors(startup.OfPhase(failures, phase)); msgs != "" {
			return fmt.Errorf("%s validation failed:\n  %s", phase, msgs)
		}
	}
	return nil
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
