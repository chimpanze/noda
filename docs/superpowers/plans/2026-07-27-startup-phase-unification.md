# Startup Phase Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the validation phase list *be* the boot path, so `noda validate`, `noda test`, MCP, the editor, and dev-mode reload cannot accept a project that `noda start` refuses to boot.

**Architecture:** A new `internal/startup` package (superseding `internal/validate`) runs boot's config-derived steps as an ordered phase list in one of two modes. `cmd/noda`'s `initRuntime` takes its `BootstrapResult` and `WorkflowCache` *from* that list, so the load-bearing phases cannot be removed without breaking the build. Every validation surface calls the same `Run` with `DryRun: true`.

**Tech Stack:** Go, testify (`require`/`assert`), robfig/cron/v3, gofiber/fiber/v3.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-27-startup-phase-unification-design.md`. Read it before Task 1.
- `Error()` output of every new typed error must be **byte-identical** to today's text. #450's determinism tests and all CLI output depend on it.
- Iterate config maps in **sorted key order** anywhere errors are produced (#450). Never range a map directly into an error message.
- Every fixture-based guard must first assert the fixture's hidden fault **actually exists**, before asserting validation catches it. Three vacuous guards have shipped in this repo; `TestProject_BootstrapFailureSkipsMiddlewarePhase` passed with the code it guarded deleted.
- Pre-push gate, from the repo root, in this exact form:
  `gofmt -l . && go build ./... && go vet ./... && golangci-lint run --timeout=5m --build-tags=integration && go test ./...`
  `golangci-lint` is a **separate gate** from `go vet`, and the `--build-tags=integration` matters — without it, tagged files are unlinted.
- Work in the worktree `.worktrees/startup-phase-unification` on branch `feat/startup-phase-unification`. Confirm with `git rev-parse --show-toplevel` before any commit.
- Noda is pre-alpha with no live deployments. Behaviour changes land directly; no compatibility shims, no opt-out flags.

---

## File Structure

**Created:**
- `internal/startup/startup.go` — `Input`, `Artifacts`, `Failure`, `Phase`, `Run`. The phase list.
- `internal/startup/startup_test.go` — phase-level tests, ported from `internal/validate/validate_test.go`.
- `internal/engine/compile_error.go` — `WorkflowCompileError`.
- `internal/server/middleware_error.go` — `MiddlewareBuildError`.
- `testdata/bad-workflow-graph-project/` — passes ValidateAll + Registries, fails Workflows.
- `testdata/bad-schedule-project/` — passes ValidateAll + Registries, fails Schedules.
- `testdata/bad-worker-project/` — passes ValidateAll + Registries, fails Workers.

**Deleted:**
- `internal/validate/validate.go`, `internal/validate/validate_test.go` — absorbed by `internal/startup`.

**Modified:**
- `internal/engine/cache.go` — wrap failures in `WorkflowCompileError`.
- `internal/server/validate_middleware.go` — emit `MiddlewareBuildError`.
- `internal/scheduler/runtime.go` — extract `cronOptions()`, add `ValidateSpecs`, add `SourceFile`.
- `internal/worker/runtime.go` — extract `ValidateConfig`, add `SourceFile`.
- `cmd/noda/validate.go` — call `startup.Run`.
- `cmd/noda/runtime.go` — `initRuntime` consumes `Artifacts`.
- `cmd/noda/main.go:407-409` — dev-mode `SetDryRun` calls `startup.Run`.
- `internal/mcp/tools.go` — call `startup.Run`.
- `internal/editor/validation.go` — call `startup.Run`; delete both workarounds.
- `internal/editor/api.go` — `API` needs the services registry reference it already holds; no signature change.
- `testdata/valid-project/schedules/cleanup.json` — five-field cron → six.
- `CHANGELOG.md`.

---

## Task 1: `engine.WorkflowCompileError`

Lift the workflow's source file out of the formatted string so `startup` can attribute it.

**Files:**
- Create: `internal/engine/compile_error.go`
- Modify: `internal/engine/cache.go:19-48` (`buildGraphs`)
- Test: `internal/engine/compile_error_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `engine.WorkflowCompileError{File string, Err error}` with `Error() string` and `Unwrap() error`. `engine.NewWorkflowCache` now returns `*WorkflowCompileError` for every failure.

- [ ] **Step 1: Write the failing test**

`internal/engine/compile_error_test.go`:

```go
package engine

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cyclicWorkflows returns a workflow map whose graph contains a cycle, keyed
// the way config.ValidateAll keys rc.Workflows: by source file path.
func cyclicWorkflows() map[string]map[string]any {
	return map[string]map[string]any{
		"/proj/workflows/loop.json": {
			"id": "loop",
			"nodes": map[string]any{
				"a": map[string]any{"type": "util.log", "config": map[string]any{"level": "info", "message": "a"}},
				"b": map[string]any{"type": "util.log", "config": map[string]any{"level": "info", "message": "b"}},
			},
			"edges": []any{
				map[string]any{"from": "a", "to": "b"},
				map[string]any{"from": "b", "to": "a"},
			},
		},
	}
}

// The file key must be recoverable from the error without parsing its text.
// internal/startup uses it to tell the editor which file to mark.
func TestNewWorkflowCache_CompileFailureCarriesSourceFile(t *testing.T) {
	_, err := NewWorkflowCache(cyclicWorkflows(), testResolver{})
	require.Error(t, err)

	var compileErr *WorkflowCompileError
	require.ErrorAs(t, err, &compileErr)
	assert.Equal(t, "/proj/workflows/loop.json", compileErr.File)
}

// The text is load-bearing: it is what `noda start` prints today, and what
// the CLI's "compiling workflows:" wrapper reads. Adding the type must not
// change a single byte of it.
func TestWorkflowCompileError_TextIsUnchanged(t *testing.T) {
	_, err := NewWorkflowCache(cyclicWorkflows(), testResolver{})
	require.Error(t, err)

	assert.Equal(t,
		`compile workflow "/proj/workflows/loop.json": cycle detected: a → b → a`,
		err.Error())
}

func TestWorkflowCompileError_Unwraps(t *testing.T) {
	inner := errors.New("boom")
	e := &WorkflowCompileError{File: "f.json", Err: inner}
	assert.Same(t, inner, errors.Unwrap(e))
	assert.Equal(t, "boom", e.Error())
}
```

`testResolver` must resolve `util.log`. Check whether `internal/engine`'s existing tests already define a resolver stub — `grep -rn "NodeOutputResolver" internal/engine/*_test.go`. If one exists, use it and delete the `testResolver{}` references above in favour of that name. If none exists, add to the same file:

```go
// testResolver reports the outputs of the node types used in this file's
// fixtures. Compile only asks for outputs, so nothing else is needed.
type testResolver struct{}

func (testResolver) OutputsFor(nodeType string) ([]string, bool) {
	switch nodeType {
	case "util.log":
		return []string{"next"}, true
	default:
		return nil, false
	}
}
```

Confirm the interface's real method set first with `grep -n "type NodeOutputResolver" -A 8 internal/engine/*.go` and match it exactly.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/engine/ -run 'TestNewWorkflowCache_CompileFailureCarriesSourceFile|TestWorkflowCompileError' -v`
Expected: FAIL — `undefined: WorkflowCompileError`.

- [ ] **Step 3: Create the error type**

`internal/engine/compile_error.go`:

```go
package engine

// WorkflowCompileError reports a workflow that failed to parse or compile,
// carrying the source file that declared it.
//
// The file is a field rather than only a fragment of the message because
// internal/startup attributes failures to files so the editor can mark them,
// and parsing it back out of formatted text would break the moment the
// wording changed. Err is already fully formatted: Error() returns it
// verbatim, so adding this type changed no output.
type WorkflowCompileError struct {
	// File is the rc.Workflows map key that declared the workflow — an
	// absolute path to the source file.
	File string
	// Err is the underlying failure, pre-wrapped with its message.
	Err error
}

func (e *WorkflowCompileError) Error() string { return e.Err.Error() }

func (e *WorkflowCompileError) Unwrap() error { return e.Err }
```

- [ ] **Step 4: Wrap every failure in `buildGraphs`**

In `internal/engine/cache.go`, replace the body of `buildGraphs` (currently lines 19-48) with:

```go
func buildGraphs(workflows map[string]map[string]any, resolver NodeOutputResolver) (map[string]*CompiledGraph, error) {
	graphs := make(map[string]*CompiledGraph, len(workflows))
	source := make(map[string]string) // index key → file key that declared it
	put := func(key, fileKey string, g *CompiledGraph) error {
		if prev, ok := source[key]; ok {
			return &WorkflowCompileError{
				File: fileKey,
				Err:  fmt.Errorf("duplicate workflow id %q (declared by %q and %q)", key, prev, fileKey),
			}
		}
		source[key] = fileKey
		graphs[key] = g
		return nil
	}
	for _, id := range sortedWorkflowKeys(workflows) {
		raw := workflows[id]
		wfConfig, err := ParseWorkflowFromMap(id, raw)
		if err != nil {
			return nil, &WorkflowCompileError{File: id, Err: fmt.Errorf("parse workflow %q: %w", id, err)}
		}
		graph, err := Compile(wfConfig, resolver)
		if err != nil {
			return nil, &WorkflowCompileError{File: id, Err: fmt.Errorf("compile workflow %q: %w", id, err)}
		}
		if err := put(id, id, graph); err != nil {
			return nil, err
		}
		if jsonID, ok := raw["id"].(string); ok && jsonID != id {
			if err := put(jsonID, id, graph); err != nil {
				return nil, err
			}
		}
	}
	return graphs, nil
}

// sortedWorkflowKeys returns the workflow file keys in a stable order. Which
// of several broken workflows is reported first must not depend on Go's map
// iteration order (#450) — that made identical input produce different errors
// between runs.
func sortedWorkflowKeys(workflows map[string]map[string]any) []string {
	keys := make([]string, 0, len(workflows))
	for k := range workflows {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
```

Add `"sort"` to the import block in `internal/engine/cache.go`.

- [ ] **Step 5: Write the determinism test**

