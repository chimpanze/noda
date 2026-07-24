# `noda test` Validation Parity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `noda test` run the same startup validation `noda validate` runs, so a green test run implies the project boots (#444).

**Architecture:** Extract everything `noda validate` does after `config.ValidateAll` — dry-run `registry.Bootstrap` plus `server.ValidateMiddlewareBuilds` — into a single `validateProject` helper in `cmd/noda/validate.go`. Call it from the validate command, the test command, and the `runProjectTestSuites` test helper that claims to mirror the test command. One implementation means the call sites cannot drift.

**Tech Stack:** Go, cobra CLI, testify (`require`/`assert`), JSON config fixtures under `testdata/`.

## Global Constraints

- `noda validate`'s output must not change. The error strings `"bootstrap failed:\n  %s"` and `"middleware validation failed:\n  %s"` are preserved verbatim, joined with `"\n  "`.
- No `--skip-validate` escape hatch, on either command.
- The helper lives in a new file `cmd/noda/validate.go`. `newValidateCmd` stays in `cmd/noda/main.go` — do not move the command.
- Work happens in the worktree at `.claude/worktrees/fix-444-test-validation-parity` on branch `worktree-fix-444-test-validation-parity`. Before any commit, confirm `git rev-parse --show-toplevel` ends in `fix-444-test-validation-parity`.
- `/docs/superpowers/` is gitignored but 73 spec/plan files are tracked; force-add with `git add -f` when committing anything under it.
- Package name for all `cmd/noda` files is `main`.

---

### Task 1: Extract the `validateProject` helper

Pure refactor. `noda validate` must behave identically afterwards; this task exists so a reviewer can check the extraction in isolation, before any behavior changes on top of it.

**Files:**
- Create: `cmd/noda/validate.go`
- Modify: `cmd/noda/main.go:142-165` (the block inside `newValidateCmd` between `config.ValidateAll` and the success `Printf`)

**Interfaces:**
- Consumes: `registerCorePlugins(*registry.PluginRegistry) error` and `corePlugins()`, both already in `cmd/noda/main.go`.
- Produces: `func validateProject(rc *config.ResolvedConfig) error` — returns `nil` when the project passes every post-`ValidateAll` check, otherwise a single formatted error. Tasks 2 and 3 call this exact signature.

- [ ] **Step 1: Capture the current `noda validate` output as the refactor's baseline**

```bash
go run ./cmd/noda validate --config ./testdata/invalid-project > /tmp/validate-invalid-before.txt 2>&1; echo "exit=$?" >> /tmp/validate-invalid-before.txt
go run ./cmd/noda validate --config ./testdata/valid-project > /tmp/validate-valid-before.txt 2>&1; echo "exit=$?" >> /tmp/validate-valid-before.txt
cat /tmp/validate-invalid-before.txt /tmp/validate-valid-before.txt
```

Keep both files; Step 5 diffs against them.

- [ ] **Step 2: Create `cmd/noda/validate.go`**

```go
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
```

- [ ] **Step 3: Replace the extracted block in `newValidateCmd`**

In `cmd/noda/main.go`, delete these lines (currently 142-165, immediately after the `config.ValidateAll` error check and immediately before `fmt.Printf("✓ All config files valid ...")`):

```go
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
```

and put this in their place:

```go
			if err := validateProject(rc); err != nil {
				return err
			}
```

Leave `main.go`'s import block alone: `context`, `strings`, `registry`, and `server` all remain in use elsewhere in the file (`context` at the `buildWorkflowRunner` closure, `strings` in the dev-command editor path, `registry` in the dev-command dry-run hook, `server` in `createServer`). Confirm with `go build ./...` in Step 4 rather than editing imports by hand.

- [ ] **Step 4: Build and run the existing command tests**

```bash
go build ./... && go test ./cmd/noda/ -run 'TestValidateCmd|TestTestCmd|TestShippedProjectsValidate|TestValidate_Harness04' -v 2>&1 | tail -30
```

Expected: PASS. These are pre-existing tests; a failure here means the extraction changed behavior.

- [ ] **Step 5: Diff the CLI output against the baseline**

