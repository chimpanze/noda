# Auth Anti-Enumeration (Tranche Auth) — Design

Date: 2026-07-06
Source: `REVIEW-FINDINGS-2026-07-05.md` — auth-1, auth-2 (both Medium, security).
Branch/worktree (planned): `feat/auth-anti-enumeration` in `.worktrees/auth-anti-enumeration`, off `main`.

## Why

The two findings let an unauthenticated attacker enumerate which email addresses have accounts — the exact
attack the sibling reset/verification flows already try to prevent with uniform "if that account exists"
messages. Both defects live in the **scaffolded default templates** (`cmd/noda/auth_templates/`), not in the
auth plugin runtime, so the fix is to the generated workflow/route/test config a user gets from `noda auth init`.

- **auth-1** — the register flow discloses account existence four ways at once: a new email returns `201` +
  a `Set-Cookie` session + a verification email; an existing email returns `400 {"error":"registration failed"}`,
  no cookie, no email. Status, body, cookie presence, and the email side-effect all differ.
- **auth-2** — `request-password-reset` and `resend-verification` return a uniform body, but the *code path*
  diverges: a known email runs `create_token` + a **synchronous SMTP `email.send`** (tens–hundreds of ms) before
  responding; an unknown email returns in ~1 ms. The response-time gap re-opens enumeration despite the uniform body.

**Scope:** auth-1 + auth-2 only (the anti-enumeration theme). auth-3 (reset-token burned before password
validation) and auth-4 (login timing oracle on argon2 param drift) are separate findings, out of scope.

## Approach (decided with the user)

- **auth-1 → verification-first registration.** Both outcomes become indistinguishable: identical `200` body,
  no session cookie, and both branches send an email. Registration no longer auto-logs-in — the user verifies
  then logs in. Secure-by-default; users can loosen the generated template.
- **auth-2 → constant-time-to-deadline response.** Every branch converges and responds at a fixed deadline
  `T` (500 ms) measured from request start, absorbing the known branch's SMTP latency. **No new infrastructure**
  — the scaffold stays DB + email only (no Redis/worker). A doc note recommends async email (worker) for a hard
  guarantee in high-security deployments.

## Verified facts (mechanism)

- `util.timestamp` supports `format: "unix_ms"` → an `int64` epoch-milliseconds scalar
  (`plugins/core/util/timestamp.go`, `now.UnixMilli()`).
- The expression engine is segment-based: a lone `"{{ x }}"` returns the typed value, while a **mixed**
  `"{{ expr }}ms"` string is evaluated segment-by-segment and concatenated with `fmt.Fprintf("%v", …)` into a
  **string** — e.g. `"{{ 437 }}ms"` → `"437ms"` (`internal/expr/evaluator.go`). `time.ParseDuration("437ms")` is valid.
- `plugin.ResolveString(nCtx, config, key)` (`internal/plugin/resolve.go`) resolves a config string's expression
  against the execution context and returns the resulting string.
- **Blocker in `util.delay`:** it parses `timeout` at *build* time in `newDelayExecutor` and `Execute` uses the
  captured `time.Duration`; it never resolves per-request config. So a templated/computed `timeout` cannot work
  today. This is the one supporting change (Unit 0 below).
- Node executors receive the **raw** (unresolved) `node.Config` both at construction and in `Execute`
  (`internal/engine/dispatch.go:35,68`); each node resolves its own templates. `util.log` uses `nCtx.Resolve`;
  `email.send` uses `plugin.ResolveString`.
- Workflow tests (`cmd/noda/auth_templates/tests/*.json`) mock each node's output and assert on node outputs; a
  branch is selected with `"output_name"`. Convergence to a single terminal `respond` node lets both branches
  assert the *same* node + status — encoding the anti-enumeration property directly.

## Design

### Unit 0 — `util.delay` resolves `timeout` per request (supporting change)

In `plugins/core/util/delay.go`, make `Execute` resolve the duration from the per-request config rather than the
build-time value:

- In `Execute`, call `plugin.ResolveString(nCtx, config, "timeout")` to get the resolved string, then
  `time.ParseDuration` it; on parse/resolve error return a clear `util.delay: invalid duration %q` error
  (an `error` output-eligible failure, matching today).
- Keep `newDelayExecutor` for construction, but it no longer needs to pre-parse for correctness. To preserve the
  existing behavior where an invalid *static* duration is reported, the Execute-time parse now covers both static
  and templated cases uniformly. A static `"500ms"` resolves to `"500ms"` and parses exactly as before
  (backward compatible).
- Add the import `github.com/chimpanze/noda/internal/plugin` to `delay.go` (path per `email/send.go`). No import
  cycle: `internal/plugin` does not import `plugins/core/util`, and sibling core plugins (`plugins/core/sse`,
  `plugins/core/response`) already import `internal/plugin`.

Test (`plugins/core/util/delay_test.go`): a delay whose `timeout` is a templated expression resolving to a
duration string (e.g. input/context yielding `"20ms"`) actually waits ~that long; a static `"10ms"` still works;
an unresolvable/invalid duration returns the error output. Keep waits tiny (≤ 50 ms) for fast tests.

### Unit 1 — Register: verification-first, indistinguishable (auth-1)

Rewrite `cmd/noda/auth_templates/workflows/auth.register.json.tmpl`:

