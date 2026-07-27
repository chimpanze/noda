// Package startup runs the steps `noda start` performs that can fail from
// configuration alone, in one of two modes: live, where boot consumes the
// artifacts they produce, and dry-run, where every surface offering to check a
// project reads only their failures.
//
// It exists because those surfaces drifted four times. #442: ValidateStartup
// and ValidateStartupDryRun diverged. #444: the checks lived in the validate
// command, so `noda test` ran what validate rejected. #448: #444's fix landed
// in package main, unreachable by internal/mcp, so a third copy survived.
// #456: the editor and dev-mode reload were still on the dry-run bootstrap
// alone. Each fix built a better-placed *copy* of the boot sequence, and a
// copy drifts — #448's was already missing three phases the day it merged, so
// `noda validate` reported "all config files valid" for a project with a cycle
// in its workflow graph.
//
// The copy is what this package removes. cmd/noda takes its BootstrapResult
// and WorkflowCache from Artifacts rather than building them itself. An
// accidental deletion of the WorkflowCache assignment into Artifacts fails to
// compile (the local it would have assigned goes unused); a deliberate one,
// or a dropped Bootstrap assignment, still returns nil Artifacts fields with
// every phase reporting success — Run's own incomplete-artifacts check below
// turns that into an explicit failure instead of a nil that would otherwise
// reach cmd/noda's artifacts test only as a panic.
//
// A boot step belongs here if it can fail from configuration alone. Steps
// needing the network or filesystem at boot — dialing services, loading Wasm
// modules, binding the port, health checks — cannot be checked offline and
// stay in cmd/noda.
//
// Add a phase here, not at a call site, and every surface gains it.
package startup

import (
	"context"
	"errors"
	"fmt"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/scheduler"
	"github.com/chimpanze/noda/internal/server"
	"github.com/chimpanze/noda/internal/worker"
	"github.com/chimpanze/noda/pkg/api"
)

// Phase names a step of the startup sequence, in the order it runs.
type Phase string

const (
	PhaseRegistries Phase = "registries"
	PhaseWorkflows  Phase = "workflows"
	PhaseMiddleware Phase = "middleware"
	PhaseSchedules  Phase = "schedules"
	PhaseWorkers    Phase = "workers"
)

// Registries holds live registries a caller already has, so the editor and
// dev-mode reload can validate without rebuilding them.
type Registries struct {
	Plugins  *registry.PluginRegistry
	Nodes    *registry.NodeRegistry
	Compiler *expr.Compiler
}

// Input configures a startup run.
type Input struct {
	// RC is the resolved config. config.ValidateAll runs before this and is
	// not a phase: it is the one step that never drifted, and its
	// config.ValidationError carries a JSONPath the editor places markers
	// with, which flattening into Failure would discard.
	RC *config.ResolvedConfig

	// Plugins is the plugin set to register. It is supplied by the caller
	// rather than imported here so that this package does not depend on
	// plugins/all — which pulls in the bimg/libvips cgo image plugin, and
	// would put it in internal/editor's dependency graph.
	Plugins []api.Plugin

	// Live reuses the caller's registries instead of building fresh ones.
	// Set by the editor and dev-mode reload, which already hold them.
	//
	// On this path, Artifacts.Bootstrap.Services is nil: the live path
	// validates against registries the caller already holds and never
	// initializes services, unlike the BootstrapResult a fresh (Live == nil)
	// run produces. A caller consuming Artifacts on this path must not
	// dereference .Bootstrap.Services — ServiceRegistry's methods lock its
	// receiver's mutex unconditionally, so a nil *ServiceRegistry panics on
	// first use.
	Live *Registries

	// RootConfigPath is the absolute path of noda.json. Failures whose fault
	// is declared project-wide — a service config, a middleware definition —
	// are attributed to it, so callers filtering by file need no special case.
	RootConfigPath string

	// DryRun skips service creation, so validation never opens a connection.
	DryRun bool
}

// Failure is one startup failure, attributed to the files a caller should
// point a user at.
type Failure struct {
	// Phase is the step that produced this failure.
	Phase Phase
	// Files are absolute paths of the config files implicated. Empty when the
	// failure belongs to no particular file.
	Files []string
	// JSONPath locates the offending field within Files, when the phase knows
	// it. Usually empty — "cycle detected" has no single field.
	JSONPath string
	// Err is the failure, formatted as the CLI prints it.
	Err error
}

