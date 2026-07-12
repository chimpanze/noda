# Homebase Hygiene — Design

**Date:** 2026-07-12
**Issues:** #303 (drops cursor robustness), #304 (/setup race), #305 (DOMAIN fail-fast), #306 (e2e compose isolation), #310 (e2e standalone-run)
**Branch:** `feat/homebase-hygiene`

## Problem

Five hygiene findings from the homebase PR #299/#307 reviews, all inside
`projects/homebase/`:

- **#303:** `GET /drops?before=<garbage>` raises a Postgres cast error → 500
  (`NULLIF(?, '')::timestamptz` in `workflows/drops.list.json`); the strict
  `created_at < ?` cursor can skip rows sharing a timestamp across a page
  boundary; `workflows/shares.get.json` keys `has_file` on `file_name` while
  every other workflow keys on `file_key`.
- **#304:** `workflows/setup.json` checks `count(auth_users) == 0` then
  calls `auth.create_user` — two concurrent requests with the valid
  SETUP_TOKEN and different emails can both pass the empty check and each
  create a user. Requires the secret token; theoretical but cheap to close.
- **#305:** `docker-compose.yml` uses `DOMAIN: ${DOMAIN:-localhost}` because
  compose interpolates at parse time regardless of profiles — `--profile
  edge` with DOMAIN unset silently requests a cert for "localhost" and fails
  later with an opaque ACME error.
- **#306:** `e2e/run.sh` runs `docker compose down -v` against the DEFAULT
  compose project for the directory — on a host also serving production from
  the same directory it would destroy the real volumes. Also the teardown
  `trap` installs after `up -d --build`, so a failed build skips teardown.
- **#310:** `TestRoomsLifecycle` can't run standalone (assumes the admin
  account exists from `TestHomebaseLifecycle`'s `/setup` in the same binary
  run). The 1.5s expiry sleeps are fine per the decision recorded in the
  issue — poll-loop only if they ever flake, never inflate the sleep.

## Decisions (user-approved 2026-07-12)

1. **Malformed `before` → 400** with a clear error (workflow guard), not
   silent-ignore. Matches the fail-loudly direction of tranches 1–2.
2. **Tuple cursor now:** `(created_at, id)` tie-break with a new optional
   `before_id` input and `next_before_id` output. Timestamp-only callers
   keep exactly today's behavior; homebase-web adoption is out of scope.

## Design

### 1. Drops cursor robustness (#303)

`workflows/drops.list.json` (and its route `routes/drops.list.json`):

- **(a) 400 guard:** a `control.if` ahead of the find: `input.before` is
  either empty or matches a timestamptz-acceptable pattern
  (`^\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}` prefix form — the exact regex is
  pinned in the plan; it must accept every value the workflow itself emits as
  `next_before`). Fail → `response.error` 400 with message
  `invalid before cursor`. `before_id` needs no format guard (TEXT id,
  no cast).