Append to `internal/engine/compile_error_test.go`:

```go
// Two broken workflows must always report the same one first. Ranging the map
// directly meant identical input produced different errors between runs (#450).
func TestNewWorkflowCache_ReportsBrokenWorkflowsDeterministically(t *testing.T) {
	workflows := cyclicWorkflows()
	workflows["/proj/workflows/aaa-loop.json"] = workflows["/proj/workflows/loop.json"]

	for range 20 {
		_, err := NewWorkflowCache(workflows, testResolver{})
		require.Error(t, err)

		var compileErr *WorkflowCompileError
		require.ErrorAs(t, err, &compileErr)
		assert.Equal(t, "/proj/workflows/aaa-loop.json", compileErr.File,
			"the lexically first broken workflow must always be the one reported")
	}
}
```

- [ ] **Step 6: Run the full engine suite**

Run: `go test ./internal/engine/ -count=1`
Expected: PASS. If an existing test asserts on a `parse workflow`/`compile workflow`/`duplicate workflow id` string, it must still pass untouched — that is the point of `Error()` returning `Err.Error()` verbatim. If one fails, the wrapping changed the text; fix the wrapping, not the test.

- [ ] **Step 7: Commit**

```bash
git add internal/engine/compile_error.go internal/engine/compile_error_test.go internal/engine/cache.go
git commit -m "feat(engine): carry the source file on workflow compile failures

internal/startup attributes failures to files so the editor can mark them.
Error() returns the wrapped error verbatim, so no output changed."
```

---

## Task 2: `server.MiddlewareBuildError`

Same lift for middleware. A middleware referenced by several route files must name all of them.

**Files:**
- Create: `internal/server/middleware_error.go`
- Modify: `internal/server/validate_middleware.go:37-110`
- Test: `internal/server/middleware_error_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `server.MiddlewareBuildError{Name string, Files []string, Err error}` with `Error()` and `Unwrap()`. `ValidateMiddlewareBuilds` returns these for build failures.

- [ ] **Step 1: Write the failing test**

`internal/server/middleware_error_test.go`:

```go
package server

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brokenLimiterConfig returns a project whose limiter has max=0 — accepted by
// file-level schema validation, rejected by the limiter factory at build time.
// Routes are keyed by source file path, matching how config.ValidateAll keys
// rc.Routes.
func brokenLimiterConfig(routeFiles ...string) *config.ResolvedConfig {
	routes := map[string]map[string]any{}
	for _, f := range routeFiles {
		routes[f] = map[string]any{
			"id":         "r",
			"method":     "GET",
			"path":       "/r",
			"middleware": []any{"limiter"},
		}
	}
	return &config.ResolvedConfig{
		Root: map[string]any{
			"middleware": map[string]any{
				"limiter": map[string]any{"max": float64(0)},
			},
		},
		Routes: routes,
	}
}

// Every file referencing the broken middleware must be recoverable, not just
// the first: the editor marks all of them.
func TestValidateMiddlewareBuilds_ErrorCarriesEveryReferencingFile(t *testing.T) {
	rc := brokenLimiterConfig("/proj/routes/a.json", "/proj/routes/b.json")

	errs := ValidateMiddlewareBuilds(rc)
	require.Len(t, errs, 1, "one failing middleware is reported once, naming every scope")

	var mwErr *MiddlewareBuildError
	require.ErrorAs(t, errs[0], &mwErr)
	assert.Equal(t, "limiter", mwErr.Name)
	assert.Equal(t, []string{"/proj/routes/a.json", "/proj/routes/b.json"}, mwErr.Files,
		"files must be in sorted order, matching the scope order in the message")
}

// global_middleware is declared project-wide, so it has no referencing file.
// internal/startup maps an empty Files to the root config.
func TestValidateMiddlewareBuilds_GlobalMiddlewareHasNoFile(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"global_middleware": []any{"limiter"},
			"middleware":        map[string]any{"limiter": map[string]any{"max": float64(0)}},
		},
	}

	errs := ValidateMiddlewareBuilds(rc)
	require.Len(t, errs, 1)

	var mwErr *MiddlewareBuildError
	require.ErrorAs(t, errs[0], &mwErr)
	assert.Empty(t, mwErr.Files)
}

// The message is what `noda validate` prints and what #450's tests pin.
func TestMiddlewareBuildError_TextIsUnchanged(t *testing.T) {
	rc := brokenLimiterConfig("/proj/routes/a.json")

	errs := ValidateMiddlewareBuilds(rc)
	require.Len(t, errs, 1)
	assert.Equal(t,
		`route "/proj/routes/a.json": middleware "limiter": limiter: max=0 is not allowed; set an explicit max request count`,
		errs[0].Error())
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/server/ -run TestValidateMiddlewareBuilds_ErrorCarries -v`
Expected: FAIL — `undefined: MiddlewareBuildError`.

If `TestMiddlewareBuildError_TextIsUnchanged` fails on the exact message string, copy the observed text into the assertion — the message must match what the code produces *today*, before any change. Verify by stashing the test and running it against `origin/main`.

- [ ] **Step 3: Create the error type**

`internal/server/middleware_error.go`:

```go
package server

// MiddlewareBuildError reports a middleware that fails to build, naming every
// config file whose routes or connection endpoints reference it.
//
// Files is a slice, not a single path, because one misconfigured middleware
// breaks every route that names it — the editor must mark all of them. It is
// empty for global_middleware, which is declared project-wide and belongs to
// no route file; internal/startup maps that case to the root config.
//
// Err is already fully formatted. Error() returns it verbatim so that lifting
// the file attribution out changed no output — #450's determinism tests pin
// this text.
type MiddlewareBuildError struct {
	// Name is the middleware as referenced, including any ":instance" suffix.
	Name string
	// Files are the absolute paths of the route and connection files
	// referencing it, in sorted order. Empty for global_middleware.
	Files []string
	// Err is the underlying failure, pre-wrapped with scopes and name.
	Err error
}

func (e *MiddlewareBuildError) Error() string { return e.Err.Error() }

func (e *MiddlewareBuildError) Unwrap() error { return e.Err }
```

- [ ] **Step 4: Emit the typed error**

In `internal/server/validate_middleware.go`, `ValidateMiddlewareBuilds` currently tracks `scopes map[string][]string` holding rendered scope strings. Add a parallel map of source files, and emit the typed error.

Change the `check` closure and the declarations above it to:

```go
	buildErr := map[string]error{}
	scopes := map[string][]string{}
	files := map[string][]string{}
	var failed []string

	// check records that `scope` references `name`. file is the source path of
	// the config declaring that scope, or "" for project-wide scopes such as
	// global_middleware.
	check := func(scope, file, name string) {
		err, built := buildErr[name]
		if !built {
			err = s.checkMiddlewareBuild(name)
			buildErr[name] = err
			if err != nil {
				failed = append(failed, name)
			}
		}
		if err == nil {
			return
		}
		if !slices.Contains(scopes[name], scope) {
			scopes[name] = append(scopes[name], scope)
		}
		if file != "" && !slices.Contains(files[name], file) {
			files[name] = append(files[name], file)
		}
	}
```

Update the four call sites to pass the file:

```go
	for _, name := range s.getGlobalMiddleware() {
		check("global_middleware", "", name)
	}
```

```go
		for _, name := range names {
			check(fmt.Sprintf("route %q", id), id, name)
		}
```

(`id` is the `s.config.Routes` key, which is the absolute source path.)

```go
			scope := fmt.Sprintf("connection %q endpoint %q", connID, epName)
```
...and within that loop:
```go
			for _, name := range names {
				check(scope, connID, name)
			}
```

(`connID` is the `s.config.Connections` key, likewise an absolute source path.)

Finally, replace the error-emitting loop at the end:

```go
	for _, name := range failed {
		errs = append(errs, &MiddlewareBuildError{
			Name:  name,
			Files: files[name],
			Err:   fmt.Errorf("%s: middleware %q: %w", joinScopes(scopes[name]), name, buildErr[name]),
		})
	}
```

`files[name]` is already sorted: routes and connections are visited via `sortedSectionKeys`, and `global_middleware` contributes no file.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/server/ -count=1`
Expected: PASS, including every pre-existing `ValidateMiddlewareBuilds` test from #450 — untouched.

- [ ] **Step 6: Commit**

```bash
git add internal/server/middleware_error.go internal/server/middleware_error_test.go internal/server/validate_middleware.go
git commit -m "feat(server): name every referencing file on middleware build failures

Files is a slice because one broken middleware breaks every route naming
it, and the editor marks all of them. Error() is unchanged."
```

---

## Task 3: Scheduler cron-spec validation

`worker`/`scheduler` config faults currently surface at `lifecycle.StartAll`, after services are dialed. Make the check shareable — and make it literally the same code path `Start` uses, so it cannot diverge.

**Files:**
- Modify: `internal/scheduler/runtime.go` (`Start` ~line 112-146, `ScheduleConfig` line 27-38, `ParseScheduleConfigs` line 425+)
- Test: `internal/scheduler/validate_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `scheduler.ScheduleConfig` gains `SourceFile string`.
  - `scheduler.ValidateSpecs(configs []ScheduleConfig) []error` — one error per schedule whose cron spec `Start` would reject.

- [ ] **Step 1: Write the failing test**

`internal/scheduler/validate_test.go`:

```go
package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The runtime installs cron.WithSeconds(), so a five-field spec — the form
// every non-Go cron accepts — is rejected. This is the shape that made
// testdata/valid-project unbootable while `noda validate` called it clean.
func TestValidateSpecs_RejectsFiveFieldSpec(t *testing.T) {
	errs := ValidateSpecs([]ScheduleConfig{
		{ID: "cleanup", Cron: "0 */6 * * *", SourceFile: "/proj/schedules/cleanup.json"},
	})

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "cleanup")
	assert.Contains(t, errs[0].Error(), "expected exactly 6 fields")
}

func TestValidateSpecs_AcceptsSixFieldSpec(t *testing.T) {
	assert.Empty(t, ValidateSpecs([]ScheduleConfig{
		{ID: "cleanup", Cron: "0 0 */6 * * *"},
	}))
}

