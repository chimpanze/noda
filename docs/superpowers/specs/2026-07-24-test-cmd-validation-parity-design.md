# `noda test` runs the full validate pipeline (#444)

**Date:** 2026-07-24
**Issue:** [#444](https://github.com/chimpanze/noda/issues/444)
**Status:** Approved, ready for planning

## Problem

`noda test` runs `config.ValidateAll` and nothing else before handing the resolved
config to the test runner. It never runs the startup dry-run, so none of these
apply to a test run: node config schemas, service slot references, service config
schemas (#376), edge-output validation (#379), and — since #442 — the
unwired-outcome-output rule.

A green `noda test` therefore does not imply the project validates or boots, which
inverts the reasonable expectation that tests are the stricter gate.

Every other consumer already funnels through the dry-run: `noda validate`, boot
(`ValidateStartup`), dev-mode reload, the editor (`internal/editor/validation.go`),
and MCP `noda_validate_config`. `noda test` is the one remaining gap.

### Reproduction

On a project freshly created by `noda init`, adding one `db.create` workflow and a
test suite that mocks it:

```
$ noda validate
Error: bootstrap failed:
  workflow ".../workflows/repro.json", node "create" (db.create): unknown config field "service"
  workflow ".../workflows/repro.json", node "create": missing required service slot "database"
  workflow ".../workflows/repro.json", node "create" (db.create): outcome output "exists" has no
    outbound edge — a fired outcome output with no edge silently ends the path; wire it ...

$ noda test
  2 passed, 0 failed, 2 total
```

Three distinct classes of error — unknown config field, missing service slot,
unwired outcome output — and the test command reports green.

## Design

### Shared helper

Add `validateProject(rc *config.ResolvedConfig) error` in a new file
`cmd/noda/validate.go`. It contains everything `noda validate` currently does
*after* `config.ValidateAll`:

1. Build a plugin registry and `registerCorePlugins`.
2. `registry.Bootstrap(ctx, rc, plugins, registry.BootstrapOptions{DryRun: true})`.
3. `server.ValidateMiddlewareBuilds(rc)`.

Error strings are preserved verbatim (`"bootstrap failed:\n  %s"`,
`"middleware validation failed:\n  %s"`) so `noda validate`'s output does not
change.

`newValidateCmd` stays in `main.go`; only the helper moves. This keeps the diff
readable and the command's shape familiar.

### Callers

| Caller | Change |
| --- | --- |
| `newValidateCmd` (`cmd/noda/main.go`) | Body of the post-`ValidateAll` block replaced by `validateProject(rc)`. No behavior change. |
| `newTestCmd` (`cmd/noda/main.go`) | **New call**, immediately after `config.ValidateAll` and before `nodatesting.LoadTests`. A non-nil error fails the command before any test executes. |
| `runProjectTestSuites` (`cmd/noda/shipped_tests_test.go`) | **New call.** Its doc comment claims it "mirrors exactly what the test command does"; without this the claim goes stale and the gate stops covering the new behavior. |

### Why full parity rather than the dry-run alone

The issue suggests calling `ValidateStartupDryRun` directly. Routing all three
callers through one helper instead is deliberate: PR #443's final review caught a
bug caused by exactly this shape — `ValidateStartup` and `ValidateStartupDryRun`
were separate functions, and a new rule added to one silently missed the other.
Two independent copies of "what validation means" drift, and the drift is silent.
One helper makes a future `validate` rule automatically a `test` rule.

Including `ValidateMiddlewareBuilds` is part of that: workflow tests never
exercise middleware, but a project with an invalid limiter or JWT config cannot
boot, and the user's mental model is "test is at least as strict as validate".

### No escape hatch

No `--skip-validate` flag. It is YAGNI, and it would reopen the gap the change
closes. If a real need for partial-fixture testing appears later, it can be added
then with a concrete use case to shape it.

### Blast radius

None of the shipped projects break:

- All nine projects that ship a `tests/` directory (`examples/auth-demo`,
  `init-example`, `realtime-collab`, `realworld`, `rest-api`, `saas-backend`,
  `testdata/auth`, `testdata/node-e2e`, `testdata/valid-project`) already pass
  dry-run bootstrap under `TestShippedProjectsValidate`.
- A fresh `noda init` scaffold was verified to pass `noda validate` cleanly, so
  the flagship `noda init` → `noda test` first-run flow stays green.

The change is nonetheless behavior-breaking for users: a project whose config does
not validate now fails `noda test` instead of running it.

## Testing

- **New fixture** `testdata/test-cmd-invalid-project/`: passes `config.ValidateAll`,
  fails the dry-run (an unwired outcome output), and ships a test suite whose cases
  *pass*. This is the exact #444 shape — without the fix `noda test` is green on it.
- **Command-level regression test** executing the real cobra command
  (`root.SetArgs([]string{"test", "--config", "../../testdata/test-cmd-invalid-project"})`,
  matching the existing pattern in `commands_test.go`) asserting it returns an error.
- **Unit test on `validateProject`**: nil for `testdata/valid-project`, non-nil for
  the new fixture, with the error naming the offending node.
- **Parity coverage is free**: `TestScaffoldedProjectPassesItsOwnTests` and
  `TestShippedExamplesPassTheirTests` now exercise `validateProject` transitively
  through `runProjectTestSuites`.
- **Mutation check**: remove the `validateProject` call from `newTestCmd` and
  confirm the new regression test goes red.

## Documentation

- `docs/04-guides/testing-and-debugging.md` — the callout in "Running Tests"
  already states that validation does not run tests; add the converse, that
  `noda test` now validates the project first and stops on failure.
- `docs/01-getting-started/quick-start.md` — §7 "Validate and Test" (line ~384)
  describes what `noda validate` checks; add that `noda test` runs those same
  checks first and stops on failure.
- `CHANGELOG.md` `[Unreleased]` under **Changed**, flagged breaking: a project
  whose config does not validate now fails `noda test` instead of running it.
