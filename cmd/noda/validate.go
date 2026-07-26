package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/server"
)

// validateProject runs every check `noda validate` performs after
// config.ValidateAll: plugin/service/node startup validation in dry-run mode
// (no database connections, no external calls) and middleware build
// validation.
//
// The validate command, the test command, and the runProjectTestSuites test
// helper all call this one function on purpose. When these checks lived only
// inside the validate command, `noda test` happily executed workflows that
// `noda validate` and boot both rejected — an unwired outcome output, an
// unknown node config field, a missing service slot — so a green test run did
// not imply the project could start (#444). Keeping a single implementation is
// what stops those surfaces from drifting apart again, the same way
// ValidateStartup and ValidateStartupDryRun drifted in #442.
func validateProject(rc *config.ResolvedConfig) error {
	// Plugin/service/node startup validation (dry-run: no database connections)
	plugins := registry.NewPluginRegistry()
	if err := registerCorePlugins(plugins); err != nil {
		return err
	}
	_, bootstrapErrs := registry.Bootstrap(context.Background(), rc, plugins, registry.BootstrapOptions{DryRun: true})
	if len(bootstrapErrs) > 0 {
		var errMsgs []string
		for _, e := range bootstrapErrs {
			errMsgs = append(errMsgs, e.Error())
		}
		return fmt.Errorf("bootstrap failed:\n  %s", strings.Join(errMsgs, "\n  "))
	}

	// Middleware factories validate config at build time (limiter max,
	// jwt secret, durations); building them here catches boot-time
	// failures that the schema and bootstrap dry-run can't see.
	if mwErrs := server.ValidateMiddlewareBuilds(rc); len(mwErrs) > 0 {
		var errMsgs []string
		for _, e := range mwErrs {
			errMsgs = append(errMsgs, e.Error())
		}
		return fmt.Errorf("middleware validation failed:\n  %s", strings.Join(errMsgs, "\n  "))
	}

	return nil
}