func TestValidateSpecs_AcceptsDescriptor(t *testing.T) {
	assert.Empty(t, ValidateSpecs([]ScheduleConfig{
		{ID: "nightly", Cron: "@daily"},
	}))
}

// A timezone prefix is part of the spec Start registers, so it is part of what
// gets validated.
func TestValidateSpecs_ValidatesWithTimezonePrefix(t *testing.T) {
	assert.Empty(t, ValidateSpecs([]ScheduleConfig{
		{ID: "tz", Cron: "0 0 3 * * *", Timezone: "Europe/Berlin"},
	}))
}

// Attribution: internal/startup needs the declaring file, and
// ParseScheduleConfigs is the only place that still knows it.
func TestParseScheduleConfigs_RecordsSourceFile(t *testing.T) {
	configs := ParseScheduleConfigs(map[string]map[string]any{
		"/proj/schedules/cleanup.json": {"id": "cleanup", "cron": "0 0 * * * *"},
	})

	require.Len(t, configs, 1)
	assert.Equal(t, "/proj/schedules/cleanup.json", configs[0].SourceFile)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/scheduler/ -run 'TestValidateSpecs|TestParseScheduleConfigs_RecordsSourceFile' -v`
Expected: FAIL — `undefined: ValidateSpecs`, and `SourceFile` is not a field.

- [ ] **Step 3: Add `SourceFile` and extract `cronOptions`**

In `internal/scheduler/runtime.go`, add to `ScheduleConfig` (after `ID`):

```go
	// SourceFile is the rc.Schedules map key — the absolute path of the file
	// declaring this schedule. Carried so validation can attribute failures to
	// a file; ID comes from the JSON "id" field and does not identify one.
	SourceFile string
```

In `ParseScheduleConfigs`, set it in the struct literal:

```go
		sc := ScheduleConfig{
			ID:          engine.MapStrVal(raw, "id"),
			SourceFile:  k,
			Cron:        engine.MapStrVal(raw, "cron"),
			Timezone:    tz,
			Description: engine.MapStrVal(raw, "description"),
		}
```

Extract the option set so validation and registration cannot diverge. Add near `Start`:

```go
// cronOptions returns the option set the runtime installs. Start and
// ValidateSpecs both build their cron instance from it, so a spec that
// validates is one Start can register — a second, hand-rolled parser would be
// free to disagree with the real one.
func cronOptions() []cron.Option {
	return []cron.Option{cron.WithSeconds()}
}
```

In `Start`, replace `opts := []cron.Option{cron.WithSeconds()}` with:

```go
	opts := cronOptions()
```

- [ ] **Step 4: Add `ValidateSpecs`**

Append to `internal/scheduler/runtime.go`:

```go
// ValidateSpecs reports schedules whose cron expression Start would reject.
//
// It registers each spec against a cron instance built from cronOptions() —
// the same construction Start uses — rather than re-deriving the field layout.
// The instance is never started, so no goroutine or timer is created.
//
// This runs at validate time and at boot. Before it existed, an unparseable
// spec surfaced only from lifecycle.StartAll, after services had been dialed
// and the port bound, while `noda validate` reported the project clean.
func ValidateSpecs(configs []ScheduleConfig) []error {
	var errs []error
	for _, sc := range configs {
		spec := sc.Cron
		if sc.Timezone != "" {
			spec = "TZ=" + sc.Timezone + " " + spec
		}
		c := cron.New(cronOptions()...)
		if _, err := c.AddFunc(spec, func() {}); err != nil {
			errs = append(errs, fmt.Errorf("schedule %q: invalid cron spec %q: %w", sc.ID, sc.Cron, err))
		}
	}
	return errs
}
```

`ParseScheduleConfigs` already iterates sorted keys, so `configs` arrives in a stable order and no extra sorting is needed here.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/scheduler/ -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/scheduler/runtime.go internal/scheduler/validate_test.go
git commit -m "feat(scheduler): validate cron specs without starting the runtime

ValidateSpecs registers against a cron built from the same cronOptions()
Start uses, so a validated spec is one Start can register."
```

---

## Task 4: Worker config validation

**Files:**
- Modify: `internal/worker/runtime.go` (`Start` line 106-160, `WorkerConfig` line 25-37, `ParseWorkerConfigs` line 679+)
- Test: `internal/worker/validate_test.go` (create)

**Interfaces:**
- Consumes: nothing.
- Produces:
  - `worker.WorkerConfig` gains `SourceFile string`.
  - `worker.ValidateConfigs(configs []WorkerConfig) []error` — config-derived failures that would abort `Start`.

- [ ] **Step 1: Write the failing test**

`internal/worker/validate_test.go`:

```go
package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The worker schema declares "concurrency": {"type": "integer"} with no bound,
// so an over-limit value passes file validation and aborts Start.
func TestValidateConfigs_RejectsConcurrencyOverMaximum(t *testing.T) {
	errs := ValidateConfigs([]WorkerConfig{
		{ID: "ingest", Concurrency: maxConcurrency + 1, SourceFile: "/proj/workers/ingest.json"},
	})

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "ingest")
	assert.Contains(t, errs[0].Error(), "exceeds maximum")
}

func TestValidateConfigs_AcceptsConcurrencyAtMaximum(t *testing.T) {
	assert.Empty(t, ValidateConfigs([]WorkerConfig{
		{ID: "ingest", Concurrency: maxConcurrency},
	}))
}

// Start applies max(Concurrency, 1), so zero and negative values are legal
// config meaning "one consumer" — validation must not reject them.
func TestValidateConfigs_AcceptsZeroAndNegativeConcurrency(t *testing.T) {
	assert.Empty(t, ValidateConfigs([]WorkerConfig{
		{ID: "a", Concurrency: 0},
		{ID: "b", Concurrency: -1},
	}))
}

func TestParseWorkerConfigs_RecordsSourceFile(t *testing.T) {
	configs := ParseWorkerConfigs(map[string]map[string]any{
		"/proj/workers/ingest.json": {"id": "ingest"},
	})

	require.Len(t, configs, 1)
	assert.Equal(t, "/proj/workers/ingest.json", configs[0].SourceFile)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/ -run 'TestValidateConfigs|TestParseWorkerConfigs_RecordsSourceFile' -v`
Expected: FAIL — `undefined: ValidateConfigs`, and `SourceFile` is not a field.

- [ ] **Step 3: Add `SourceFile`**

In `internal/worker/runtime.go`, add to `WorkerConfig` (after `ID`):

```go
	// SourceFile is the rc.Workers map key — the absolute path of the file
	// declaring this worker. Carried so validation can attribute failures to a
	// file; ID comes from the JSON "id" field and does not identify one.
	SourceFile string
```

In `ParseWorkerConfigs`:

```go
		wc := WorkerConfig{
			ID:         engine.MapStrVal(raw, "id"),
			SourceFile: k,
		}
```

- [ ] **Step 4: Add `ValidateConfigs` and make `Start` use it**

Append to `internal/worker/runtime.go`:

```go
// ValidateConfigs reports worker configuration that would abort Start.
//
// Only the concurrency bound lives here. Everything else Start can reject —
// an unknown stream service, a service with the wrong plugin, a missing
// workflow, an unparseable timeout — is already caught by config.ValidateAll's
// cross-reference checks, and the remaining Start failures (service not
// reachable, consumer group creation) need a live Redis and cannot be checked
// offline.
//
// Start calls this too, so a worker that validates is one Start accepts.
func ValidateConfigs(configs []WorkerConfig) []error {
	var errs []error
	for _, w := range configs {
		if w.Concurrency > maxConcurrency {
			errs = append(errs, fmt.Errorf("worker %q: concurrency %d exceeds maximum %d", w.ID, w.Concurrency, maxConcurrency))
		}
	}
	return errs
}
```

In `Start`, replace the inline check (lines 116-119):

```go
	for _, w := range r.workers {
		concurrency := max(w.Concurrency, 1)
		if concurrency > maxConcurrency {
			return fmt.Errorf("worker %q: concurrency %d exceeds maximum %d", w.ID, concurrency, maxConcurrency)
		}
```

with a single up-front call before the loop:

```go
	if errs := ValidateConfigs(r.workers); len(errs) > 0 {
		return errs[0]
	}

	for _, w := range r.workers {
		concurrency := max(w.Concurrency, 1)
```

`ValidateConfigs` checks `w.Concurrency` directly rather than the `max(w.Concurrency, 1)` value. That is not a behaviour change: `max(w.Concurrency, 1)` only ever differs from `w.Concurrency` when the value is below 1, and such a value can never exceed `maxConcurrency`.

`ParseWorkerConfigs` iterates sorted keys, so `r.workers` is stably ordered and `errs[0]` is deterministic.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/worker/ -count=1`
Expected: PASS. An existing `Start` test asserting the concurrency error message must still pass — the text is unchanged.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/runtime.go internal/worker/validate_test.go
git commit -m "feat(worker): extract the config checks Start performs

Start now calls ValidateConfigs, so a worker that validates is one Start
accepts. Only the concurrency bound qualifies; crossrefs cover the rest."
```

---

## Task 5: `internal/startup` — types and the first two phases

**Files:**
- Create: `internal/startup/startup.go`, `internal/startup/startup_test.go`
- Test fixtures used: existing `testdata/valid-project`, `testdata/test-cmd-invalid-project`

**Interfaces:**
- Consumes: `engine.WorkflowCompileError` (Task 1), `registry.ServiceConfigError` (existing).
- Produces:

```go
type Phase string
const (
    PhaseRegistries Phase = "registries"
    PhaseWorkflows  Phase = "workflows"
    PhaseMiddleware Phase = "middleware"
    PhaseSchedules  Phase = "schedules"
    PhaseWorkers    Phase = "workers"
)

type Registries struct {
    Plugins  *registry.PluginRegistry
    Nodes    *registry.NodeRegistry
    Compiler *expr.Compiler
}

type Input struct {
    RC             *config.ResolvedConfig
    Plugins        []api.Plugin
    Live           *Registries
    RootConfigPath string
    DryRun         bool
}

type Failure struct {
    Phase    Phase
    Files    []string
    JSONPath string
    Err      error
}

type Artifacts struct {
    Bootstrap     *registry.BootstrapResult
    WorkflowCache *engine.WorkflowCache
}

func Run(ctx context.Context, in Input) (*Artifacts, []Failure)
func Errors(failures []Failure) []error
func OfPhase(failures []Failure, p Phase) []error
```

- [ ] **Step 1: Write the failing test**

`internal/startup/startup_test.go`:

```go
package startup

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/plugins/all"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolve loads a testdata project the way every caller of Run does:
// config.ValidateAll first, then Run on the result.
func resolve(t *testing.T, dir string) (*config.ResolvedConfig, string) {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("../../testdata", dir))
	require.NoError(t, err)

	sm, err := config.NewSecretsManager(abs, "")
	require.NoError(t, err)
	rc, errs := config.ValidateAll(abs, "", sm)
	require.Empty(t, errs, "fixture must pass file-level validation; Run only covers what comes after")
	return rc, filepath.Join(abs, "noda.json")
}

// dryRun runs the full phase list against a fixture with fresh registries,
// the way `noda validate`, `noda test`, and MCP do.
func dryRun(t *testing.T, dir string) []Failure {
	t.Helper()
	rc, rootPath := resolve(t, dir)
	_, failures := Run(context.Background(), Input{
		RC:             rc,
		Plugins:        all.All(),
		RootConfigPath: rootPath,
		DryRun:         true,
	})
	return failures
}

func TestRun_AcceptsValidProject(t *testing.T) {
	assert.Empty(t, dryRun(t, "valid-project"))
}

// Dry-run mode must produce the artifacts later phases and boot depend on.
func TestRun_ProducesArtifacts(t *testing.T) {
	rc, rootPath := resolve(t, "valid-project")

	arts, failures := Run(context.Background(), Input{
		RC: rc, Plugins: all.All(), RootConfigPath: rootPath, DryRun: true,
	})

	require.Empty(t, failures)
	require.NotNil(t, arts)
	assert.NotNil(t, arts.Bootstrap, "cmd/noda takes its registries from here")
	assert.NotNil(t, arts.WorkflowCache, "cmd/noda takes its workflow cache from here")
}

// testdata/test-cmd-invalid-project leaves db.create's "exists" outcome
// output unwired — a registries-phase fault.
func TestRun_ReportsRegistriesFailures(t *testing.T) {
	failures := dryRun(t, "test-cmd-invalid-project")

	require.NotEmpty(t, failures)
	assert.Equal(t, PhaseRegistries, failures[0].Phase)
	assert.Contains(t, errorText(failures), "outcome output")
}

// A failing phase stops the list, matching what `noda validate` has always
// printed. The fixture must fail two phases or this asserts nothing.
func TestRun_StopsAtFirstFailingPhase(t *testing.T) {
	rc, rootPath := resolve(t, "bad-both-phases-project")

	// Guard the guard: prove a middleware fault really is waiting behind the
	// registries one.
	require.NotEmpty(t, serverMiddlewareFaults(t, rc),
		"fixture must have a middleware failure for the short-circuit to hide")

	_, failures := Run(context.Background(), Input{
		RC: rc, Plugins: all.All(), RootConfigPath: rootPath, DryRun: true,
	})

	require.NotEmpty(t, failures)
	for _, f := range failures {
		assert.Equal(t, PhaseRegistries, f.Phase,
			"no phase after the first failing one may run")
	}
}

func TestOfPhase_SelectsOnePhase(t *testing.T) {
	failures := []Failure{
		{Phase: PhaseRegistries, Err: assertErr("a")},
		{Phase: PhaseMiddleware, Err: assertErr("b")},
	}

	assert.Len(t, OfPhase(failures, PhaseRegistries), 1)
	assert.Len(t, OfPhase(failures, PhaseMiddleware), 1)
	assert.Empty(t, OfPhase(failures, PhaseWorkflows))
	assert.Len(t, Errors(failures), 2)
}

func errorText(failures []Failure) string {
	var b strings.Builder
	for _, f := range failures {
		b.WriteString(f.Err.Error())
		b.WriteString("\n")
	}
	return b.String()
}
```

Add the two small helpers at the bottom of the same file:

```go
func assertErr(msg string) error { return errors.New(msg) }

// serverMiddlewareFaults calls the middleware phase's underlying check
// directly, so a test can prove a fixture has a fault the phase list would
// hide behind an earlier failure.
func serverMiddlewareFaults(t *testing.T, rc *config.ResolvedConfig) []error {
	t.Helper()
	return server.ValidateMiddlewareBuilds(rc)
}
```

...and add `"errors"` and `"github.com/chimpanze/noda/internal/server"` to the imports.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/startup/ -v`
Expected: FAIL — package does not compile, `undefined: Run`.

- [ ] **Step 3: Write the package**

`internal/startup/startup.go`:

```go
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
// and WorkflowCache from Artifacts rather than building them itself, so the
// phases producing them cannot be dropped without breaking the build.
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
	"fmt"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
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

	return arts, nil
}