// Artifacts holds what the live phases built. Boot consumes these rather than
// constructing its own, which is what keeps the phases producing them from
// being dropped.
type Artifacts struct {
	Bootstrap     *registry.BootstrapResult
	WorkflowCache *engine.WorkflowCache
}

// Errors returns every failure's error, in phase order.
func Errors(failures []Failure) []error {
	out := make([]error, 0, len(failures))
	for _, f := range failures {
		out = append(out, f.Err)
	}
	return out
}

// OfPhase returns the errors from one phase. Callers print one headed message
// per phase, so the split is kept rather than flattened — flattening forces
// each caller to re-derive it, which is how they drift.
func OfPhase(failures []Failure, p Phase) []error {
	var out []error
	for _, f := range failures {
		if f.Phase == p {
			out = append(out, f.Err)
		}
	}
	return out
}

// Run executes the startup phases in order, stopping at the first that fails.
//
// It stops rather than collecting every phase's failures because that is what
// `noda validate` has always printed. Running on would spare a
// fix-then-revalidate round trip, but it is a change to CLI output and belongs
// to its own decision.
//
// The returned error-free case yields non-nil Artifacts. A phase failure
// yields whatever artifacts earlier phases produced, and nil for the rest.
func Run(ctx context.Context, in Input) (*Artifacts, []Failure) {
	arts := &Artifacts{}

	boot, failures := runRegistries(ctx, in)
	if len(failures) > 0 {
		return arts, failures
	}
	arts.Bootstrap = boot

	cache, failures := runWorkflows(in, boot)
	if len(failures) > 0 {
		return arts, failures
	}
	arts.WorkflowCache = cache

	if failures := runMiddleware(in); len(failures) > 0 {
		return arts, failures
	}
	if failures := runSchedules(in); len(failures) > 0 {
		return arts, failures
	}
	if failures := runWorkers(in); len(failures) > 0 {
		return arts, failures
	}

	if failures := checkArtifactsComplete(arts); len(failures) > 0 {
		return arts, failures
	}

	return arts, nil
}

// runMiddleware builds every middleware the routes, groups, presets, and
// connection endpoints reference, without connecting to Redis or performing
// OIDC discovery. Boot runs this too: server.Setup would otherwise fail with
// "register routes:" naming whichever route it reached first, where this
// names every affected route in a stable order (#450).
func runMiddleware(in Input) []Failure {
	var failures []Failure
	for _, err := range server.ValidateMiddlewareBuilds(in.RC) {
		f := Failure{Phase: PhaseMiddleware, Err: err}

		var mwErr *server.MiddlewareBuildError
		if errors.As(err, &mwErr) {
			f.Files = append(f.Files, mwErr.Files...)
			// A middleware's config lives in the root config, so that file is
			// implicated alongside every route referencing it — editing it is
			// where the fix goes.
			if in.RootConfigPath != "" {
				f.Files = append(f.Files, in.RootConfigPath)
			}
		}
		failures = append(failures, f)
	}
	return failures
}

// runSchedules checks that every cron spec is one the scheduler can register.
// Before this was a phase, a five-field spec — the form every non-Go cron
// accepts — passed `noda validate` and failed at lifecycle.StartAll, after
// services had been dialed.
func runSchedules(in Input) []Failure {
	var failures []Failure
	for _, err := range scheduler.ValidateSpecs(scheduler.ParseScheduleConfigs(in.RC.Schedules)) {
		f := Failure{Phase: PhaseSchedules, Err: err, JSONPath: "/cron"}
		if err.Config.SourceFile != "" {
			f.Files = []string{err.Config.SourceFile}
		}
		failures = append(failures, f)
	}
	return failures
}

// runWorkers checks worker configuration Start would reject — today only the
// concurrency bound, which the worker JSON schema does not express.
func runWorkers(in Input) []Failure {
	var failures []Failure
	for _, err := range worker.ValidateConfigs(worker.ParseWorkerConfigs(in.RC.Workers)) {
		f := Failure{Phase: PhaseWorkers, Err: err, JSONPath: "/concurrency"}
		if err.Config.SourceFile != "" {
			f.Files = []string{err.Config.SourceFile}
		}
		failures = append(failures, f)
	}
	return failures
}