```bash
go run ./cmd/noda validate --config ./testdata/invalid-project > /tmp/validate-invalid-after.txt 2>&1; echo "exit=$?" >> /tmp/validate-invalid-after.txt
go run ./cmd/noda validate --config ./testdata/valid-project > /tmp/validate-valid-after.txt 2>&1; echo "exit=$?" >> /tmp/validate-valid-after.txt
diff /tmp/validate-invalid-before.txt /tmp/validate-invalid-after.txt && diff /tmp/validate-valid-before.txt /tmp/validate-valid-after.txt && echo "IDENTICAL"
```

Expected: `IDENTICAL`. Any diff means the extraction was not behavior-preserving — fix it before committing.

- [ ] **Step 6: Commit**

```bash
git rev-parse --show-toplevel   # must end in fix-444-test-validation-parity
git add cmd/noda/validate.go cmd/noda/main.go
git commit -m "refactor(cmd): extract validateProject from the validate command

Pure extraction, no behavior change: the dry-run bootstrap and middleware
build checks move verbatim out of newValidateCmd into a helper the test
command can share (#444).

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Fixture + failing test, then wire `validateProject` into `noda test`

This is the behavior change. The fixture is the exact #444 shape: a project whose config parses, whose tests pass, and which cannot boot.

**Files:**
- Create: `testdata/test-cmd-invalid-project/noda.json`
- Create: `testdata/test-cmd-invalid-project/workflows/create-widget.json`
- Create: `testdata/test-cmd-invalid-project/tests/test-create-widget.json`
- Create: `cmd/noda/validate_test.go`
- Modify: `cmd/noda/commands_test.go` (append one test)
- Modify: `cmd/noda/main.go` — `newTestCmd`, immediately after its `config.ValidateAll` error check (currently line 197) and before `nodatesting.LoadTests` (currently line 200)

**Interfaces:**
- Consumes: `validateProject(rc *config.ResolvedConfig) error` from Task 1; `testRootCmd(sub *cobra.Command) *cobra.Command` from `cmd/noda/commands_test.go:15`.
- Produces: the fixture path `testdata/test-cmd-invalid-project`, used by Task 3's verification sweep.

- [ ] **Step 1: Create the fixture config**

`testdata/test-cmd-invalid-project/noda.json` — a literal connection URL, not `$env()`, so the fixture needs no environment variables. Dry-run never opens a connection.

```json
{
  "services": {
    "main-db": {
      "plugin": "postgres",
      "config": {
        "url": "postgres://localhost:5432/noda_test?sslmode=disable"
      }
    }
  }
}
```

- [ ] **Step 2: Create the fixture workflow with an unwired outcome output**

`testdata/test-cmd-invalid-project/workflows/create-widget.json`. `db.create` declares `exists` as an outcome output (`plugins/db/create.go:43`); wiring only `success` is what makes the project fail the dry-run. Leaving `error` unwired is deliberate and is *not* a validate failure — the engine catches that at runtime only.

```json
{
  "id": "create-widget",
  "name": "Create Widget",
  "nodes": {
    "insert": {
      "type": "db.create",
      "services": { "database": "main-db" },
      "config": {
        "table": "widgets",
        "data": {
          "name": "{{ input.name }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": "{{ nodes.insert }}"
      }
    }
  },
  "edges": [
    { "from": "insert", "to": "respond" }
  ]
}
```

- [ ] **Step 3: Create the fixture's passing test suite**

`testdata/test-cmd-invalid-project/tests/test-create-widget.json`. This case must PASS — that is the whole point of the fixture.

```json
{
  "id": "test-create-widget",
  "workflow": "create-widget",
  "tests": [
    {
      "name": "creates widget successfully",
      "input": {
        "name": "Test Widget"
      },
      "mocks": {
        "insert": {
          "output": {
            "id": "uuid-123",
            "name": "Test Widget"
          }
        },
        "respond": {
          "output": {
            "status": 201,
            "body": {
              "id": "uuid-123",
              "name": "Test Widget"
            }
          }
        }
      },
      "expect": {
        "status": "success",
        "output": {
          "respond.status": 201
        }
      }
    }
  ]
}
```

- [ ] **Step 4: Write the characterization test proving the fixture has the #444 shape**

Create `cmd/noda/validate_test.go`. This test does not depend on the fix — it asserts the fixture is genuinely the trap #444 describes, and it will keep the fixture honest if someone later "fixes" the workflow by wiring `exists`.

```go
package main

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validateProject must accept a project that `noda validate` accepts.
func TestValidateProject_AcceptsValidProject(t *testing.T) {
	dir := "../../testdata/valid-project"

	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)
	rc, errs := config.ValidateAll(dir, "", sm)
	require.Empty(t, errs)

	require.NoError(t, validateProject(rc))
}

