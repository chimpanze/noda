# Fixture Refresh + Tick-Time Locking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close #291 (regenerate `testdata/auth`, adapt the auth e2e, add a drift guard) and #283 (scheduler distributed lock keyed on the cron tick time) per `docs/superpowers/specs/2026-07-07-fixture-lock-design.md`.

**Architecture:** Two independent changes. #283 threads the cron-scheduled fire time (`Entry.Prev`) into `runJob` and uses it only for the lock key. #291 regenerates the fixture via the CLI against its own `noda.json` (service names `main-db`/`email`), rewrites the stale e2e subtests for the verification-first/padded/atomic template shapes, and pins template↔fixture equality with a non-integration drift-guard test.

**Tech Stack:** Go, robfig/cron/v3, sqlite-backed integration tests (no external services), testify.

## Global Constraints

- Branch `fixture-lock`, worktree `.worktrees/fixture-lock`, off `main` (`0d47510`).
- Per-commit gates: gofmt clean on touched files, `go vet ./...`, package tests green.
- Final gates additionally: `golangci-lint run`, `go test -race ./internal/scheduler/ ./cmd/noda/`, `go test -tags=integration ./plugins/auth/` (the #291 deliverable), `go build ./...`.
- Verify the robfig/cron `Prev` claim against vendored source before relying on it (working-conventions rule).
- Commits end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.

---

### Task 1: #283 — scheduler lock keyed on cron tick time

**Files:**
- Modify: `internal/scheduler/runtime.go` (`Start` ~L127, new `scheduledFireTime`, `runJob` signature ~L200, lock-key site ~L253)
- Test: `internal/scheduler/runtime_test.go`

**Interfaces:**
- Produces: `func (r *Runtime) runJob(sc ScheduleConfig, fireTime time.Time)`; `func (r *Runtime) scheduledFireTime(id cron.EntryID) time.Time`.

- [ ] **Step 1: Verify the cron claim.** Read `~/go/pkg/mod/github.com/robfig/cron/v3@*/cron.go` `run()`: confirm `e.Prev = e.Next` is assigned before `startJob` for the firing entry, and that `Entry()`/`Entries()` is safe to call from a job goroutine while running. If either does not hold, STOP and redesign (fallback: compute the tick as `sched.Next(start.Add(-jitterWindow))` — do not improvise silently).

- [ ] **Step 2: Write the failing polarity test** (append to `runtime_test.go`; adapt helper names to the file's existing mock-lock harness after reading it):

```go
// The distributed lock must key on the cron-scheduled tick time, not the
// wall clock at runJob entry: two instances firing the same logical tick
// but straddling a second boundary must compute the SAME key, so exactly
// one runs (#283). With the old time.Now() keying, wall instants >1s apart
// yielded different keys and both instances ran.
func TestDistributedLock_SameTickAcrossWallSeconds_OneRuns(t *testing.T) {
	// Build two runtimes sharing one mock lock service and one schedule
	// (LockEnabled), mirroring TestDistributedLock_SecondInstanceSkips.
	// Then:
	tick := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	rA.runJob(sc, tick)                       // instance A, on time
	rB.runJob(sc, tick)                       // instance B, same tick, any wall time later
	// Assert: A's history has a real run; B's history records Skipped: true.
}
```

(The test body must use the file's actual construction helpers — read `TestDistributedLock_SecondInstanceSkips` (~L336) first and mirror it exactly; the snippet above fixes the *assertions*, not the plumbing.)

- [ ] **Step 3:** Run — compile FAIL (`runJob` has no fireTime param yet).

- [ ] **Step 4: Implement.**

(a) `runJob` signature + lock key:

```go
// runJob executes a single scheduled job with optional distributed locking.
// fireTime is the cron-scheduled tick instant (Entry.Prev) — used ONLY for
// the distributed-lock key, so instances that straddle a wall-clock second
// boundary while handling the same tick still agree on the key. Durations
// and history keep the actual wall clock.
func (r *Runtime) runJob(sc ScheduleConfig, fireTime time.Time) {
```

and at ~L253: `lockKey := scheduleLockKey(sc.ID, fireTime)` (delete the `now := start` alias if it becomes unused — check other uses first).

(b) helper + registration in `Start`:

```go
		var entryID cron.EntryID
		entryID, err = r.cron.AddFunc(spec, func() {
			r.runJob(sc, r.scheduledFireTime(entryID))
		})
```

```go
// scheduledFireTime returns the cron-scheduled instant of the tick being
// handled: robfig/cron sets Entry.Prev to the fire time before launching
// the job goroutine. Falls back to time.Now() if unset (defensive; direct
// callers in tests pass explicit times instead). Residual: for a schedule
// whose interval is shorter than its dispatch delay (e.g. a 1s schedule
// delayed >1s), Prev may already belong to the next tick — boundary
// sensitivity shrinks from routine GC jitter to pathological overload,
// and is inherent to time-bucketed locking.
func (r *Runtime) scheduledFireTime(id cron.EntryID) time.Time {
	if r.cron != nil {
		if e := r.cron.Entry(id); !e.Prev.IsZero() {
			return e.Prev
		}
	}
	return time.Now()
}
```

(c) update every direct `runJob(` caller in tests to pass an explicit time (`time.Now()` where the key doesn't matter, a fixed tick where it does).

- [ ] **Step 5:** `go test ./internal/scheduler/ -race` — all green, new test passes. Also update the `scheduleLockKey` doc comment and the "Do NOT release the lock" comment (~L301) if they reference "runJob entry time".
- [ ] **Step 6:** Commit `fix(scheduler): key the distributed lock on the cron tick time, not time.Now() (#283)`.

### Task 2: #291a — regenerate the fixture

**Files:**
- Modify: `testdata/auth/noda.json` (strip+re-add `auth` service via the CLI), `testdata/auth/{workflows,routes,tests,migrations}/*` (regenerated)

- [ ] **Step 1:** In the worktree: edit `testdata/auth/noda.json` to remove the `"auth"` service entry; delete the auth-owned files:

```bash
rm testdata/auth/workflows/auth.*.json testdata/auth/routes/auth.*.json \
   testdata/auth/tests/test-auth-*.json testdata/auth/migrations/*_auth_tables.*.sql
go run ./cmd/noda auth init --dir testdata/auth
```

- [ ] **Step 2:** Inspect `git status`/`git diff` on testdata/auth: expect new `auth.resend-verification.json` (workflow/route/test), pads in login + request-password-reset + resend-verification workflows, single-node reset-password, new migration timestamp, `auth` service restored in noda.json (byte-diff should show only ordering/regeneration effects — verify the service block matches what was removed).
- [ ] **Step 3:** Run the pre-adaptation e2e to see the EXPECTED failures: `go test -tags=integration ./plugins/auth/ -run TestEngineE2E -v` — register subtest must fail (verification-first), scaffolded suites must pass (they ship with the templates). Anything else failing → investigate before proceeding.
- [ ] **Step 4:** Commit `test(auth): regenerate testdata/auth fixture from current templates (#291)` (fixture only — e2e adapts next; note in the commit body that TestEngineE2E_AuthFlows is red at this commit and green at the next).

Actually: to keep every commit green, fold Tasks 2+3 into ONE commit — do Step 3's run, then Task 3, then commit both together. (Checkbox discipline: complete Task 3 before committing.)

### Task 3: #291b — adapt the e2e subtests

**Files:**
- Modify: `plugins/auth/engine_e2e_integration_test.go`

- [ ] **Step 1: register subtest** — replace the 201+token+session assertions:

```go
	t.Run("register", func(t *testing.T) {
		wf := loadWorkflow(t, "auth.register.json", "auth-register")
		execCtx := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{
			"email":    "alice@example.com",
			"password": "password123",
		})

		// Verification-first (#289): a generic 200, no session, no token.
		resp := httpOutput(t, execCtx, "respond")
		assert.Equal(t, 200, resp.Status)
		body, ok := resp.Body.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Check your email to continue", body["message"])
		assert.NotContains(t, body, "token")

		var userCount, sessionCount int64
		require.NoError(t, gdb.Table("auth_users").Where("email = ?", "alice@example.com").Count(&userCount).Error)
		assert.Equal(t, int64(1), userCount)
		require.NoError(t, gdb.Table("auth_sessions").Count(&sessionCount).Error)
		assert.Equal(t, int64(0), sessionCount, "verification-first register must not create a session")

		sent := mb.last()
		assert.Equal(t, "alice@example.com", sent.To)
		require.NotEmpty(t, extractToken(t, sent.Body))

		// Anti-enumeration: registering the same email again returns the
		// byte-identical body from respond_exists and sends the notice.
		execCtx2 := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{
			"email":    "alice@example.com",
			"password": "password123",
		})
		resp2 := httpOutput(t, execCtx2, "respond_exists")
		assert.Equal(t, 200, resp2.Status)
		exists, err := json.Marshal(resp2.Body)
		require.NoError(t, err)
		fresh, err := json.Marshal(resp.Body)
		require.NoError(t, err)
		assert.JSONEq(t, string(fresh), string(exists))
		assert.Equal(t, "Account already registered", mb.last().Subject)
		require.NoError(t, gdb.Table("auth_users").Where("email = ?", "alice@example.com").Count(&userCount).Error)
		assert.Equal(t, int64(1), userCount, "duplicate register must not create a second user")
	})
```

CAREFUL: `verify_email` (next subtest) extracts the verification token from `mb.last()` — after the duplicate-register the last mail is the "already registered" notice with NO token. Capture the verification token into a variable inside `register` (subtests share the closure) or have `verify_email` use `mb.sent[len-2]`. Cleanest: add `var verifyToken string` at the `TestEngineE2E_AuthFlows` top; `register` sets it from the first send; `verify_email` uses it.

- [ ] **Step 2: request_password_reset_enumeration** — keep the byte-identical assertion (node names `respond_sent`/`respond_unknown` unchanged); add the pad check on the fast branch:

```go
		unknownStart := time.Now()
		unknownCtx := runWorkflow(t, svcReg, nodeReg, wf, map[string]any{"email": "nobody@example.com"})
		unknownElapsed := time.Since(unknownStart)
		...
		// The unknown branch is the fast path the pad exists to slow down
		// (#289): without util.delay it returns in ~1ms.
		assert.GreaterOrEqual(t, unknownElapsed, 400*time.Millisecond,
			"unknown-email branch must be padded to the fixed deadline")
```

- [ ] **Step 3: reset_password** — workflow is now atomic token-mode (#290); `respond`/`respond_invalid` unchanged. After the successful reset, add the reuse case:

```go
		// The token was consumed atomically; reuse must hit respond_invalid.
		reuseCtx := runWorkflow(t, svcReg, nodeReg, resetWF, map[string]any{
			"token":    resetToken,
			"password": "anotherpassword789",
		})
		assert.Equal(t, 400, httpOutput(t, reuseCtx, "respond_invalid").Status)
```

(Reuse runs BEFORE the login re-checks, or after — order irrelevant, put it right after the 200 assertion. Note the old-password login check in this subtest now walks the ~500ms pad — fine.)

- [ ] **Step 4:** `go test -tags=integration ./plugins/auth/ -v -run TestEngineE2E` — ALL green (expect ~2-3s added wall from pads). Then the full `go test -tags=integration ./plugins/auth/` and plain `go test ./plugins/auth/`.
- [ ] **Step 5:** Commit fixture + e2e together: `test(auth): regenerate testdata/auth and cover the anti-enumeration/atomic template shapes in e2e (#291)`.

### Task 4: #291c — drift guard

**Files:**
- Create: `cmd/noda/auth_fixture_drift_test.go`

- [ ] **Step 1: Write the test** (plain build, no integration tag):

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestAuthFixtureMatchesTemplates pins testdata/auth to the auth_templates:
// the committed fixture must be byte-identical to a fresh scaffold rendered
// with the fixture's own service names (main-db + email). If this fails you
// changed the templates without regenerating the fixture — regenerate:
//
//	1. remove the "auth" service entry from testdata/auth/noda.json
//	2. rm testdata/auth/workflows/auth.*.json testdata/auth/routes/auth.*.json \
//	      testdata/auth/tests/test-auth-*.json testdata/auth/migrations/*_auth_tables.*.sql
//	3. go run ./cmd/noda auth init --dir testdata/auth
//	4. re-run go test -tags=integration ./plugins/auth/ and adapt if needed
//
// (#291 — the fixture rotted silently for three tranches before this guard.)
func TestAuthFixtureMatchesTemplates(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "auth")

	// Scaffold with the fixture's service names so [[.EmailService]] renders
	// to "email" (scaffoldAuthProject's writeMinimalProject uses "mailer").
	dir := t.TempDir()
	services := map[string]any{
		"main-db": map[string]any{"plugin": "db", "config": map[string]any{"driver": "sqlite", "path": "data/app.db"}},
		"email":   map[string]any{"plugin": "email", "config": map[string]any{"host": "localhost", "port": 1025}},
	}
	b, err := json.MarshalIndent(map[string]any{"services": services}, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "noda.json"), b, 0644))
	require.NoError(t, runAuthInit(dir))

	// Byte-compare the rendered trees (file set + contents).
	for _, sub := range []string{"workflows", "routes", "tests"} {
		requireDirsEqual(t, filepath.Join(dir, sub), filepath.Join(fixture, sub))
	}

	// Migrations: content-equal, generation-timestamp prefix ignored.
	requireMigrationsEqual(t, filepath.Join(dir, "migrations"), filepath.Join(fixture, "migrations"))
}

func requireDirsEqual(t *testing.T, got, want string) {
	t.Helper()
	gotNames := dirFileNames(t, got)
	wantNames := dirFileNames(t, want)
	require.Equal(t, wantNames, gotNames, "file sets differ between %s and %s", got, want)
	for _, name := range wantNames {
		gotB, err := os.ReadFile(filepath.Join(got, name))
		require.NoError(t, err)
		wantB, err := os.ReadFile(filepath.Join(want, name))
		require.NoError(t, err)
		require.Equal(t, string(wantB), string(gotB),
			"testdata/auth/%s/%s lags the auth templates — regenerate the fixture (see this test's doc comment)", filepath.Base(want), name)
	}
}

var migrationTS = regexp.MustCompile(`^\d{14}_`)

func requireMigrationsEqual(t *testing.T, got, want string) {
	t.Helper()
	norm := func(dir string) map[string]string {
		out := map[string]string{}
		for _, name := range dirFileNames(t, dir) {
			b, err := os.ReadFile(filepath.Join(dir, name))
			require.NoError(t, err)
			out[migrationTS.ReplaceAllString(name, "TS_")] = string(b)
		}
		return out
	}
	require.Equal(t, norm(want), norm(got), "migrations differ (timestamps normalized) — regenerate the fixture")
}

func dirFileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}
```

CAREFUL: the fixture `tests/` and `routes/`/`workflows/` dirs contain ONLY auth files today, so whole-dir comparison works; if that assumption is wrong (check `ls`), filter to the auth-owned globs instead. Helper names must not collide with existing ones in package main (Grep first).

- [ ] **Step 2:** `go test ./cmd/noda/ -run TestAuthFixtureMatchesTemplates -v` — PASS against the freshly regenerated fixture. Polarity check: temporarily edit one fixture workflow byte → test FAILS with the regen message → revert.
- [ ] **Step 3:** Commit `test(cmd/noda): drift guard pinning testdata/auth to the auth templates (#291)`.

### Task 5: CHANGELOG + gates + PR

- [ ] **Step 1: CHANGELOG** `[Unreleased]`:
  - `### Fixed`: `- Scheduler distributed locking now keys each fire on the cron-scheduled tick time instead of the wall clock at dispatch, so two instances handling the same tick but straddling a second boundary (GC pause, load) can no longer both acquire "different" locks and double-run the job.`
  - `### Changed` (test infra): `- The committed testdata/auth fixture is regenerated from the current auth templates (verification-first register, constant-time pads, atomic reset) and a new drift-guard test fails CI whenever the templates change without a fixture regen; the auth engine e2e now exercises the hardened flows.`
- [ ] **Step 2: Gates:** gofmt on touched dirs, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/scheduler/ ./cmd/noda/`, `go test ./plugins/auth/`, `go test -tags=integration ./plugins/auth/`, `go build ./...`.
- [ ] **Step 3:** `git add -f` spec+plan; commit docs+CHANGELOG.
- [ ] **Step 4:** Whole-branch review via the code-review skill; fix Critical/Important, file follow-ups for Minors.
- [ ] **Step 5:** Push, PR `fix(scheduler)/test(auth): tick-time lock keying + auth fixture refresh with drift guard` with `Fixes #283`, `Fixes #291`; note the backward-compat proof relocation (set_password user_id mode covered by unit tests) and the ~2-3s added integration wall time. Wait for CI; merge on green (user pre-authorized? NO — ask/report unless told). Actually: per this session's pattern, report CI result; the user decides merge.