- **(b) tuple cursor:** the find's query becomes
  `... AND (created_at, id) < (COALESCE(NULLIF(?, '')::timestamptz, 'infinity'), ?)`
  with params `input.before`, `input.before_id`. The empty string is the
  deliberate default for a missing `before_id`: `''` sorts below every uuid
  in TEXT comparison, so `(created_at, id) < (ts, '')` reduces to strict
  `created_at < ts` — a `before`-without-`before_id` call reproduces today's
  semantics exactly (no re-served boundary row, no behavior change). When
  `before_id` IS supplied (the last row's id), equal-timestamp rows with
  smaller ids are correctly included — the no-skip tie-break. An e2e
  assertion pins the no-skip/no-dup property across a same-timestamp page
  boundary. Output map gains
  `next_before_id: {{ len(nodes.find) > 0 ? nodes.find[-1].id : nil }}`.
- Route input gains `"before_id": "{{ query.before_id ?? '' }}"`.
- **(c)** `workflows/shares.get.json` keys `has_file` on
  `nodes.get_drop.file_key != nil`, aligning with every other workflow.

Note: `drops.list` is the only cursor-paginated workflow (verified — shares/
rooms/sessions lists carry no `before`).

### 2. Setup race closure (#304)

New migration pair `migrations/20260712000004_single_admin.up.sql` /
`.down.sql`:

```sql
-- up: homebase is single-owner by design; guests are tokens, never users.
CREATE UNIQUE INDEX IF NOT EXISTS auth_users_single_row ON auth_users ((true));
-- down:
DROP INDEX IF EXISTS auth_users_single_row;
```

No workflow change: the race loser's INSERT hits the index,
`auth.create_user`'s `isUniqueViolation` maps it to the `exists` output
(`plugins/auth/create_user.go:117-119`), and `setup.json` already routes
`exists` → 403. Concurrent double-setup yields exactly one 200 and one 403.

Constraint check (verified): homebase creates users ONLY via `/setup`
(routes: auth.login/logout/me + setup; no register), so the single-row index
cannot break any other flow.

### 3. DOMAIN fail-fast via edge override (#305)

- The `caddy` service (with its volumes/ports) moves from
  `docker-compose.yml` to a new `projects/homebase/docker-compose.edge.yml`
  override file, with `DOMAIN: ${DOMAIN:?set DOMAIN in .env}` restored and
  the `profiles: ["edge"]` key removed — the override file is the opt-in.
- `caddy-data` volume declaration moves with it.
- Plain `docker compose up` never parses the caddy service, so `${DOMAIN:?}`
  cannot break dev; edge deploys fail fast at parse time when DOMAIN is
  unset.
- **Runbook change:** prod invocation becomes
  `docker compose -f docker-compose.yml -f docker-compose.edge.yml up -d`
  (was `--profile edge`). README deployment section updated; `.env.example`'s
  DOMAIN comment adjusted if it references the profile.

### 4. e2e compose isolation (#306)

`e2e/run.sh`:

- `COMPOSE="docker compose -p homebase-e2e -f docker-compose.yml -f e2e/docker-compose.e2e.yml"`
  — the dedicated project name applies to the leading cleanup `down -v`, the
  `up`, and the trap teardown, so the script can never touch the default
  project's (production) volumes.
- `trap '$COMPOSE down -v --remove-orphans' EXIT` moves ABOVE
  `$COMPOSE up -d --build`, so a failed build still tears down.
- The README's "never run e2e on prod" warning is softened to reflect the
  defense-in-depth (kept, but noting the project isolation).

### 5. e2e standalone-run (#310)

`e2e/e2e_test.go`:

- New helper `loginOrSetup(t, client)` — attempt login with the e2e
  credentials; on 401, POST `/setup` with the e2e SETUP_TOKEN, then login.
  `TestRoomsLifecycle` uses it instead of bare login, making
  `go test -tags e2e -run TestRoomsLifecycle` work against a fresh stack.
  `TestHomebaseLifecycle` keeps its explicit setup subtests (they test
  `/setup` itself) — unchanged.
- The two 1.5s expiry sleeps stay; each gains a one-line comment referencing
  #310's recorded decision (poll loop if it ever flakes; never inflate).

### Testing

The homebase e2e suite is the acceptance test:

- New e2e subtests: malformed `before` → 400; same-timestamp pagination
  proving the tuple cursor doesn't skip (insert rows with identical
  `created_at` via drops API fast-path or direct SQL, page with
  `before`+`before_id`, assert full coverage no dups); concurrent
  double-`/setup` (two goroutines, assert exactly one 200 and one 403 in
  either order); `has_file` via shares.get consistency (existing coverage
  extended only if a subtest already exercises it).
- Full `e2e/run.sh` green.
- Standalone proof: `go test -tags e2e -run TestRoomsLifecycle` against a
  fresh stack (fresh `-p homebase-e2e` project) passes.
- `noda validate --config projects/homebase` passes; compose config parses in
  both flavors (`docker compose config -q` with and without the edge
  override, DOMAIN set and unset — unset edge must FAIL).
- Migration cycle: `noda migrate up` then `down` then `up` against the e2e
  database applies/reverts `20260712000004` cleanly.

## Execution shape

Standing conventions: worktree `.worktrees/homebase-hygiene`, branch
`feat/homebase-hygiene` off main; spec + plan `git add -f`'d onto the branch;
subagent-driven per task (5 implementation tasks + final verification);
whole-branch review before PR. PR closes #303, #304, #305, #306, #310.

PR #312 (tranche 2) may merge while this is in flight — no file overlap
(different directories entirely except CHANGELOG); final task rebases onto
latest main. CHANGELOG entries under [Unreleased] (Fixed: drops 500 +
has_file; Added: single-admin migration note if house style wants it;
Changed: edge override runbook).

## Out of scope

- homebase-web (separate repo) adopting `before_id` — its timestamp-only
  pagination keeps working, minus the same-timestamp edge until updated.
- Prod VPS redeploy with the new compose invocation (user's manual step).
- Any noda-runtime change — this tranche is entirely `projects/homebase/`.
- Changing the expiry sleeps (#310.2 records the decision; no code change).