// testdata/test-cmd-invalid-project is the #444 trap in fixture form: its
// config parses, its workflow test passes, and it cannot boot. Asserting all
// three here is what makes TestTestCmd_FailsOnProjectThatDoesNotValidate
// meaningful — without the middle assertion, that test could pass for the
// boring reason that the fixture's tests were failing anyway.
func TestValidateProject_RejectsProjectThatCannotBoot(t *testing.T) {
	dir := "../../testdata/test-cmd-invalid-project"

	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)

	rc, errs := config.ValidateAll(dir, "", sm)
	require.Empty(t, errs, "fixture must pass config validation — the gap only exists past that point")

	suites, err := nodatesting.LoadTests(rc)
	require.NoError(t, err)
	require.NotEmpty(t, suites)

	reg, err := buildCoreNodeRegistry()
	require.NoError(t, err)
	for _, suite := range suites {
		for _, res := range nodatesting.RunTestSuite(suite, rc, reg, sm.ExpressionContext()) {
			require.Truef(t, res.Passed,
				"fixture's own tests must pass: suite %q case %q: %s", suite.ID, res.CaseName, res.Error)
		}
	}

	err = validateProject(rc)
	require.Error(t, err, "unwired outcome output must fail startup validation")
	assert.Contains(t, err.Error(), "outcome output")
	assert.Contains(t, err.Error(), "exists")
}
```

- [ ] **Step 5: Write the failing command-level regression test**

Append to `cmd/noda/commands_test.go`, after `TestTestCmd_VerboseMode` (currently ends at line 156):

```go
// `noda test` must refuse to run a project that `noda validate` rejects.
// Before #444 the test runner skipped the startup dry-run entirely, so a
// workflow with an unwired outcome output, an unknown node config field, or a
// missing service slot ran green here while validate and boot both failed.
func TestTestCmd_FailsOnProjectThatDoesNotValidate(t *testing.T) {
	root := testRootCmd(newTestCmd())
	root.SetArgs([]string{"test", "--config", "../../testdata/test-cmd-invalid-project"})

	err := root.Execute()
	require.Error(t, err, "test must fail on a project that cannot boot")
	assert.Contains(t, err.Error(), "outcome output")
}
```

- [ ] **Step 6: Run both new tests and verify the split — characterization green, regression red**

```bash
go test ./cmd/noda/ -run 'TestValidateProject' -v 2>&1 | tail -20
go test ./cmd/noda/ -run 'TestTestCmd_FailsOnProjectThatDoesNotValidate' -v 2>&1 | tail -20
```

Expected: the two `TestValidateProject_*` tests PASS. `TestTestCmd_FailsOnProjectThatDoesNotValidate` FAILS with "test must fail on a project that cannot boot" — the command returned `nil` because it ran the fixture's passing test suite. That failure is the bug, reproduced.

- [ ] **Step 7: Wire `validateProject` into `newTestCmd`**

In `cmd/noda/main.go`, inside `newTestCmd`, insert after the `config.ValidateAll` error check (which ends with the closing brace of `if len(errs) > 0 { ... }`, currently line 197) and before the `// Load test suites` comment:

```go
			// The same startup validation `noda validate` runs. Without it a
			// test run executes workflows that validate and boot both reject,
			// so green tests would not imply the project starts (#444).
			if err := validateProject(rc); err != nil {
				return err
			}
```

- [ ] **Step 8: Run the tests to verify they pass**

```bash
go test ./cmd/noda/ -run 'TestValidateProject|TestTestCmd' -v 2>&1 | tail -25
```

Expected: all PASS, including the pre-existing `TestTestCmd_RunsAgainstTestdata`, `TestTestCmd_WithWorkflowFilter`, and `TestTestCmd_VerboseMode` (all of which use `testdata/valid-project`, which passes the dry-run).