// runRegistries registers plugins and nodes, initializes services (unless
// dry-run), and runs the node/service/expression startup validation.
func runRegistries(ctx context.Context, in Input) (*registry.BootstrapResult, []Failure) {
	if in.Live != nil {
		errs := registry.DryRun(in.RC, in.Live.Plugins, in.Live.Nodes, in.Live.Compiler)
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

// attributeRegistries points service-config failures at the root config, where
// services are declared. Other registries failures already name their workflow
// file in the message and are left unattributed until a typed error exists for
// them.
func attributeRegistries(in Input, errs []error) []Failure {
	failures := make([]Failure, 0, len(errs))
	for _, err := range errs {
		f := Failure{Phase: PhaseRegistries, Err: err}
		var svcErr *registry.ServiceConfigError
		if errors.As(err, &svcErr) && in.RootConfigPath != "" {
			f.Files = []string{in.RootConfigPath}
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
```

Add `"errors"` to the import block.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/startup/ -count=1 -v`
Expected: PASS. `TestRun_StopsAtFirstFailingPhase` currently passes trivially (no later phase exists yet) — Task 6 gives it teeth.

- [ ] **Step 5: Commit**

```bash
git add internal/startup/
git commit -m "feat(startup): phase list with registries and workflow compilation

The workflow phase is new to every surface: a cycle in a workflow graph
passed 'noda validate' clean and killed boot."
```

---

## Task 6: The remaining three phases

**Files:**
- Modify: `internal/startup/startup.go`, `internal/startup/startup_test.go`
- Modify: `testdata/valid-project/schedules/cleanup.json`

**Interfaces:**
- Consumes: `server.MiddlewareBuildError` (Task 2), `scheduler.ValidateSpecs` (Task 3), `worker.ValidateConfigs` (Task 4).
- Produces: `Run` now executes all five phases.

- [ ] **Step 1: Fix the unbootable fixture**

`testdata/valid-project/schedules/cleanup.json` declares `"cron": "0 */6 * * *"` — five fields, where the runtime installs `cron.WithSeconds()` and needs six. The fixture named `valid-project` describes a project whose scheduler cannot start.

Change that line to:

```json
  "cron": "0 0 */6 * * *",
```

Same schedule — every six hours on the hour — now expressed with the seconds field.

- [ ] **Step 2: Write the failing tests**

Create the three fixtures. Each must pass `config.ValidateAll` and the registries phase, and fail exactly one later phase.

`testdata/bad-workflow-graph-project/noda.json`:
```json
{
  "services": {}
}
```

`testdata/bad-workflow-graph-project/routes/hello.json`:
```json
{
  "id": "hello",
  "method": "GET",
  "path": "/hello",
  "trigger": {
    "workflow": "hello"
  }
}
```

`testdata/bad-workflow-graph-project/workflows/hello.json`:
```json
{
  "id": "hello",
  "nodes": {
    "a": { "type": "util.log", "config": { "level": "info", "message": "a" } },
    "b": { "type": "util.log", "config": { "level": "info", "message": "b" } },
    "respond": { "type": "response.json", "config": { "status": 200, "body": {} } }
  },
  "edges": [
    { "from": "a", "to": "b" },
    { "from": "b", "to": "a" },
    { "from": "b", "to": "respond" }
  ]
}
```

`testdata/bad-schedule-project/noda.json`:
```json
{
  "services": {}
}
```

`testdata/bad-schedule-project/workflows/cleanup.json`:
```json
{
  "id": "cleanup",
  "nodes": {
    "log": { "type": "util.log", "config": { "level": "info", "message": "cleanup" } }
  },
  "edges": []
}
```

`testdata/bad-schedule-project/schedules/cleanup.json`:
```json
{
  "id": "cleanup",
  "cron": "0 */6 * * *",
  "trigger": {
    "workflow": "cleanup"
  }
}
```

`testdata/bad-worker-project/noda.json`:
```json
{
  "services": {}
}
```

`testdata/bad-worker-project/workflows/ingest.json`:
```json
{
  "id": "ingest",
  "nodes": {
    "log": { "type": "util.log", "config": { "level": "info", "message": "ingest" } }
  },
  "edges": []
}
```

`testdata/bad-worker-project/workers/ingest.json`:
```json
{
  "id": "ingest",
  "concurrency": 5000,
  "subscribe": {
    "topic": "events",
    "group": "ingest"
  },
  "trigger": {
    "workflow": "ingest"
  }
}
```

The worker fixture deliberately omits `services.stream`: naming a service that does not exist would fail `config.ValidateAll` crossrefs and the fixture would never reach the workers phase. Verify the omission is legal against `internal/config/schemas/` — if `services` is required by the worker schema, add a `stream` service to `noda.json` and reference it, so the fixture still fails *only* the workers phase.

Append to `internal/startup/startup_test.go`:

```go
// The workflow phase. A cycle passes config.ValidateAll and the registries
// phase, and killed boot at engine.NewWorkflowCache.
func TestRun_ReportsWorkflowGraphFailures(t *testing.T) {
	failures := dryRun(t, "bad-workflow-graph-project")

	require.Len(t, failures, 1)
	assert.Equal(t, PhaseWorkflows, failures[0].Phase)
	assert.Contains(t, failures[0].Err.Error(), "cycle detected")
	assert.Equal(t,
		[]string{filepath.Join(fixtureDir(t, "bad-workflow-graph-project"), "workflows", "hello.json")},
		failures[0].Files,
		"the editor marks the workflow file this cycle is in")
}

// The middleware phase — issue #456's subject, and #448's before it.
func TestRun_ReportsMiddlewareFailures(t *testing.T) {
	failures := dryRun(t, "bad-middleware-project")

	require.Len(t, failures, 1)
	assert.Equal(t, PhaseMiddleware, failures[0].Phase)
	assert.Contains(t, failures[0].Err.Error(), "limiter")

	dir := fixtureDir(t, "bad-middleware-project")
	assert.ElementsMatch(t,
		[]string{
			filepath.Join(dir, "routes", "hello.json"),
			filepath.Join(dir, "noda.json"),
		},
		failures[0].Files,
		"both the route referencing the middleware and the file defining it")
}

// The schedules phase. A five-field cron spec passed `noda validate` and
// failed at lifecycle.StartAll, after services were dialed.
func TestRun_ReportsScheduleFailures(t *testing.T) {
	failures := dryRun(t, "bad-schedule-project")

	require.Len(t, failures, 1)
	assert.Equal(t, PhaseSchedules, failures[0].Phase)
	assert.Contains(t, failures[0].Err.Error(), "expected exactly 6 fields")
	assert.Equal(t,
		[]string{filepath.Join(fixtureDir(t, "bad-schedule-project"), "schedules", "cleanup.json")},
		failures[0].Files)
}

// The workers phase. The worker schema puts no bound on concurrency.
func TestRun_ReportsWorkerFailures(t *testing.T) {
	failures := dryRun(t, "bad-worker-project")

	require.Len(t, failures, 1)
	assert.Equal(t, PhaseWorkers, failures[0].Phase)
	assert.Contains(t, failures[0].Err.Error(), "exceeds maximum")
	assert.Equal(t, "/concurrency", failures[0].JSONPath)
	assert.Equal(t,
		[]string{filepath.Join(fixtureDir(t, "bad-worker-project"), "workers", "ingest.json")},
		failures[0].Files)
}

// Guard the guards. Each fixture must fail ONLY its own phase — otherwise the
// tests above prove nothing about the phase they name. Three vacuous guards
// have shipped in this repo; this is what catches the fourth.
func TestFixtures_FailExactlyOnePhaseEach(t *testing.T) {
	for fixture, want := range map[string]Phase{
		"bad-workflow-graph-project": PhaseWorkflows,
		"bad-middleware-project":     PhaseMiddleware,
		"bad-schedule-project":       PhaseSchedules,
		"bad-worker-project":         PhaseWorkers,
	} {
		t.Run(fixture, func(t *testing.T) {
			failures := dryRun(t, fixture)
			require.NotEmpty(t, failures, "fixture must fail its phase")
			for _, f := range failures {
				assert.Equal(t, want, f.Phase,
					"fixture must fail only %s, so a guard naming it proves that phase ran", want)
			}
		})
	}
}

func fixtureDir(t *testing.T, dir string) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("../../testdata", dir))
	require.NoError(t, err)
	return abs
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/startup/ -run 'TestRun_Reports|TestFixtures_' -v`
Expected: FAIL — the middleware, schedules, and workers phases do not run, so `failures` is empty for those three fixtures.

- [ ] **Step 4: Make schedule and worker failures carry their config**

The phases need to attribute a failure to the file that declared it, and `ValidateSpecs`/`ValidateConfigs` as written in Tasks 3 and 4 return bare errors with no link back to the config that produced them. Matching on the ID inside the message would work today and break the moment the wording changes, so instead both return the config alongside the error.

In `internal/scheduler/runtime.go`, replace `ValidateSpecs` with:

```go
// SpecError reports a schedule whose cron expression Start would reject,
// carrying the schedule it came from so callers can attribute it to a file.
type SpecError struct {
	Config ScheduleConfig
	Err    error
}

func (e *SpecError) Error() string { return e.Err.Error() }

func (e *SpecError) Unwrap() error { return e.Err }

// ValidateSpecs reports schedules whose cron expression Start would reject.
//
// It registers each spec against a cron instance built from cronOptions() —
// the same construction Start uses — rather than re-deriving the field layout.
// The instance is never started, so no goroutine or timer is created.
//
// This runs at validate time and at boot. Before it existed, an unparseable
// spec surfaced only from lifecycle.StartAll, after services had been dialed
// and the port bound, while `noda validate` reported the project clean.
func ValidateSpecs(configs []ScheduleConfig) []*SpecError {
	var errs []*SpecError
	for _, sc := range configs {
		spec := sc.Cron
		if sc.Timezone != "" {
			spec = "TZ=" + sc.Timezone + " " + spec
		}
		c := cron.New(cronOptions()...)
		if _, err := c.AddFunc(spec, func() {}); err != nil {
			errs = append(errs, &SpecError{
				Config: sc,
				Err:    fmt.Errorf("schedule %q: invalid cron spec %q: %w", sc.ID, sc.Cron, err),
			})
		}
	}
	return errs
}
```

Update `internal/scheduler/validate_test.go`'s assertions from `errs[0].Error()` — they still compile, since `*SpecError` has `Error()`. Add one assertion to `TestValidateSpecs_RejectsFiveFieldSpec`:

```go
	assert.Equal(t, "/proj/schedules/cleanup.json", errs[0].Config.SourceFile)
```

Apply the identical shape in `internal/worker/runtime.go`:

```go
// ConfigError reports worker configuration Start would reject, carrying the
// worker it came from so callers can attribute it to a file.
type ConfigError struct {
	Config WorkerConfig
	Err    error
}

func (e *ConfigError) Error() string { return e.Err.Error() }

func (e *ConfigError) Unwrap() error { return e.Err }
```

...with `ValidateConfigs` returning `[]*ConfigError` and `Start` doing:

```go
	if errs := ValidateConfigs(r.workers); len(errs) > 0 {
		return errs[0]
	}
```

Add to `internal/worker/validate_test.go`'s `TestValidateConfigs_RejectsConcurrencyOverMaximum`:

```go
	assert.Equal(t, "/proj/workers/ingest.json", errs[0].Config.SourceFile)
```

- [ ] **Step 5: Add the three phases**

In `internal/startup/startup.go`, extend `Run` after the workflows phase:

```go
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
```

Add `"github.com/chimpanze/noda/internal/scheduler"`, `"github.com/chimpanze/noda/internal/server"`, and `"github.com/chimpanze/noda/internal/worker"` to `internal/startup/startup.go`'s imports.

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/startup/ ./internal/scheduler/ ./internal/worker/ -count=1`
Expected: PASS.

- [ ] **Step 7: Verify the short-circuit guard now has teeth**

Delete the `return arts, failures` line after `runRegistries`, run `go test ./internal/startup/ -run TestRun_StopsAtFirstFailingPhase`, and confirm it FAILS. Restore the line and confirm it passes. A guard that passes with the code it guards deleted is worth nothing, and this repo has shipped three of them.

- [ ] **Step 8: Commit**

```bash
git add internal/startup/ internal/scheduler/ internal/worker/ testdata/
git commit -m "feat(startup): add the middleware, schedules, and workers phases

Schedules and workers are new to every surface. Also fixes
testdata/valid-project, whose five-field cron spec meant the fixture named
'valid-project' described a project whose scheduler could not start."
```

---

## Task 7: Migrate the CLI and delete `internal/validate`

**Files:**
- Modify: `cmd/noda/validate.go`
- Delete: `internal/validate/validate.go`, `internal/validate/validate_test.go`
- Test: `cmd/noda/validate_test.go` (check for an existing file first)

**Interfaces:**
- Consumes: `startup.Run`, `startup.OfPhase` (Task 5/6).
- Produces: `validateProject(rc *config.ResolvedConfig) error` — unchanged signature, so `noda test`'s call site needs no edit.

- [ ] **Step 1: Write the failing test**

Find the existing test file with `ls cmd/noda/validate*_test.go`. Append to it, or create `cmd/noda/validate_test.go` with a matching package clause (`package main`):

```go
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/noda/ -run TestValidateProject_ReportsEveryPhase -v`
Expected: FAIL — the three new fixtures produce no error, and the heading for the first is `bootstrap failed`, not `registries validation failed`.

- [ ] **Step 3: Rewrite `validateProject`**

Replace `cmd/noda/validate.go`'s `validateProject` with:

```go
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
```

Replace the `internal/validate` import with `"github.com/chimpanze/noda/internal/startup"` and `"github.com/chimpanze/noda/plugins/all"`.

`RootConfigPath` is deliberately left empty here: the CLI prints messages, it does not mark files, and every message already names its file.

- [ ] **Step 4: Run the tests**

Run: `go test ./cmd/noda/ -count=1`
Expected: PASS. Any pre-existing test asserting the old `bootstrap failed:` heading now needs `registries validation failed:` — update it, since the heading genuinely changed.

- [ ] **Step 5: Commit**

`internal/validate` is left in place for now — `internal/mcp` still imports it, and deleting it here would leave the tree unbuildable. Task 8 migrates that last caller and deletes the package in the same commit.

```bash
git add cmd/noda/
git commit -m "refactor(cli): run the startup phase list from noda validate and noda test

Three phases are new to both commands: workflow compilation, cron specs,
and the worker concurrency bound."
```

---

## Task 8: Migrate the MCP tool

**Files:**
- Modify: `internal/mcp/tools.go:793` and its imports
- Test: `internal/mcp/tools_test.go` (append)

**Interfaces:**
- Consumes: `startup.Run`, `startup.Errors`.
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

Find the existing `noda_validate_config` test with `grep -rn "validate_config" internal/mcp/*_test.go` and follow its setup exactly. Append a table covering every phase:

```go
// MCP must not answer {"valid": true} for a project the CLI rejects — the bug
// #448 was filed for. Every phase must reach this tool.
func TestValidateConfigHandler_ReportsEveryPhase(t *testing.T) {
	for _, fixture := range []string{
		"test-cmd-invalid-project",
		"bad-workflow-graph-project",
		"bad-middleware-project",
		"bad-schedule-project",
		"bad-worker-project",
	} {
		t.Run(fixture, func(t *testing.T) {
			abs, err := filepath.Abs(filepath.Join("../../testdata", fixture))
			require.NoError(t, err)

			result := callValidateConfig(t, abs)

			assert.False(t, result["valid"].(bool),
				"a project that fails a startup phase is not valid")
			assert.NotEmpty(t, result["errors"])
		})
	}
}
```

`callValidateConfig` must match the existing tests' helper — reuse it if one exists, otherwise extract one from the existing test and use the same name.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/mcp/ -run TestValidateConfigHandler_ReportsEveryPhase -v`
Expected: FAIL to compile (`internal/validate` was deleted in Task 7), then FAIL on the three new fixtures once the import is fixed.

- [ ] **Step 3: Switch the call**

In `internal/mcp/tools.go`, replace the `validate.Project` block:

```go
	// File-level validation is clean — run the same startup phases `noda
	// validate`, `noda test`, the editor, and boot run, through the same
	// shared list, so this tool cannot answer "valid" for a project the CLI
	// rejects. It did exactly that before #448: it ran only the dry-run
	// bootstrap and skipped the middleware builds, so a route wired to a
	// misconfigured limiter validated clean here and failed at boot.
	_, failures := startup.Run(ctx, startup.Input{
		RC:             rc,
		Plugins:        all.All(),
		RootConfigPath: filepath.Join(path, "noda.json"),
		DryRun:         true,
	})
	if len(failures) > 0 {
		errList := make([]map[string]any, len(failures))
		for i, f := range failures {
			file := ""
			if len(f.Files) > 0 {
				file = f.Files[0]
			}
			errList[i] = map[string]any{
				"error":   f.Err.Error(),
				"phase":   string(f.Phase),
				"file":    file,
				"pointer": f.JSONPath,
				"message": f.Err.Error(),
			}
		}
		return jsonResult(map[string]any{
			"valid":  false,
			"errors": errList,
		})
	}
```

Replace the `internal/validate` import with `internal/startup` and `plugins/all`. Confirm the variable holding the project directory is named `path` at this point in the handler — if not, use whatever it is called.

- [ ] **Step 4: Delete the superseded package**

`internal/mcp` was the last caller. With it migrated, `internal/validate` has none:

```bash
grep -rn "internal/validate" --include=*.go . && echo "STILL REFERENCED — migrate the caller above first"
git rm -r internal/validate
```

The `grep` must print nothing before the `git rm`. Its tests are not lost — Task 5 ported them to `internal/startup/startup_test.go`, where they cover the same behaviour plus three more phases.

- [ ] **Step 5: Run the tests**

Run: `go build ./... && go test ./internal/mcp/ ./cmd/noda/ ./internal/startup/ -count=1`
Expected: PASS, and the build is green again — Task 7 left `internal/validate` in place precisely so it never was not.

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/ internal/validate
git commit -m "refactor(mcp): run the startup phase list from validate_config

Failures now carry their phase and source file, so the tool reports which
step rejected the project. internal/validate had no callers left and is
deleted here; internal/startup supersedes it."
```

---

## Task 9: Migrate the editor and delete both workarounds

**Files:**
- Modify: `internal/editor/validation.go`
- Test: `internal/editor/validation_test.go` (append)

**Interfaces:**
- Consumes: `startup.Run` with `Live` set.
- Produces: nothing new.

- [ ] **Step 1: Write the failing test**

Append to `internal/editor/validation_test.go`. Follow `setupValidationApp`'s existing pattern; the plugin registry it builds must additionally register whatever plugins the fixtures below need — check `setupValidationApp`'s body and extend its registry rather than writing a new helper.

```go
// A route wired to a misconfigured limiter showed green in the editor and
// failed at boot — issue #456.
func limiterProjectFiles() map[string]string {
	return map[string]string{
		"noda.json": `{"middleware": {"limiter": {"max": 0}}}`,
		"routes/hello.json": `{
  "id": "hello",
  "method": "GET",
  "path": "/hello",
  "trigger": { "workflow": "hello" },
  "middleware": ["limiter"]
}`,
		"workflows/hello.json": `{
  "id": "hello",
  "nodes": {
    "respond": { "type": "response.json", "config": { "status": 200, "body": {} } }
  },
  "edges": []
}`,
	}
}

func TestValidateAll_ReportsMiddlewareFailures(t *testing.T) {
	app := setupValidationApp(t, limiterProjectFiles())

	body := postJSON(t, app, "/_noda/validate", nil)

	assert.False(t, body["valid"].(bool), "a project that cannot boot is not valid")
	assert.Contains(t, marshalJSON(t, body["errors"]), "limiter")
}

// The route file references the broken middleware, so saving it must surface
// the error there.
func TestValidateFile_AttributesMiddlewareErrorToReferencingRoute(t *testing.T) {
	app := setupValidationApp(t, limiterProjectFiles())

	body := postJSON(t, app, "/_noda/validate-file", map[string]any{"path": "routes/hello.json"})

	assert.False(t, body["valid"].(bool))
	assert.Contains(t, marshalJSON(t, body["errors"]), "limiter")
}

// The middleware's config lives in noda.json, so saving that file must surface
// it too — that is where the fix goes.
func TestValidateFile_AttributesMiddlewareErrorToRootConfig(t *testing.T) {
	app := setupValidationApp(t, limiterProjectFiles())

	body := postJSON(t, app, "/_noda/validate-file", map[string]any{"path": "noda.json"})

	assert.False(t, body["valid"].(bool))
	assert.Contains(t, marshalJSON(t, body["errors"]), "limiter")
}

// A file with no connection to the fault must stay clean, or every marker in
// the editor becomes noise.
func TestValidateFile_DoesNotAttributeUnrelatedFile(t *testing.T) {
	app := setupValidationApp(t, limiterProjectFiles())

	body := postJSON(t, app, "/_noda/validate-file", map[string]any{"path": "workflows/hello.json"})

	assert.True(t, body["valid"].(bool),
		"the workflow neither references nor defines the broken middleware")
}

// The workflow phase, also invisible to the editor before this change.
func TestValidateAll_ReportsWorkflowGraphFailures(t *testing.T) {
	app := setupValidationApp(t, map[string]string{
		"noda.json": `{}`,
		"routes/hello.json": `{
  "id": "hello", "method": "GET", "path": "/hello",
  "trigger": { "workflow": "hello" }
}`,
		"workflows/hello.json": `{
  "id": "hello",
  "nodes": {
    "a": { "type": "util.log", "config": { "level": "info", "message": "a" } },
    "b": { "type": "util.log", "config": { "level": "info", "message": "b" } }
  },
  "edges": [
    { "from": "a", "to": "b" },
    { "from": "b", "to": "a" }
  ]
}`,
	})

	body := postJSON(t, app, "/_noda/validate", nil)

	assert.False(t, body["valid"].(bool))
	assert.Contains(t, marshalJSON(t, body["errors"]), "cycle detected")
}
```

`postJSON` and `marshalJSON` may not exist. Check the existing tests' request helpers with `grep -n "httptest.NewRequest" internal/editor/validation_test.go` and reuse whatever they use; if each test builds its request inline, extract these two helpers once and use them throughout the new tests:

```go
// postJSON sends a JSON body to an editor endpoint and decodes the response.
func postJSON(t *testing.T, app *fiber.App, path string, payload any) map[string]any {
	t.Helper()
	var body io.Reader = strings.NewReader("{}")
	if payload != nil {
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		body = strings.NewReader(string(raw))
	}
	req := httptest.NewRequest("POST", path, body)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	return decoded
}

func marshalJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/editor/ -run 'TestValidateAll_Reports|TestValidateFile_Attributes' -v`
Expected: FAIL — the editor runs `registry.DryRun` alone, so the middleware and workflow faults are invisible and every response says `valid: true`.

- [ ] **Step 3: Rewrite `internal/editor/validation.go`**

Replace `startupDryRunErrors`, `validateFile`, and `validateAll` with:

```go
// startupFailures runs the same startup phases boot and `noda validate` run,
// reusing the editor's live registries and without opening any connection.
//
// It returns nil when the registries needed for it are absent — dev-mode
// always has them, but tests may construct a bare instance.
func (e *API) startupFailures(rc *config.ResolvedConfig) []startup.Failure {
	if rc == nil || e.plugins == nil || e.nodes == nil || e.compiler == nil {
		return nil
	}
	_, failures := startup.Run(context.Background(), startup.Input{
		RC: rc,
		Live: &startup.Registries{
			Plugins:  e.plugins,
			Nodes:    e.nodes,
			Compiler: e.compiler,
		},
		RootConfigPath: e.root.Join("noda.json"),
		DryRun:         true,
	})
	return failures
}

// validateFile validates a single JSON config against its schema, then reports
// the startup failures implicating that file.
func (e *API) validateFile(c fiber.Ctx) error {
	var req struct {
		Path string `json:"path"`
	}
	if err := c.Bind().JSON(&req); err != nil {
		return c.Status(400).JSON(map[string]any{"error": "invalid request body"})
	}

	sm, smErr := config.NewSecretsManager(e.root.String(), e.envFlag)
	if smErr != nil {
		return c.Status(500).JSON(map[string]any{"error": smErr.Error()})
	}
	rc, errs := config.ValidateAll(e.root.String(), e.envFlag, sm)

	absPath, err := e.root.Resolve(req.Path)
	if err != nil {
		return c.Status(403).JSON(map[string]any{"error": "invalid path"})
	}

	var filtered []map[string]any
	for _, ve := range errs {
		if ve.FilePath == absPath {
			filtered = append(filtered, map[string]any{
				"file":    ve.FilePath,
				"path":    ve.JSONPath,
				"message": ve.Message,
			})
		}
	}

	if len(errs) == 0 {
		// Every startup failure names the files it implicates, so scoping is
		// one containment check. This replaces two workarounds that existed
		// only because the failures were untyped: a special case for
		// registry.ServiceConfigError, which has no file and belongs to the
		// root config, and a trick that pre-trimmed rc.Workflows to this file
		// so other files' errors would not appear (#349). The trick *hid*
		// cross-file failures rather than attributing them, so a file could
		// read clean while the project could not boot.
		for _, f := range e.startupFailures(rc) {
			if !slices.Contains(f.Files, absPath) {
				continue
			}
			filtered = append(filtered, map[string]any{
				"file":    absPath,
				"path":    f.JSONPath,
				"message": f.Err.Error(),
			})
		}
	}

	return c.JSON(map[string]any{
		"valid":  len(filtered) == 0,
		"errors": filtered,
	})
}

// validateAll runs the full validation pipeline and returns all errors.
func (e *API) validateAll(c fiber.Ctx) error {
	sm, smErr := config.NewSecretsManager(e.root.String(), e.envFlag)
	if smErr != nil {
		return c.Status(500).JSON(map[string]any{"error": smErr.Error()})
	}
	rc, errs := config.ValidateAll(e.root.String(), e.envFlag, sm)

	var out []map[string]any
	for _, ve := range errs {
		out = append(out, map[string]any{
			"file":    e.root.Rel(ve.FilePath),
			"path":    ve.JSONPath,
			"message": ve.Message,
		})
	}

	if len(errs) == 0 {
		for _, f := range e.startupFailures(rc) {
			file := ""
			if len(f.Files) > 0 {
				file = e.root.Rel(f.Files[0])
			}
			out = append(out, map[string]any{
				"file":    file,
				"path":    f.JSONPath,
				"message": f.Err.Error(),
			})
		}
	}

	return c.JSON(map[string]any{
		"valid":  len(out) == 0,
		"errors": out,
	})
}
```

Update the imports: drop `"errors"` and `"github.com/chimpanze/noda/internal/registry"` if now unused, add `"context"`, `"slices"`, and `"github.com/chimpanze/noda/internal/startup"`.

Note the renamed local in `validateAll` (`errors` → `out`): the old code shadowed the `errors` package with a variable, which is why `errors.As` had to be called before the shadow. Keeping the shadow would break the file.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/editor/ -count=1`
Expected: PASS, including every pre-existing validation test.

If a pre-existing test asserted the #349 scoping behaviour — that a workflow error in file A does *not* appear when saving file B — it should still pass: `f.Files` for a workflow failure names only the file declaring it. If one fails because it asserted a *service* error appears only on `noda.json`, that behaviour is preserved too, via `RootConfigPath`. A failure here means the attribution is wrong; fix the attribution, not the test.

- [ ] **Step 5: Verify the guard has teeth**

Temporarily change `startupFailures` to call `registry.DryRun` directly instead of `startup.Run`, and confirm `TestValidateAll_ReportsMiddlewareFailures` and `TestValidateAll_ReportsWorkflowGraphFailures` both FAIL. Restore. This is the mutation check #456 asked for, on the surface it was filed about.

- [ ] **Step 6: Commit**

```bash
git add internal/editor/
git commit -m "fix(editor): run the full startup phase list, not the dry-run bootstrap alone (#456)

A route wired to a misconfigured limiter showed green and failed at boot.
Uniform file attribution also removes two workarounds: the
ServiceConfigError special case and the #349 workflow-scoping trick, which
hid cross-file failures instead of attributing them."
```

---

## Task 10: Migrate dev-mode reload and make boot depend on the list

The last surface, and the change that makes the list structural rather than merely central.

**Files:**
- Modify: `cmd/noda/runtime.go:80-105`, `cmd/noda/main.go:407-409`
- Test: `cmd/noda/runtime_test.go` (create or append)

**Interfaces:**
- Consumes: `startup.Run`, `startup.Artifacts`.
- Produces: `initRuntime` unchanged externally; internally it no longer calls `registry.Bootstrap` or `engine.NewWorkflowCache`.

- [ ] **Step 1: Write the failing test**

Append to `cmd/noda/runtime_test.go` (create with `package main` if absent):

```go
// Boot must reject every project a validation surface rejects. This is the
// invariant the whole phase list exists to hold: if boot accepted one of
// these, `noda validate` would be lying about it.
func TestInitRuntime_RejectsEveryProjectValidateRejects(t *testing.T) {
	for _, fixture := range []string{
		"bad-workflow-graph-project",
		"bad-middleware-project",
		"bad-schedule-project",
		"bad-worker-project",
	} {
		t.Run(fixture, func(t *testing.T) {
			abs, err := filepath.Abs(filepath.Join("../../testdata", fixture))
			require.NoError(t, err)

			_, err = initRuntime(abs, "", initOptions{})

			require.Error(t, err, "boot must not accept a project validate rejects")
		})
	}
}

// The artifacts boot runs on must come from the phase list, not be rebuilt
// beside it. This is what stops the list from being deleted phase by phase.
func TestInitRuntime_TakesArtifactsFromTheStartupPhases(t *testing.T) {
	abs, err := filepath.Abs(filepath.Join("../../testdata", "valid-project"))
	require.NoError(t, err)

	rtCtx, err := initRuntime(abs, "", initOptions{})
	require.NoError(t, err)

	require.NotNil(t, rtCtx.Bootstrap)
	require.NotNil(t, rtCtx.WorkflowCache)
}
```

`testdata/valid-project` may declare services that `initRuntime` will try to dial with `DryRun: false`. Run the second test first: if it fails on a connection, use `testdata/minimal-project` instead, and confirm with `cat testdata/minimal-project/noda.json` that it declares no services.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./cmd/noda/ -run TestInitRuntime_ -v`
Expected: FAIL — the three non-middleware fixtures boot past their fault, or fail later than `initRuntime`.

- [ ] **Step 3: Rewrite `initRuntime`'s middle**

In `cmd/noda/runtime.go`, replace the bootstrap and workflow-cache blocks (lines 80-105) with:

```go
	// Run the startup phases. Boot takes its registries and workflow cache
	// from here rather than building them alongside, so a phase cannot be
	// dropped from the list without breaking this. That is what keeps
	// `noda validate` honest: the phases it runs are the ones boot runs.
	plugins := registry.NewPluginRegistry()
	arts, failures := startup.Run(context.Background(), startup.Input{
		RC:      rc,
		Plugins: allPlugins(),
	})
	if len(failures) > 0 {
		var msgs []string
		for _, f := range failures {
			msgs = append(msgs, f.Err.Error())
		}
		return nil, fmt.Errorf("%s validation failed:\n  %s", failures[0].Phase, strings.Join(msgs, "\n  "))
	}
	_ = plugins
```

Then delete the now-dead `plugins` lines — `registry.NewPluginRegistry()`, `registerCorePlugins(plugins)`, and the `_ = plugins` placeholder above. The final shape of that section is:

```go
	arts, failures := startup.Run(context.Background(), startup.Input{
		RC:      rc,
		Plugins: allPlugins(),
	})
	if len(failures) > 0 {
		var msgs []string
		for _, f := range failures {
			msgs = append(msgs, f.Err.Error())
		}
		return nil, fmt.Errorf("%s validation failed:\n  %s", failures[0].Phase, strings.Join(msgs, "\n  "))
	}

	secretsCtx := sm.ExpressionContext()

	return &runtimeContext{
		RC:             rc,
		SecretsCtx:     secretsCtx,
		SecretsManager: sm,
		Bootstrap:      arts.Bootstrap,
		WorkflowCache:  arts.WorkflowCache,
		TraceProvider:  traceProvider,
		Plugins:        arts.Bootstrap.Plugins,
		Logger:         logger,
		ConfigDir:      configDir,
	}, nil
}
```

`allPlugins` replaces `registerCorePlugins`. `cmd/noda/main.go:739` currently registers `corePlugins()` then `serviceOnlyPlugins()` into a registry. Add beside it:

```go
// allPlugins returns every plugin the runtime registers, in the order
// registerCorePlugins registered them. startup.Run does the registering, so
// this supplies the list rather than a populated registry.
func allPlugins() []api.Plugin {
	return append(corePlugins(), serviceOnlyPlugins()...)
}
```

Add `"github.com/chimpanze/noda/pkg/api"` to `cmd/noda/main.go`'s imports if absent. Leave `registerCorePlugins` in place if other call sites still use it — check with `grep -rn registerCorePlugins cmd/`. Delete it if none remain.

Add `"github.com/chimpanze/noda/internal/startup"` to `cmd/noda/runtime.go`'s imports and drop `"github.com/chimpanze/noda/internal/engine"` and `"github.com/chimpanze/noda/internal/registry"` if now unused.

- [ ] **Step 4: Wire dev-mode reload**

In `cmd/noda/main.go`, replace lines 407-409:

```go
			reloader.SetDryRun(func(rc *config.ResolvedConfig) []error {
				_, failures := startup.Run(context.Background(), startup.Input{
					RC: rc,
					Live: &startup.Registries{
						Plugins:  rtCtx.Plugins,
						Nodes:    rtCtx.Bootstrap.Nodes,
						Compiler: rtCtx.Bootstrap.Compiler,
					},
					RootConfigPath: filepath.Join(configDir, "noda.json"),
					DryRun:         true,
				})
				return startup.Errors(failures)
			})
```

`reload.go:147-167` already refuses the config swap when this hook reports failures (#345/#349), so no change is needed in `internal/devmode`. This changes what the hook runs, not what happens when it fails.

Add `"github.com/chimpanze/noda/internal/startup"` to `cmd/noda/main.go`'s imports.

- [ ] **Step 5: Write the dev-mode guard**

Append to `cmd/noda/runtime_test.go`:

```go
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
```

- [ ] **Step 6: Run everything**

Run: `go test ./cmd/noda/ ./internal/devmode/ -count=1`
Expected: PASS.

- [ ] **Step 7: Verify boot's dependency is real**

Delete the `arts.WorkflowCache = cache` assignment in `internal/startup/startup.go` and run `go build ./...`. It must fail or produce a nil-pointer panic in `TestInitRuntime_TakesArtifactsFromTheStartupPhases` — boot has no other source for that cache. Restore.

- [ ] **Step 8: Commit**

```bash
git add cmd/noda/
git commit -m "fix(devmode): run the full startup phase list on reload (#456)

initRuntime now takes its registries and workflow cache from startup.Run,
so a phase cannot be dropped from the list without breaking boot."
```

---

## Task 11: Documentation and full verification

**Files:**
- Modify: `CHANGELOG.md`
- Check: `docs/02-config/schedules.md`, `docs/02-config/workers.md`

- [ ] **Step 1: Add the CHANGELOG entry**

Under the unreleased `### Fixed` heading (create `### Changed` if the behaviour note needs its own):

```markdown
### Fixed

- The editor and dev-mode hot reload now run the full startup validation, not
  the dry-run bootstrap alone. A route wired to a misconfigured `limiter` or
  `auth.jwt` showed green in the editor and reloaded clean, then failed at
  boot (#456).

### Changed

- `noda validate`, `noda test`, MCP `noda_validate_config`, the editor, and
  dev-mode reload now also check workflow graph compilation, cron specs, and
  the worker concurrency bound. These were boot failures no validation surface
  ran: a workflow containing a cycle, or a schedule with a five-field cron
  spec, passed `noda validate` and then killed `noda start`. Projects with
  these faults are now rejected at validate time.
```

- [ ] **Step 2: Check the docs for the five-field cron form**

Run: `grep -rn '"cron"' docs/ examples/`

Every user-facing occurrence must have six fields. As of this plan, `docs/02-config/schedules.md` and `docs/04-guides/authentication.md` are correct and `docs/_internal/architecture-plan.md:335` has the five-field form. Fix the internal doc; if any user-facing doc has drifted since, fix that too.

Then confirm `docs/02-config/schedules.md` states the seconds field is required. If it does not, add a sentence — the five-field form is what every non-Go cron accepts, so readers will reach for it:

```markdown
Noda's cron specs have **six** fields, not five: the leading field is seconds.
`0 0 */6 * * *` is every six hours on the hour; the five-field `0 */6 * * *`
is rejected.
```

- [ ] **Step 3: Document the concurrency bound**

Confirm `docs/02-config/workers.md` states the maximum for `concurrency`. If it does not, add it — the JSON schema does not express the bound, so the doc is the only place a reader can learn it:

```markdown
`concurrency` — number of parallel consumers. Maximum 1000. Values below 1
are treated as 1.
```

- [ ] **Step 4: Run the full gate**

From the repo root of the worktree:

```bash
gofmt -l . && go build ./... && go vet ./... && golangci-lint run --timeout=5m --build-tags=integration && go test ./...
```

Expected: `gofmt -l` prints nothing, and everything passes. `golangci-lint` is a separate gate from `go vet` and has caught things `go vet` did not; the `--build-tags=integration` is what CI uses and without it tagged files go unlinted.

- [ ] **Step 5: Run the integration suite**

```bash
go test -tags integration ./... -count=1
```

Expected: PASS, including `TestCookbookCoverage`. `internal/testing/cookbook/runner.go:301,324` calls `registry.Bootstrap` and `engine.NewWorkflowCache` directly — it is a test harness, not a validation surface, so it is deliberately not migrated. If it now fails, a cookbook example has a fault the new phases catch; fix the example.

- [ ] **Step 6: Confirm the original bug is dead**

```bash
mkdir -p /tmp/noda-cycle/workflows /tmp/noda-cycle/routes
echo '{"services":{}}' > /tmp/noda-cycle/noda.json
cat > /tmp/noda-cycle/routes/hello.json <<'JSON'
{"id":"hello","method":"GET","path":"/hello","trigger":{"workflow":"hello"}}
JSON
cat > /tmp/noda-cycle/workflows/hello.json <<'JSON'
{"id":"hello","nodes":{
  "a":{"type":"util.log","config":{"level":"info","message":"a"}},
  "b":{"type":"util.log","config":{"level":"info","message":"b"}},
  "respond":{"type":"response.json","config":{"status":200,"body":{}}}},
 "edges":[{"from":"a","to":"b"},{"from":"b","to":"a"},{"from":"b","to":"respond"}]}
JSON
go run ./cmd/noda validate --config /tmp/noda-cycle
```

Expected: a non-zero exit naming the cycle. Against `origin/main` this printed `✓ All config files valid (3 files checked)` and exited 0.

- [ ] **Step 7: Commit**

```bash
git add CHANGELOG.md docs/
git commit -m "docs: record the startup phase unification (#456)

Notes the three phases new to every validation surface, and documents the
six-field cron requirement and the worker concurrency bound."
```

---

## Task 12: Whole-branch review

- [ ] **Step 1: Read the full diff**

```bash
git diff origin/main...HEAD
```

- [ ] **Step 2: Check each claim in the spec against the code**

Walk the spec's sections. For each, name the file and line implementing it. Specifically confirm:

- Every one of the five surfaces calls `startup.Run` — `grep -rn "startup.Run" --include=*.go .` must show `cmd/noda/runtime.go` (boot), `cmd/noda/validate.go`, `cmd/noda/main.go` (dev-mode), `internal/mcp/tools.go`, `internal/editor/validation.go`. Five call sites plus the package's own tests.
- No surface still calls `registry.DryRun` or `server.ValidateMiddlewareBuilds` directly — `grep -rn "registry.DryRun\|ValidateMiddlewareBuilds" --include=*.go .` must show only `internal/startup/` and tests.
- `internal/validate` is gone — `ls internal/validate` must fail.
- `internal/startup` does not import `plugins/all` — `go list -deps ./internal/startup | grep "plugins/"` must print nothing.
- `internal/editor` gained no cgo dependency — `go list -deps ./internal/editor | grep -E "bimg|plugins/image"` must print nothing.

- [ ] **Step 3: Re-run every mutation check in one pass**

For each phase, delete it from `Run`, run the five surfaces' guards, confirm they redden, restore:

```bash
go test ./internal/startup/ ./cmd/noda/ ./internal/mcp/ ./internal/editor/ -count=1
```

Twenty assertions — five surfaces × four phases. Any that stays green with its phase deleted is a vacuous guard and must be rewritten. This repo has shipped three.

- [ ] **Step 4: Push and open the PR**

```bash
git push -u origin feat/startup-phase-unification
gh pr create --title "fix: make the validation phase list the boot path (#456)" --body "$(cat <<'EOF'
Closes #456.

`noda validate` reported `✓ All config files valid` for projects `noda start`
refused to boot. Two reproductions against `53ffd46`:

- a workflow containing a cycle → `compiling workflows: cycle detected: a → b → a`
- a five-field cron spec → `lifecycle start: starting scheduler: expected exactly 6 fields, found 4`

#456 describes this as two surfaces missing one phase. It was four gaps, three
of them on every surface — including the three #455 had just unified.

## Why a fourth shared helper would not have worked

#442, #444, and #448 each fixed this by building a better-placed *copy* of the
boot sequence. A copy drifts: #448's was already missing three phases the day
it merged.

`internal/startup` is not a copy. `initRuntime` takes its `BootstrapResult` and
`WorkflowCache` from `startup.Run`, so the load-bearing phases cannot be
dropped without breaking the build. The rest are held by mutation guards across
all five surfaces.

The boundary is falsifiable and was tested: `server.Setup`'s connection and
OpenAPI registration were audited and found already covered by
`config.ValidateAll` crossrefs, as was everything in `ParseWorkerConfigs`
except the concurrency bound. The rule predicted which gaps existed.

## Also fixed

`testdata/valid-project` declared `"cron": "0 */6 * * *"` — five fields where
the runtime installs `cron.WithSeconds()`. The fixture named `valid-project`
described a project whose scheduler could not start.

The editor loses two workarounds that existed only because failures were
untyped: the `ServiceConfigError` special case, and the #349 trick that
pre-trimmed `rc.Workflows` to the saved file — which *hid* cross-file failures
rather than attributing them, so a file could read clean while the project
could not boot.

## Behaviour change

Projects with a workflow cycle, an invalid cron spec, or an over-limit worker
concurrency are now rejected at validate time. All 10 example projects pass
clean; blast radius on real projects is zero.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| `internal/startup` package, `Run` signature | 5 |
| Registries phase | 5 |
| Workflows phase | 5 |
| Middleware, Schedules, Workers phases | 6 |
| Boot consumes `Artifacts` | 10 |
| `MiddlewareBuildError`, `WorkflowCompileError` | 2, 1 |
| Editor deletes both workarounds | 9 |
| Dev mode | 10 |
| Dependency placement (no `plugins/all`) | 5, verified in 12 |
| Call-site table (5 surfaces) | 7, 8, 9, 10 |
| `testdata/valid-project` cron fix | 6 |
| CHANGELOG | 11 |
| Mutation guards on all five surfaces | 6, 7, 8, 9, 10, re-run in 12 |
| Fixtures + "fault really exists" assertions | 6 |
| `Error()` byte-identical | 1, 2 |

**Deviations from the spec**, both found while pinning signatures:

1. `Failure.File string` → `Failure.Files []string`. One misconfigured middleware breaks every route naming it, and the editor must mark all of them; a single string cannot express that.
2. `Input` gains `RootConfigPath`. With it the editor's filter is `slices.Contains(f.Files, absPath)` with no special cases at all — which is what the spec promised but could not deliver with a `File` that was empty for root-config faults.

Both are recorded in the spec document as of this plan's commit.

**Type consistency:** `startup.Run`, `startup.Input`, `startup.Failure`, `startup.Artifacts`, `startup.Registries`, `startup.Errors`, `startup.OfPhase`, `engine.WorkflowCompileError`, `server.MiddlewareBuildError`, `scheduler.SpecError`, `scheduler.ValidateSpecs`, `worker.ConfigError`, `worker.ValidateConfigs`, `allPlugins` — each defined in exactly one task and referenced with the same name and shape thereafter. `scheduler.ValidateSpecs` and `worker.ValidateConfigs` change return type inside Task 6 Step 5; the tests written in Tasks 3 and 4 are updated there in the same step.