// runRegistries registers plugins and nodes, initializes services (unless
// dry-run), and runs the node/service/expression startup validation.
func runRegistries(ctx context.Context, in Input) (*registry.BootstrapResult, []Failure) {
	if in.Live != nil {
		errs := registry.DryRun(in.RC, in.Live.Plugins, in.Live.Nodes, in.Live.Compiler)
		// Services is deliberately left nil here: this path validates against
		// registries the caller already holds and never creates services, so
		// there is nothing to populate it with. A caller reading Artifacts on
		// this path must not dereference .Bootstrap.Services — it will panic
		// (ServiceRegistry's methods lock the receiver's mutex unconditionally).
		boot := &registry.BootstrapResult{
			Plugins:  in.Live.Plugins,
			Nodes:    in.Live.Nodes,
			Compiler: in.Live.Compiler,
		}
		return boot, attributeRegistries(in, errs)
	}

	plugins := registry.NewPluginRegistry()
	for _, p := range in.Plugins {
		if err := plugins.Register(p); err != nil {
			// A plugin failing to register is a defect in Noda, not in the
			// project being checked, so it is reported as a failure of the
			// first phase rather than silently dropped.
			return nil, []Failure{{
				Phase: PhaseRegistries,
				Err:   fmt.Errorf("register plugin %q: %w", p.Name(), err),
			}}
		}
	}

	boot, errs := registry.Bootstrap(ctx, in.RC, plugins, registry.BootstrapOptions{DryRun: in.DryRun})
	return boot, attributeRegistries(in, errs)
}

// attributeRegistries points service-config failures at the root config,
// where services are declared, and every workflow-scoped registries-phase
// failure (node type, config schema, service slots, edge outputs,
// expressions) at the workflow file registry.WorkflowScopedError names.
func attributeRegistries(in Input, errs []error) []Failure {
	failures := make([]Failure, 0, len(errs))
	for _, err := range errs {
		f := Failure{Phase: PhaseRegistries, Err: err}
		var svcErr *registry.ServiceConfigError
		var wfErr *registry.WorkflowScopedError
		switch {
		case errors.As(err, &svcErr):
			if in.RootConfigPath != "" {
				f.Files = []string{in.RootConfigPath}
			}
		case errors.As(err, &wfErr):
			f.Files = []string{wfErr.File}
		}
		failures = append(failures, f)
	}
	return failures
}

// runWorkflows parses and compiles every workflow graph. Before this was a
// phase, a cycle, an edge to an unknown node, a retry block on a non-error
// edge, or a duplicate workflow id passed `noda validate` and killed boot.
func runWorkflows(in Input, boot *registry.BootstrapResult) (*engine.WorkflowCache, []Failure) {
	cache, err := engine.NewWorkflowCache(in.RC.Workflows, boot.Nodes)
	if err == nil {
		return cache, nil
	}

	f := Failure{Phase: PhaseWorkflows, Err: err}
	var compileErr *engine.WorkflowCompileError
	if errors.As(err, &compileErr) && compileErr.File != "" {
		f.Files = []string{compileErr.File}
	}
	return nil, []Failure{f}
}

// checkArtifactsComplete is Run's last check, after every phase has reported
// success. A nil Bootstrap or WorkflowCache at this point does not fail
// cleanly at boot: Server.Setup recompiles a nil WorkflowCache itself, and
// the scheduler/worker runtimes degrade gracefully on a nil one, so the
// process boots and serves ordinary traffic — it only panics later, lazily,
// the first time a Wasm-triggered workflow runs (buildWorkflowRunner) or
// dev-mode's next hot reload calls WorkflowCache.Invalidate. This turns that
// into an immediate, readable startup failure instead, and is what remains
// of the safety Task 10's "cannot be dropped without breaking the build"
// claim overstated: deleting the WorkflowCache assignment above fails to
// compile, but deleting the Bootstrap assignment does not — this check is
// what catches that one instead.
func checkArtifactsComplete(arts *Artifacts) []Failure {
	if arts.Bootstrap == nil || arts.WorkflowCache == nil {
		return []Failure{{
			Err: fmt.Errorf("internal/startup: incomplete artifacts — a phase returned success without populating its result"),
		}}
	}
	return nil
}