- [ ] **Step 9: Mutation check — prove the new test is load-bearing**

Comment out the four lines added in Step 7, re-run, confirm red, then restore them.

```bash
go test ./cmd/noda/ -run 'TestTestCmd_FailsOnProjectThatDoesNotValidate' 2>&1 | tail -5
```

Expected while mutated: FAIL. After restoring: PASS. If it passes while mutated, the test is not testing the fix — stop and fix the test.

- [ ] **Step 10: Commit**

```bash
git rev-parse --show-toplevel   # must end in fix-444-test-validation-parity
git add testdata/test-cmd-invalid-project cmd/noda/validate_test.go cmd/noda/commands_test.go cmd/noda/main.go
git commit -m "fix(cmd)!: \`noda test\` runs the startup dry-run before executing tests

BREAKING: a project whose config does not pass \`noda validate\` now fails
\`noda test\` instead of running it. Previously the test runner skipped the
startup dry-run, so a workflow with an unwired outcome output, an unknown
node config field, or a missing service slot ran green while validate and
boot both rejected it — a green test run did not imply the project starts.

Closes #444.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: Restore the shipped-project test helper's mirror claim

`runProjectTestSuites` documents itself as mirroring `newTestCmd` exactly. Task 2 made that false. Fixing it also makes every shipped example and the `noda init` scaffold enforce the new rule for free.

**Files:**
- Modify: `cmd/noda/shipped_tests_test.go:18-39` (`runProjectTestSuites`)

**Interfaces:**
- Consumes: `validateProject(rc *config.ResolvedConfig) error` from Task 1.
- Produces: nothing new.

- [ ] **Step 1: Add the `validateProject` call to the helper**

In `cmd/noda/shipped_tests_test.go`, inside `runProjectTestSuites`, insert immediately after the `config.ValidateAll` block (the `require.Empty(t, errs, ...)` on line 25) and before `nodatesting.LoadTests`:

```go
	require.NoError(t, validateProject(rc),
		"project must pass startup validation before its tests can run")
```

- [ ] **Step 2: Update the helper's doc comment so the mirror claim stays true**

Replace the existing comment above `runProjectTestSuites` (lines 14-17) with:

```go
// runProjectTestSuites runs every workflow test suite a project ships through
// the real `noda test` runner and asserts each case passes. It mirrors exactly
// what the test command does (main.go newTestCmd) — including the startup
// validation the command runs before executing anything (#444) — so a green
// run here means a user running `noda test` in that directory sees green too.
// When newTestCmd gains a step, add it here.
```

- [ ] **Step 3: Run every test that uses the helper**

```bash
go test ./cmd/noda/ -run 'TestScaffoldedProjectPassesItsOwnTests|TestShippedExamplesPassTheirTests' -v 2>&1 | tail -30
```

Expected: PASS, including subtests for `auth-demo`, `init-example`, `realtime-collab`, `realworld`, `rest-api`, `saas-backend`. All six already pass the dry-run under `TestShippedProjectsValidate`, so this should be green on the first run. If one fails, that is a real pre-existing defect in that example — report it rather than weakening the helper.

- [ ] **Step 4: Run the whole affected surface**

```bash
go build ./... && go vet ./... && go test ./cmd/noda/... ./internal/registry/... ./internal/testing/... 2>&1 | tail -20
```

Expected: all `ok`.

- [ ] **Step 5: Commit**

```bash
git rev-parse --show-toplevel   # must end in fix-444-test-validation-parity
git add cmd/noda/shipped_tests_test.go
git commit -m "test(cmd): shipped-project helper mirrors newTestCmd's validation

