package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/validate"
)

// validateProject runs the shared startup validation (internal/validate) and
// renders its result as the single error `noda validate` and `noda test`
// print.
//
// The formatting lives here rather than in internal/validate because it is a
// CLI concern: the MCP tool consumes the same Result and emits a structured
// JSON list from it instead. The checks themselves must not be duplicated —
// keeping one implementation is what stops these surfaces drifting, which they
// have now done twice (#444, #448).
func validateProject(rc *config.ResolvedConfig) error {
	res, err := validate.Project(context.Background(), rc)
	if err != nil {
		return err
	}
	if msgs := joinErrors(res.Bootstrap); msgs != "" {
		return fmt.Errorf("bootstrap failed:\n  %s", msgs)
	}
	if msgs := joinErrors(res.Middleware); msgs != "" {
		return fmt.Errorf("middleware validation failed:\n  %s", msgs)
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
