# Auth fixture refresh + scheduler tick-time locking — design (2026-07-07)

Two follow-up issues picked by the user after the quick-wins tranche merged:
**#291** (testdata/auth fixture drift) and **#283** (scheduler distributed
lock keyed on wall clock). Independent changes, one tranche/PR (established
bundling precedent). Grounded against `main` (`0d47510`).

---

## 1. #291 — regenerate `testdata/auth`, adapt e2e, add a drift guard

**Problem.** The committed fixture was generated at #247 and lags the shipped
templates by three tranches: no `auth.resend-verification` workflow (#289),
no constant-time pads in request-password-reset / resend-verification (#289)
or login (#290), and reset-password still uses the removed two-node
`consume → set_password(user_id)` chain (#290). The `-tags=integration` e2e
(`plugins/auth/engine_e2e_integration_test.go`) therefore exercises stale
workflow shapes; the new template shapes have no real-engine coverage.

**Regeneration.** The fixture's `noda.json` names its services `main-db` +
`email` (NOT `mailer` as `writeMinimalProject` does), and `[[.EmailService]]`
is rendered into the workflow files — so regeneration must preserve the
fixture's own `noda.json`. Procedure: (1) strip the `auth` service entry from
`testdata/auth/noda.json` (`runAuthInit` errors on `services.auth already
exists`; it re-adds it), (2) delete the auth-owned files (workflows/routes/
tests `auth.*`/`test-auth-*`, `migrations/*_auth_tables.*` — collision check
aborts otherwise), (3) `go run ./cmd/noda auth init --dir testdata/auth`
(flag confirmed: `--dir`, default `.`). Migration filenames carry a
generation timestamp — expected to change; `migrate.Up` doesn't care.
`middleware`/`middleware_presets` in the fixture `noda.json` are left alone.

**E2E adaptation** (all in `plugins/auth/engine_e2e_integration_test.go`):
- `register` subtest: the new flow is verification-first — expect `200`
  `{"message":"Check your email to continue"}` from `respond`, **no** token
  in the body, **zero** rows in `auth_sessions`, user row created,
  verification email captured. Add the anti-enumeration half: registering
  the same email again returns the byte-identical `200` from
  `respond_exists` and sends the "already registered" notice.
- `login` subtests: node names unchanged (`respond`/`respond_invalid`); the
  invalid path now walks `now_ts_invalid → pad_invalid`, adding ~500 ms wall
  per invalid login (three across the suite: wrong-password, old-password
  after reset ×1, plus pads in request-password-reset ×2). Accepted — this
  is an integration-tagged suite.
- `request_password_reset_enumeration`: template still has distinct
  `respond_sent`/`respond_unknown` nodes with identical bodies — the
  existing byte-identical assertion stands. New: assert both runs take
  ≥ ~400 ms (the pad actually padding, loose lower bound to avoid flake)…
  actually assert ≥400ms only on the *unknown* branch (the fast one that the
  pad exists to slow down); the known branch is naturally slower.
- `reset_password`: workflow is now the single atomic node; `respond` /
  `respond_invalid` names unchanged. Add the invalid-token case (reuse the
  consumed token → `respond_invalid` 400) since `consume` no longer exists
  as a separate node.
- New `resend_verification` subtest is NOT added to `TestEngineE2E_AuthFlows`
  (scope: adapt, don't expand) — but `TestEngineE2E_ScaffoldedTestSuites`
  automatically picks up the new `tests/test-auth-resend-verification.json`
  since it globs `tests/*.json`.

**Backward-compat note.** The old fixture doubled as the proof that
`set_password`'s `user_id` mode still works. That proof lives in unit tests
(`TestSetPassword` drives user_id mode directly); nothing is lost. State
this in the PR.

**Drift guard** (the "can't rot again" piece): new test in
`cmd/noda/auth_fixture_drift_test.go` (NO integration tag — runs in normal
CI): write a minimal project whose services match the FIXTURE's names
(`main-db` db + `email` email service, so `[[.EmailService]]` renders
identically), run `runAuthInit` on it, then compare against
`../../testdata/auth`:
- `workflows/auth.*.json`, `routes/auth.*.json`, `tests/test-auth-*.json`:
  identical file sets, byte-identical contents.
- `migrations/*_auth_tables.up.sql` / `.down.sql`: content-equal, filename
  timestamp prefix ignored.
- `noda.json` NOT compared (the fixture's carries project-level bits the
  guard doesn't own).
Failure message must say: "testdata/auth lags the auth templates — regenerate
it (see the comment at the top of this test)" with the regen recipe in the
test's doc comment.

## 2. #283 — key the scheduler's distributed lock on the cron tick time

**Problem** (`internal/scheduler/runtime.go`): `runJob` builds the lock key
from `time.Now()` at entry. Two instances firing the same logical tick but
straddling a second boundary (A at `:00.990`, B GC-delayed to `:01.010`)
compute different keys and **both run**. Second-truncation absorbs ~1 s of
dispatch jitter where minute-truncation absorbed ~60 s.

**Fix.** Key on the cron-scheduled fire time. robfig/cron v3's run loop sets
`entry.Prev = entry.Next` for the firing entry *before* `startJob` launches
the job goroutine, so at job-run time `Entry(id).Prev` IS this tick's
scheduled time — all instances that agree on the schedule agree on it,
regardless of dispatch jitter. **Verify this against the vendored source**
(`~/go/pkg/mod/github.com/robfig/cron/v3@*/cron.go`) before relying on it
(working-conventions rule).

Mechanics:
- `Start`: capture the `cron.EntryID` returned by `AddFunc` into a variable
  the job closure reads: `var eid cron.EntryID; eid, err = r.cron.AddFunc(
  spec, func() { r.runJob(sc, r.scheduledFireTime(eid)) })`. Assignment
  happens before `r.cron.Start()`, so no fire can observe it unset.
- New helper `scheduledFireTime(id cron.EntryID) time.Time`: returns
  `r.cron.Entry(id).Prev`, falling back to `time.Now()` when zero (defensive;
  also keeps direct-invocation semantics sane). `cron.Entry()` from a job
  goroutine is safe: v3 jobs run on their own goroutines while the run loop
  services snapshot requests.
- `runJob(sc ScheduleConfig, fireTime time.Time)`: use `fireTime` **only**
  for `scheduleLockKey`; `start`/history/durations keep `time.Now()`.
  Update the direct test callers to pass an explicit time.
- Comment the residual honestly: for a 1 s schedule delayed ≥1 s, `Prev` may
  already be the next tick — boundary sensitivity shrinks to pathological
  cases instead of routine GC jitter; inherent to time-bucketed locking.

**Tests** (`internal/scheduler/runtime_test.go`):
- Polarity/regression: two runtimes sharing one mock lock service, `runJob`
  called on each with the **same** `fireTime` but ≥1.1 s apart in wall time
  (or just different wall instants — wall clock no longer participates):
  second must record `Skipped: true`. Against the old code (key from
  `time.Now()`), a >1 s wall gap guarantees different keys → both ran, so
  the test fails pre-fix.
- `scheduledFireTime`: after a real 1 s-cron fire, the captured lock key
  matches `Entry.Prev`, not the (later) wall clock — or simpler, unit-assert
  the helper returns `Prev` when set and now-ish when zero.
- Existing `TestDistributedLock_*` tests updated for the new `runJob`
  signature.

## Cross-cutting

- Branch `fixture-lock`, worktree `.worktrees/fixture-lock`, off `main`.
  Spec+plan `git add -f`'d per convention.
- CHANGELOG `[Unreleased]`: `### Fixed` entry for #283 (lock keyed on tick
  time); `### Changed`/test-infra note for #291 (fixture regenerated +
  drift-guarded; e2e now covers the anti-enumeration templates).
- PR body: `Fixes #291`, `Fixes #283`.
- Gates: gofmt, vet, golangci-lint, `go test -race ./internal/scheduler/
  ./cmd/noda/ ./plugins/auth/`, plus `go test -tags=integration
  ./plugins/auth/` (sqlite-backed, no external services needed) — the e2e
  suite is the core deliverable of #291 and MUST run locally.
- Not in scope: adding a resend-verification e2e subtest beyond the suite
  runner's automatic pickup; #284's history asymmetry; releasing the lock
  early (per-fire TTL semantics unchanged).
