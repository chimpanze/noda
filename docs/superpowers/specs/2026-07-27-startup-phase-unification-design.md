# Startup phase unification — design

**Date:** 2026-07-27
**Issue:** #456 (plus three previously unfiled holes found while scoping it)
**Branch:** `feat/startup-phase-unification`

## Problem

`noda validate` reports a project valid that `noda start` refuses to boot. Two
demonstrations, both reproduced against `53ffd46`:

```
$ noda validate   →  ✓ All config files valid (3 files checked)      exit 0
$ noda start      →  Error: compiling workflows: compile workflow "hello":
                            cycle detected: a → b → a                exit 1
```

```
$ noda validate   →  ✓ All config files valid (4 files checked)      exit 0
$ noda start      →  Error: lifecycle start: starting scheduler:
                            register job "nightly":
                            expected exactly 6 fields, found 4       exit 1
```

Issue #456 describes this as two surfaces (editor, dev-mode reload) missing one
phase (middleware). That is one cell of a larger table. The real state:

| Boot step (`cmd/noda/runtime.go`) | validate / test / MCP | editor | devmode |
|---|---|---|---|
| `config.ValidateAll` | yes | yes | yes |
| `registry.Bootstrap` (dry-run) | yes | yes | yes |
| `engine.NewWorkflowCache` | **no** | **no** | **no** |
| `server.Setup` middleware builds | yes (#455) | **no** | **no** |
| `scheduler.Start` cron specs | **no** | **no** | **no** |
| `worker.Start` concurrency bound | **no** | **no** | **no** |

Three of the four gaps are on *every* surface, including the three that #455
had just finished unifying.

## Root cause

This is the fourth occurrence of one bug. #442 (`ValidateStartup` vs
`ValidateStartupDryRun`), #444 (checks lived only in the validate command, so
`noda test` ran what validate rejected), #448 (#444's fix landed in `package
main`, unreachable by `internal/mcp`), and now #456.

Each fix built a **copy** of the boot sequence and placed it somewhere more
central. A copy drifts, because nothing ties it to the thing it copies. #455's
copy was already missing three rows on the day it merged.

The fix is not a better-placed copy. It is to make the phase list *be* the boot
path: boot consumes the list and depends on an artifact the list produces, so
the list cannot be reduced without breaking boot.

## Design

### `internal/startup`

A new package superseding and absorbing `internal/validate`.

```go
package startup // imports: config, registry, engine, server, scheduler, worker
                // NOT plugins/all, NOT cgo — see "Dependency placement"

func Run(ctx context.Context, in Input) (*Artifacts, []Failure)

type Input struct {
    RC             *config.ResolvedConfig
    Plugins        []api.Plugin  // caller-supplied; keeps plugins/all out of the graph
    Live           *Registries   // non-nil: reuse caller's registries (editor, devmode)
    RootConfigPath string        // absolute path of noda.json; see Attribution
    DryRun         bool          // skip service creation
}

type Artifacts struct {
    Bootstrap     *registry.BootstrapResult
    WorkflowCache *engine.WorkflowCache  // populated in all modes; boot consumes it
}