The helper documents itself as mirroring the test command exactly; adding
the startup validation to newTestCmd made that false. Restoring it makes
every shipped example and the noda init scaffold enforce the rule (#444).

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Documentation and CHANGELOG

**Files:**
- Modify: `docs/04-guides/testing-and-debugging.md` (the "Running Tests" callout, around line 302)
- Modify: `docs/01-getting-started/quick-start.md` (§7 "Validate and Test", around line 384)
- Modify: `CHANGELOG.md` (`[Unreleased]`)

**Interfaces:**
- Consumes: nothing. Documentation only.
- Produces: nothing.

- [ ] **Step 1: Document the converse in the testing guide**

`docs/04-guides/testing-and-debugging.md` currently opens "## Running Tests" with a blockquote whose first line is **"Validation does not run tests."** Append a new paragraph inside that same blockquote, after the existing `> In particular, the scaffolded ...` line:

```markdown
>
> **The reverse does hold:** `noda test` validates the project before it runs anything. It performs the same checks as `noda validate` — node config schemas, service slot references, service config schemas, edge outputs, and unwired outcome outputs — and stops with those errors without executing a single test. A project that cannot boot cannot be tested.
```

- [ ] **Step 2: Note the behavior in the quick start**

`docs/01-getting-started/quick-start.md` §7 "Validate and Test" ends with a paragraph beginning "`noda validate` checks config schemas, ...". Add one sentence immediately after that paragraph:

```markdown
`noda test` runs those same checks first and stops on failure, so it never executes a workflow that would be rejected at boot.
```

- [ ] **Step 3: Add the CHANGELOG entry**

`CHANGELOG.md` already has exactly one `### Changed` heading in the whole file, and it is the one under `## [Unreleased]` (directly after the `### Added` list, before `### Fixed`). Add this bullet as the **first** entry of that existing section — do not create a second `### Changed`; the repo has had duplicate-subsection cleanups before.

```markdown
- **BREAKING:** `noda test` now runs the same startup validation as `noda validate` before executing any test, and fails with those errors instead of running the suite (#444). Previously it ran only config-file validation, so the startup dry-run — node config schemas, service slot references, service config schemas (#376), edge outputs (#379), and the unwired-outcome-output rule (#442) — never applied to a test run. A workflow that `noda validate` and boot both rejected still reported a green `noda test`, which inverts the expectation that tests are the stricter gate. Migration: run `noda validate` and fix what it reports; any project that already validates is unaffected.
```

- [ ] **Step 4: Verify the docs claims are true, not just plausible**

```bash
grep -n "The reverse does hold" docs/04-guides/testing-and-debugging.md
grep -n "runs those same checks first" docs/01-getting-started/quick-start.md
grep -c "^### Changed" CHANGELOG.md
go run ./cmd/noda test --config ./testdata/test-cmd-invalid-project; echo "exit=$?"
```

Expected: the first two greps each print one line. `grep -c "^### Changed"` must print exactly `1` — it was 1 before this edit, so any other number means a duplicate section was created. The `noda test` run must print the bootstrap errors and exit non-zero, matching what the docs now promise.

- [ ] **Step 5: Commit**

```bash
git rev-parse --show-toplevel   # must end in fix-444-test-validation-parity
git add docs/04-guides/testing-and-debugging.md docs/01-getting-started/quick-start.md CHANGELOG.md
git commit -m "docs: \`noda test\` validates the project before running tests (#444)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Final Verification

- [ ] **Full build, vet, and test sweep**

```bash
go build ./... && go vet ./... && go test ./... 2>&1 | grep -v "^ok" | head -30
```

Expected: no output past the filter (every package `ok`), or only known-flaky entries. Known flakes on this repo, which are *not* caused by this change: `TestWatcher_Debounce` (internal/devmode, #347) and `TestEventHub_NoGoroutineLeakOnUnsubscribe` under `-count>1` (internal/trace, #416).

- [ ] **Confirm the integration-tagged gates still compile**

The cookbook coverage gate only builds under a tag; a change to `cmd/noda` test files can break it without any untagged test noticing.

```bash
go vet -tags integration ./... 2>&1 | head -20
```

Expected: no output.

- [ ] **End-to-end check of the actual user story**

```bash
go run ./cmd/noda validate --config ./testdata/test-cmd-invalid-project; echo "validate exit=$?"
go run ./cmd/noda test --config ./testdata/test-cmd-invalid-project; echo "test exit=$?"
go run ./cmd/noda test --config ./testdata/valid-project; echo "valid test exit=$?"
```

Expected: validate exits non-zero naming the unwired `exists` output; `test` on the same fixture exits non-zero with the *same* errors and runs no tests; `test` on `valid-project` still exits 0 and reports its passing cases.
