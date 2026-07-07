# Review Closeout (Tranche G) — realtime-6, auth-3, auth-4

**Date:** 2026-07-07
**Findings:** realtime-6 (Low, correctness), auth-3 (Low, correctness), auth-4 (Low, security) from `REVIEW-FINDINGS-2026-07-05.md`
**Status:** Approved design

This is the final tranche of the 2026-07-05 clean-slate Go review. These three findings
are the only ones never bucketed into a shipped tranche (A–F + auth, PRs #263–#289).
They ship together as one combined tranche: one worktree, one spec/plan, three
independent tasks, one squash-merged PR.

## Pre-design findings from exploration

- **realtime-6 confirmed as written.** `ParticipantPermission` has 9 fields; the node
  only sets 5. `GetParticipant` is already on the plugin's `RoomClient` interface
  (`plugins/livekit/interfaces.go:15`), so merge-then-send is testable with the
  existing mock infrastructure.
- **auth-3 is partially stale.** The scaffolded route schema
  (`cmd/noda/auth_templates/routes/auth.reset-password.json`) has enforced password
  `minLength: 8, maxLength: 512` since #247, with a test asserting it
  (`TestAuthInitRegisterRouteEnforcesPasswordLength`). The headline scenario
  (short password burns the token) is already blocked at the HTTP layer. Residual
  exposure: (a) JSON Schema `maxLength` counts code points while `validatePassword`
  counts bytes — a password ≤512 code points but >512 bytes passes the route and
  fails the node, burning the token; (b) any `set_password` failure after `consume`
  commits (DB error, user deleted meanwhile) burns the token — the two nodes run in
  separate transactions; (c) workflows wired without the scaffolded route get no
  protection.
- **auth-4 confirmed; design space is constrained.** Eager rehash is impossible
  (argon2 needs the plaintext; only lazy rehash-on-login works). A "max cost" dummy
  fixes nothing in the reported direction — legacy wrong-password attempts stay fast
  regardless of dummy weight. A "min observed cost" dummy only narrows the oracle
  (unknown-email would match legacy accounts but differ from upgraded ones). With
  heterogeneous hash params, CPU-time equalization is unwinnable; the complete fix
  is a wall-clock floor on the invalid path — the exact pattern auth-2 shipped in
  `auth.request-password-reset.json.tmpl` (`start_ts` → `util.delay` to a 500ms floor).

## Decisions (user-approved)

1. **One combined tranche** ("review-closeout", tranche G), not two or three.
2. **auth-4:** template-level pad in the scaffolded login flow + honest doc/comment
   fixes. No node-level min-duration config.
3. **auth-3:** atomic consume-inside-`set_password` (optional `token` config, one
   transaction) + rune/byte alignment in `validatePassword`. No new composite node.
4. **realtime-6 hardening:** unknown keys in the `permissions` map become an error
   (approved behavior change).
5. **`set_password` token purpose is hardcoded** to `reset_password` (YAGNI on an enum).

## Design

### Task 1 — realtime-6: merge-then-send in `lk.participantUpdate`

`plugins/livekit/participant_update.go`:

- When `permissions` is present in config: call
  `svc.Room.GetParticipant(ctx, &lkproto.RoomParticipantIdentity{Room: room, Identity: identity})`
  first. `proto.Clone` the returned `Permission` (nil → fresh
  `&lkproto.ParticipantPermission{}`), overlay only the boolean keys present in the
  config map, set the result on the update request.
- Recognized keys stay the current five: `canPublish`, `canSubscribe`,
  `canPublishData`, `hidden`, `recorder` (the `recorder` SA1019 nolint from BASE-3
  stands). All other `ParticipantPermission` fields (`canPublishSources`,
  `canUpdateMetadata`, `canSubscribeMetrics`, `canManageAgentSession`, `agent`) are
  preserved by the merge but not settable — unchanged scope.
- **Unknown keys in the `permissions` map are an error** (`lk.participantUpdate:
  unknown permission key "x"`). Rationale: pre-merge, a typo silently revoked every
  permission; post-merge it would be a silent no-op; an explicit error is strictly
  more useful. This rejects previously-accepted configs — approved.
- `GetParticipant` failure → node `error` output. No `GetParticipant` call when
  `permissions` is absent (metadata-only updates unchanged).
- Node `Description()` and the node doc in `docs/03-nodes/` document merge semantics
  and the get-then-update race (a concurrent permission change between the read and
  the write can be lost; acceptable for an admin operation).

Tests (`plugins/livekit`, existing mock `RoomClient`):
- Partial map (`{"canPublish": false}` against a participant with all-true perms)
  preserves `canSubscribe`/`canPublishData`/`hidden` in the sent request.
- Nil current `Permission` → zero base, overlay still applies.
- Unknown key → error, no RPC sent.
- `GetParticipant` error → error output, no `UpdateParticipant` call.
- No `permissions` in config → no `GetParticipant` call.

### Task 2 — auth-3: atomic consume-in-`set_password` + rune alignment

`plugins/auth/set_password.go`:

- New optional config key `token` (raw one-time token, expression). When present,
  execution order is:
  1. Validate the password first (pure CPU, before any DB write).
  2. One `db.Transaction`: consume the token (purpose hardcoded `reset_password`,
     same `WHERE consumed_at IS NULL AND expires_at > ?` guard as `consume_token`)
     → resolve `user_id` from the token row (the `user_id` config key is NOT used in
     token mode; the token owns identity) → update `password_hash` → revoke sessions
     (per `revoke_sessions`, default true). Any failure rolls back everything; the
     token survives.
- `ConfigSchema`: `token` added; exactly one of `user_id` | `token` must be provided
  (validated at execute time; schema documents it).
- `Outputs()` becomes `{success, invalid, error}`. `invalid` = token unknown /
  expired / already consumed (mirrors `consume_token`; undifferentiated). Password
  validation failure stays `error` (unchanged semantics for existing non-token
  callers) — but in token mode it happens before the transaction, so the token is
  never burned.
- The consume UPDATE + user_id lookup logic is factored into a shared unexported
  helper in `one_time_tokens.go` (e.g. `consumeTokenInTx(tx, hash, purpose, now)`)
  used by both `consume_token` and `set_password` — no duplicated WHERE-guard logic.
  `consume_token`'s verify_email side effect stays where it is.
- `validatePassword` (`plugins/auth/helpers.go`) switches from byte length to
  `utf8.RuneCountInString` (8–512), exactly matching the route schemas' code-point
  semantics. Affects `create_user` too. Behavior change: a 4-emoji password
  (16 bytes, 4 runes) passed before and fails now — which is what the documented
  schema always claimed.

`cmd/noda/auth_templates/workflows/auth.reset-password.json.tmpl`:

- Collapses to one `set_password` node (`token` + `password` config), edges:
  `invalid` → `respond_invalid` (400, unchanged body), success → `respond` (200).
  The separate `consume` node disappears from the template.

Tests:
- `plugins/auth` unit tests: token mode happy path (password updated, sessions
  revoked, token consumed); invalid/expired/reused token → `invalid`, password
  unchanged; bad password in token mode → `error` AND token still consumable
  afterward; mutual exclusion of `user_id`/`token`; rune-boundary cases for
  `validatePassword` (4-emoji fails min, 512 multi-byte runes passes max).
- Scaffold suite (`runScaffoldedAuthSuite` in `cmd/noda/auth_init_test.go` +
  `loadResolvedConfigForTest`): full reset flow through the new template; a failed
  reset attempt leaves the token usable (same token succeeds on retry).
- Update `TestAuthInitRegisterRouteEnforcesPasswordLength` expectations only if the
  route schema changes (it does not — route stays as is).

### Task 3 — auth-4: wall-clock floor on login's invalid path

`cmd/noda/auth_templates/workflows/auth.login.json.tmpl`:

- Adopt the auth-2 pattern verbatim: `start_ts` (`util.timestamp`, unix_ms) runs
  first; the `verify` node's `invalid` output routes through `now_ts_invalid` →
  `pad_invalid` (`util.delay` with the
  `{{ (nodes.start_ts + 500) > nodes.now_ts_invalid ? … : 0 }}ms` floor expression)
  → `respond_invalid` (401, unchanged body). Unknown-email and wrong-password both
  land there and flatten to the same wall-clock time regardless of argon2 param
  drift. The success path is unpadded (someone holding the correct password is not
  enumerating).
- Floor is 500ms, matching request-password-reset. Documented caveat (template-
  adjacent docs + CHANGELOG): if argon2 verification exceeds the floor, the pad
  clamps to 0 — operators with heavy params must raise the floor. Policy lives in
  the visible scaffold per the shadcn model.
- `VerifyDummy` comment in `plugins/auth/crypto.go` rewritten to be honest: the
  dummy uses the *configured* params; stored hashes with older, cheaper params
  remain CPU-time distinguishable from the dummy until rehash-on-login converges;
  the scaffolded login flow's wall-clock pad (not the dummy) closes that drift
  oracle; custom login flows should copy the pad pattern. `VerifyDummy` itself is
  unchanged — it still flattens the common no-drift case.
- Auth guide docs updated with the drift caveat + pad pattern for custom flows.

Tests:
- Scaffold suite: run `test-auth-login` through `runScaffoldedAuthSuite` (invalid
  case exercises the pad chain with mocks, mirroring the request-password-reset
  suite), plus a rendered-template wiring assertion that `pad_invalid`'s timeout
  expression references `nodes.start_ts + 500` and `nodes.now_ts_invalid` — the
  suite mocks the pad, so a node-name typo would otherwise only explode at
  runtime. The unmocked expression mechanics are already proven by
  `TestScratch_PasswordResetPadExpressionResolvesUnmocked` (identical expression);
  a login-specific clone would be pure duplication.

## Behavior changes (CHANGELOG-worthy)

1. `lk.participantUpdate`: partial `permissions` maps no longer revoke omitted
   permissions (merge-then-send; one extra `GetParticipant` RPC on the permissions
   path). Unknown permission keys are now an error.
2. `auth.set_password`: new optional `token` config (atomic consume+set), new
   `invalid` output. `validatePassword` counts runes, not bytes (affects
   `create_user` too).
3. New scaffolds: login's invalid path gains a 500ms wall-clock floor;
   reset-password workflow collapses to the atomic single-node form. **Existing
   scaffolded projects do not auto-update** — the CHANGELOG entry tells operators
   to apply both template changes manually.

## Out of scope

- Supporting the four unhandled `ParticipantPermission` fields as settable config.
- A `purpose` enum on `set_password` token consumption (hardcoded `reset_password`).
- Node-level min-duration/padding config on `auth.verify_credentials`.
- Retrofitting existing scaffolded projects.
- The remaining Low long-tail findings (tracked in `REVIEW-FINDINGS-2026-07-05.md`).

## Mechanics

- Worktree `.worktrees/review-closeout` off main; branch `review-closeout`.
- SDD ledger at `.superpowers/sdd/progress.md`; subagent-driven execution (fresh
  implementer + two-stage review per task); final whole-branch review on opus
  (auth-sensitive).
- Gates per task: `gofmt`, `go vet ./...`, `golangci-lint run`, `go test -race` on
  `plugins/livekit`, `plugins/auth`, `cmd/noda`.
- One CHANGELOG entry: realtime-6 + auth-3 under the existing `### Fixed`, auth-4
  under the single `### Security` (no duplicate category headers).
- Squash-merge PR (UNSTABLE = pending non-required `benchmark` check is safe);
  "Shipped" notes added to all three findings in `REVIEW-FINDINGS-2026-07-05.md`
  at merge time; follow-up issues filed at finish.