type Failure struct {
    Phase    Phase
    Files    []string // absolute paths implicated; empty when attributable to none
    JSONPath string   // optional, when a phase can point at a field
    Err      error
}
```

`Files` is a slice rather than a single path because one misconfigured
middleware breaks every route that references it, and the editor must mark all
of them.

`config.ValidateAll` stays **outside** `Run`, which takes an already-resolved
`*ResolvedConfig`. Two reasons: it is the one step that never drifted (all five
surfaces run it), and it returns `config.ValidationError` carrying `JSONPath`,
which the editor consumes directly to place in-editor markers. Folding it in
would flatten detail the UI depends on.

### The phases

| Phase | DryRun + fresh registries | DryRun + live registries | Live (boot) |
|---|---|---|---|
| `Registries` | `Bootstrap(DryRun: true)` | `registry.DryRun(...)` | `Bootstrap(DryRun: false)` |
| `Workflows` | `engine.NewWorkflowCache` | same | same — **boot takes the cache from here** |
| `Middleware` | `server.ValidateMiddlewareBuilds` | same | same |
| `Schedules` | cron spec parse | same | same |
| `Workers` | concurrency bound | same | same |

`Run` stops at the first failing phase, matching today's `validate.Project`
behaviour and the CLI's one-headed-message-per-phase output.

Three phases now also run at boot that did not before. All three are
improvements, not merely consistency:

- **Middleware** — boot currently fails inside `server.Setup` with
  `register routes: ...`, naming whichever route it reached first.
  `ValidateMiddlewareBuilds` names every affected route in a deterministic
  order (#450).
- **Schedules** — a bad cron spec currently fails at `lifecycle.StartAll`,
  after services have been dialed and the port bound. It will now fail before
  any of that.
- **Workers** — same, one step earlier than `worker.Runtime.Start`.

### What makes this structural

`initRuntime` takes its `WorkflowCache` from `Artifacts` rather than calling
`engine.NewWorkflowCache` itself. The Workflows phase therefore cannot be
deleted or skipped without breaking the build. The `Registries` phase is
load-bearing the same way, via `Artifacts.Bootstrap`.

`Middleware`, `Schedules`, and `Workers` produce no artifact, so they have no
compile-time tether. They are held by the mutation guards below — the same
check PR #455 used.

This does not make drift impossible in the limit: someone can still add a boot
step outside `Run`. It makes it impossible to add a *check* to one surface and
not the others, which is the failure mode that recurred four times. The
boundary rule (below) is what keeps new boot steps landing in the right place.

### The boundary rule

A boot step belongs in `Run` if it can fail from configuration alone. Steps
requiring the network or filesystem at boot — dialing services, loading Wasm
modules, binding the port, `HealthCheckAll` — cannot be checked offline and
stay in `cmd/noda`.

The rule is falsifiable, and it was tested while scoping: `server.Setup`'s
connection registration and OpenAPI registration were audited and found
**already fully covered** by `config.ValidateAll` crossrefs
(`crossrefs.go:126-138` covers `connections.sync.pubsub`). Likewise all of
`worker.ParseWorkerConfigs` except the concurrency bound. The rule predicts
which gaps exist, and the prediction held.

### Attribution

`Failure.Files` requires lifting the source file out of two error types that
today bury it in formatted text:

- `server.MiddlewareBuildError` — scopes already name the file, because
  `rc.Routes` and `rc.Connections` are keyed by absolute path. Observed:
  `route "/…/routes/hello.json": middleware "limiter": limiter: max=0 …`
- An equivalent in `engine` for workflow compile failures. Observed:
  `compile workflow "/…/workflows/hello.json": cycle detected: a → b → a`

Both keep `Error()` byte-identical, so #450's determinism tests and all CLI
output are unchanged. This is a pure type addition.

`Failure.JSONPath` is optional and mostly empty — "cycle detected" has no
meaningful field to point at. The `Workers` phase supplies `/concurrency`.

### What the editor deletes

`internal/editor/validation.go` currently carries two workarounds, both
symptoms of untyped errors:

1. The `isRootConfig` branch (`validation.go:74-79`), special-casing
   `registry.ServiceConfigError` because it has no file.
2. The #349 workflow-scoping trick (`validation.go:60-65`), which pre-trims
   `rc.Workflows` to the saved file so that other files' errors do not appear.

Both are replaced by one filter, with no special cases at all:

```go
slices.Contains(f.Files, absPath)
```

This is what `Input.RootConfigPath` buys. Faults whose *definition* lives in
`noda.json` — a service config, a middleware config — append it to `Files`
alongside the referencing route files. So editing `noda.json` surfaces the
limiter fault where the fix goes, and editing the route surfaces it where the
breakage shows, without the caller special-casing either.

The scoping trick is strictly worse than the filter — it *hides* cross-file
errors rather than attributing them, so a file can read clean while the project
is broken. Removing it is a fix, not just a simplification.

### Dev mode

No new policy decision. `reload.go:147-167` already refuses the config swap
when the dry-run hook reports failures, established by #345/#349. This changes
what that hook runs, not what happens when it fails.

Worth recording: dev-mode reload does not rebuild routes or middleware
(`onReload` only invalidates the workflow cache), so a middleware fault in a
reloaded config was never going to crash the running process. Rejecting the
reload is about telling the developer at save time instead of at next boot.

### Dependency placement

`internal/validate` cannot be imported by `internal/editor` today because it
imports `plugins/all`, which pulls the bimg/libvips cgo image plugin. Moving
the plugin list to a caller-supplied `Input.Plugins` removes that edge —
`internal/startup` then imports only config, registry, engine, server,
scheduler, and worker.

Verified: `internal/server` imports no plugins and no cgo; `internal/editor`
already has 10 of its 12 internal dependencies; neither `internal/scheduler`
nor `internal/engine` imports `internal/editor` or `internal/devmode`, so no
cycle is introduced.

`registry.DryRun` stays in `internal/registry` for the same reason it was put
there in #455.

## Call sites

| Surface | Call |
|---|---|
| `cmd/noda` boot (`initRuntime`) | `Run(ctx, Input{RC: rc, Plugins: all.All()})`, takes `Artifacts.Bootstrap` and `Artifacts.WorkflowCache` |
| `noda validate` | `Run(ctx, Input{RC: rc, Plugins: all.All(), DryRun: true})` |
| `noda test` | same |
| MCP `noda_validate_config` | same |
| editor `/_noda/validate`, `/_noda/validate-file` | `Run(ctx, Input{RC: rc, Live: e.registries, DryRun: true})` |
| dev-mode reload (`SetDryRun`) | same |

## Blast radius

Measured by running all four new phases against every project in `examples/`
and `testdata/`:

- **All 10 example projects pass clean.** Nothing real regresses.
- `testdata/valid-project/schedules/cleanup.json` has `"cron": "0 */6 * * *"` —
  five fields, where the scheduler runs `cron.WithSeconds()` and needs six. The
  fixture named `valid-project` describes a project that cannot boot its
  scheduler. Fix to `"0 0 */6 * * *"` as part of this work.
- `testdata/bad-middleware-project` fails the Middleware phase, as intended —
  it is #455's fixture and exists for that purpose.

User-facing docs were checked: `docs/02-config/schedules.md` and
`docs/04-guides/authentication.md` all use correct six-field specs. The
five-field form appears only in the fixture above and in
`docs/_internal/architecture-plan.md`.

Noda is pre-alpha with no live deployments, so the stricter validation lands
directly — no opt-out flag, no staged rollout. A CHANGELOG entry under
`### Changed` records that `noda validate` now rejects projects it previously
accepted, which is the point of the change.

