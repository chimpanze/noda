// Package validate holds the startup validation that every surface offering
// to check a Noda project must run: the CLI's `noda validate` and `noda test`,
// and the MCP `noda_validate_config` tool.
//
// It exists because those surfaces drifted. The checks originally lived inside
// the validate command, so `noda test` executed workflows that `noda validate`
// and boot both rejected (#444); extracting them into a helper fixed that, but
// the helper lived in package main, so the MCP tool could not reuse it and
// still answered {"valid": true} for a project the CLI rejected (#448). The
// same shape of bug twice, for the same reason: one implementation per caller.
//
// Add a new check here, not at a call site, and every surface gains it.
package validate

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/server"
	"github.com/chimpanze/noda/plugins/all"
)

// Result holds startup-validation failures grouped by the phase that produced
// them. The phases are kept apart rather than flattened because callers report
// them differently: the CLI prints one headed message per phase, while the MCP
// tool emits a structured list. Flattening here would force one of them to
// re-derive the split.
type Result struct {
	// Bootstrap holds plugin, service, node, and expression failures from the
	// dry-run bootstrap.
	Bootstrap []error
	// Middleware holds failures from building the middleware a project's
	// routes, groups, presets, and connections reference.
	Middleware []error
}

// OK reports whether the project passed every check.
func (r Result) OK() bool { return len(r.Bootstrap) == 0 && len(r.Middleware) == 0 }

// Errors returns every failure, bootstrap phase first, for callers that do not
// distinguish the phases.
func (r Result) Errors() []error {
	out := make([]error, 0, len(r.Bootstrap)+len(r.Middleware))
	out = append(out, r.Bootstrap...)
	return append(out, r.Middleware...)
}

// Project runs every check `noda validate` performs after config.ValidateAll:
// plugin/service/node startup validation in dry-run mode, then middleware
// build validation.
//
// Neither phase opens a connection. The dry-run bootstrap skips service
// creation entirely, and the middleware factories are built without dialing
// Redis or performing OIDC discovery, so this is safe to run against a project
// whose backing services are not up.
//
// The returned error is non-nil only when validation could not be performed at
// all — a plugin failing to register, which is a defect in Noda rather than in
// the project being checked. A project that is merely invalid returns a
// non-OK Result and a nil error.
func Project(ctx context.Context, rc *config.ResolvedConfig) (Result, error) {
	plugins := registry.NewPluginRegistry()
	for _, p := range all.All() {
		if err := plugins.Register(p); err != nil {
			return Result{}, fmt.Errorf("register plugin %q: %w", p.Name(), err)
		}
	}

	var res Result
	if _, errs := registry.Bootstrap(ctx, rc, plugins, registry.BootstrapOptions{DryRun: true}); len(errs) > 0 {
		// Stop at the first failing phase, matching what `noda validate` has
		// always reported. Running both phases and returning their union would
		// spare a fix-then-revalidate round trip, but it is a behaviour change
		// to the CLI's output rather than part of unifying these call sites,
		// so it is deliberately not made here.
		res.Bootstrap = errs
		return res, nil
	}
	res.Middleware = server.ValidateMiddlewareBuilds(rc)

	return res, nil
}