- Nodes: `create_user` (unchanged); on the default success output → `verify_token` (`auth.create_token`,
  `purpose: verify_email`) → `send_verify_email` (`email.send` to `{{ nodes.create_user.email }}`) → `respond`.
- On `create_user`'s `"exists"` output → `send_exists_email` (`email.send` to `{{ input.email }}`, subject e.g.
  "Someone tried to register with your email", body advising to log in or reset) → the **same** `respond` node.
- Single terminal `respond` (`response.json`): `status: 200`, body `{ "message": "Check your email to continue" }`,
  **no `cookies`**. Remove the `session` (`auth.create_session`) node and the old `respond_exists`.
- Both branches send exactly one email and end at the identical response — status, body, and cookie-absence match,
  and timing is symmetric (both do one SMTP send).

Edges:
```
create_user            -> verify_token
create_user  (exists)  -> send_exists_email
verify_token           -> send_verify_email
send_verify_email      -> respond
send_exists_email      -> respond
```

Route (`routes/auth.register.json`): unchanged unless it references the removed session/cookie — verify it does
not assert a 201/cookie.

### Unit 2 — Reset & resend: constant-time-to-deadline (auth-2)

Both `auth.request-password-reset.json.tmpl` and `auth.resend-verification.json.tmpl` gain a shared timing
scaffold and converge to one uniform response.

Shared pattern (T = 500):
- `start_ts` = `util.timestamp` `{ "format": "unix_ms" }` — the **first** node.
- Each outcome branch does its real work (or nothing) and then flows into a shared `now_ts` = `util.timestamp`
  `{ "format": "unix_ms" }`.
- `now_ts` → `pad` (`util.delay`) → `respond`.
- `pad.config.timeout` =
  `"{{ (nodes.start_ts + 500) > nodes.now_ts ? (nodes.start_ts + 500 - nodes.now_ts) : 0 }}ms"`
  (deadline = start + 500 ms; wait the remainder, clamped at 0). `nodes.start_ts` / `nodes.now_ts` are the
  scalar `int64` outputs of the timestamp nodes. The implementer must confirm the exact scalar reference form
  against a run (`nodes.start_ts` vs a wrapped field) and adjust if needed.
- Single `respond` (`response.json`, `status: 200`) with the existing uniform body message.

`request-password-reset` edges:
```
start_ts                 -> find_user
find_user                -> reset_token
find_user   (not_found)  -> now_ts
reset_token              -> send_reset_email
send_reset_email         -> now_ts
now_ts                   -> pad
pad                      -> respond
```

`resend-verification` edges (three outcomes converge):
```
start_ts                    -> find_user
find_user                   -> check_unverified
find_user     (not_found)   -> now_ts
check_unverified (then)     -> verify_token
check_unverified (else)     -> now_ts
verify_token                -> send_verify_email
send_verify_email           -> now_ts
now_ts                      -> pad
pad                         -> respond
```

Result: known, unknown, and already-verified requests all reach `respond` at ~500 ms; the SMTP round-trip on the
known path is absorbed by the deadline. Residual leak only if actual SMTP exceeds T (documented; raise T or use
async email).

### Unit 3 — Scaffolded tests + docs + changelog

- Update `tests/test-auth-register.json`: both cases (new email, existing email via `output_name: "exists"`)
  now assert the **same** `respond` node at `status: 200`; mock `send_verify_email` / `send_exists_email`; remove
  the `session` / `respond_exists` / 201 expectations.
- Update `tests/test-auth-request-password-reset.json` and `tests/test-auth-resend-verification.json`: mock
  `start_ts`, `now_ts`, and `pad` (so tests stay fast and deterministic — no real 500 ms wait); every branch
  asserts the single `respond` node at `status: 200`.
- Add a note in the generated auth README / relevant doc: the reset/resend flows pad to a fixed 500 ms deadline
  to resist timing enumeration; registration is verification-first (no auto-login); for a hard timing guarantee,
  move `email.send` to an async worker consuming an emitted event.
- `CHANGELOG.md` "Security" entry summarizing both fixes.

## Testing / gate

- Unit 0: `go test -race ./plugins/core/util/...`.
- Units 1–3: the scaffolded workflow tests must pass via the test runner. Additionally, scaffold a throwaway
  project in a temp dir (`noda auth init` against a minimal `noda.json` with a db + email service) and run
  `noda validate` + the generated tests to prove the templates produce valid, passing config end to end. If a CLI
  scaffold run is impractical in the harness, at minimum `noda validate` the rendered templates.
- Full gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./plugins/core/util/... ./cmd/noda/...`
  (plus the auth template test runner). No new lint issues from this branch.

## Mechanics

- Worktree `.worktrees/auth-anti-enumeration`, branch `feat/auth-anti-enumeration` off `main`.
- Subagent-driven execution per task: implementer → spec-compliance reviewer → code-quality reviewer; final
  whole-branch review on the most capable model (this is security-sensitive).
- Spec + plan force-added to the branch.
- At merge: add a "Shipped 2026-07-06" note for auth-1/auth-2 to `REVIEW-FINDINGS-2026-07-05.md` (on review
  PR #262's branch).

## Out of scope

auth-3, auth-4 (separate findings). No change to the auth plugin runtime, the login flow, or the session model.
No new service dependency (no Redis/worker) — deliberately, to keep `noda auth init` DB+email only. The async-email
worker is documented as the high-security upgrade, not implemented here.