## Testing

### Mutation guards

Per #456's requirement, and the check PR #455 used: deleting any phase from
`Run` must redden a test on **each of the five surfaces** — `noda validate`,
`noda test`, MCP, editor, dev-mode reload. Twenty assertions across four
phases; a table test per surface.

The vacuous-guard trap is live here. `internal/validate`'s
`TestProject_BootstrapFailureSkipsMiddlewarePhase` passed with the
short-circuit deleted, because its fixture had no middleware at all. Every
guard below must assert up front that the fixture's hidden fault actually
exists, before asserting that validation catches it.

### Fixtures

| Fixture | Passes | Fails only |
|---|---|---|
| `testdata/bad-middleware-project` (exists) | ValidateAll, Registries | Middleware |
| `testdata/bad-workflow-graph-project` (new) | ValidateAll, Registries | Workflows — cycle `a → b → a` |
| `testdata/bad-schedule-project` (new) | ValidateAll, Registries | Schedules — five-field cron |
| `testdata/bad-worker-project` (new) | ValidateAll, Registries | Workers — `concurrency: 5000` |

Each new fixture needs the same up-front assertion `bad-middleware-project`
earned: that it passes `config.ValidateAll` and the Registries phase, so the
guard proves the *phase* caught it rather than something upstream.

### Regression

- `Error()` output of the two new typed errors is byte-identical to today's —
  pin with a golden assertion, since #450's determinism tests depend on it.
- Boot fails on each of the four fixtures, proving the phases match real boot
  behaviour rather than a parallel opinion of it.
- Editor `validate-file` attributes a middleware error to the route file *and*
  to `noda.json`, and does not attribute another file's error to the saved file.

## Out of scope

- Running both phases and returning their union instead of short-circuiting.
  `internal/validate` deliberately preserved the short-circuit as a CLI output
  question separate from unification; that reasoning still holds.
- Wasm module loading, service dialing, port binding, `HealthCheckAll` — the
  boundary rule excludes them.
- `worker.ResolveMiddleware` silently dropping unknown middleware names. Noted
  while auditing; it is a silent-failure bug, not a boot failure, so it has no
  phase to live in. File separately.
