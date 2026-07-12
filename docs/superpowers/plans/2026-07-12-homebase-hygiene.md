# Homebase Hygiene Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the five homebase hygiene issues — drops cursor robustness (#303), the /setup race (#304), DOMAIN fail-fast via a compose edge override (#305), e2e compose isolation (#306), and e2e standalone-run (#310) — entirely inside `projects/homebase/`.

**Architecture:** All changes are homebase config/compose/test files; zero noda-runtime changes. Ordering is deliberate: the e2e isolation (#306) lands first so every later task's manual stack uses the safe `-p homebase-e2e` project; `loginOrSetup` (#310) lands second because later e2e tests use it; then the drops workflow (#303), the migration (#304), and the compose override (#305); a final task rebases, runs the full suite, and opens the PR.

**Tech Stack:** noda JSON workflows (expr-lang expressions — note `matches` is INFIX), Postgres (tuple comparison, partial-unique-index-via-`((true))`), docker compose v2, Go e2e tests (`-tags e2e`).

**Spec:** `docs/superpowers/specs/2026-07-12-homebase-hygiene-design.md`

## Global Constraints

- Every manual compose invocation during this work MUST carry `-p homebase-e2e` — the default project for `projects/homebase/` may hold real volumes on some hosts (the very risk #306 closes).
- The 1.5s expiry sleeps in `e2e/e2e_test.go` (~lines 310, 553) must NOT be changed — #310 records that decision; they only gain a reference comment.
- Missing-`before_id` sentinel is the EMPTY STRING: `(created_at, id) < (ts, '')` reduces to strict `created_at < ts` (`''` sorts below every uuid), so timestamp-only callers keep exactly today's behavior. Do not "improve" this to a high sentinel — `'~'` would re-serve the boundary row.
- The tuple cursor requires a total order: `db.find`'s `order` becomes `"created_at DESC, id DESC"` (same-timestamp rows must page deterministically).
- Malformed `before` → HTTP 400, `code: "INVALID_CURSOR"`, `message: "invalid before cursor"`. The accept-regex must match every value the workflow emits as `next_before` (Go RFC3339Nano like `2026-07-12T10:00:00.123456Z` AND Postgres text like `2026-07-12 10:00:00.123456+00`).
- expr-lang: `matches` is an infix operator (`x matches '...'`), never `matches(x, ...)`.
- Workflow-test-runner constraints (learned in prior homebase cycles): plugin nodes with NON-default output edges must be mocked in every test case; unmocked plugin nodes synthesize success; `control.if`/`response.*` evaluate for real.
- e2e helpers that shell out target the container by name `homebase-e2e-postgres-1` (couples to Task 1's `-p homebase-e2e`).
- Conventional commits with trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. Go files touched: `gofmt -l` clean (ignore the known pre-existing `examples/wasm-helpers` hit), `go vet ./projects/homebase/e2e/` (with `-tags e2e`) clean.
- PR #312 may merge mid-flight; final task rebases onto latest main (only expected overlap: CHANGELOG).

## Worktree setup (before Task 1)

```bash
git -C /Users/marten/GolandProjects/noda worktree add .worktrees/homebase-hygiene -b feat/homebase-hygiene main
cd /Users/marten/GolandProjects/noda/.worktrees/homebase-hygiene
mkdir -p docs/superpowers/specs docs/superpowers/plans
cp ../../docs/superpowers/specs/2026-07-12-homebase-hygiene-design.md docs/superpowers/specs/
cp ../../docs/superpowers/plans/2026-07-12-homebase-hygiene.md docs/superpowers/plans/
git add -f docs/superpowers/specs/2026-07-12-homebase-hygiene-design.md docs/superpowers/plans/2026-07-12-homebase-hygiene.md
git commit -m "docs: spec + plan for homebase hygiene tranche (#303-#306, #310)"
```

## Manual e2e stack (used by Tasks 2-4; run from the worktree)

```bash
cd projects/homebase
export SETUP_TOKEN=e2e-setup-token PUBLIC_BASE_URL=http://localhost:3000 \
  LIVEKIT_URL=ws://livekit:7880 LIVEKIT_API_KEY=devkey LIVEKIT_API_SECRET=secret
COMPOSE="docker compose -p homebase-e2e -f docker-compose.yml -f e2e/docker-compose.e2e.yml"
$COMPOSE down -v --remove-orphans   # always start fresh
$COMPOSE up -d --build
until curl -fso /dev/null http://localhost:3000/health/ready; do sleep 1; done
# run focused tests from the REPO ROOT of the worktree:
SETUP_TOKEN=e2e-setup-token go test -tags e2e -count=1 -v -run <TestName> ./projects/homebase/e2e/
# config-only change (workflows/routes): $COMPOSE restart noda
# new migration: full "down -v && up -d --build" (migrate is a one-shot service)
$COMPOSE down -v --remove-orphans   # teardown when done
```

---

### Task 1: e2e compose isolation + unconditional teardown (#306)

**Files:**
- Modify: `projects/homebase/e2e/run.sh`
- Modify: `projects/homebase/README.md` (~line 29, the "Never run e2e/run.sh" bullet)

**Interfaces:**
- Produces: the `-p homebase-e2e` project name that ALL later tasks' manual stacks and test helpers (`homebase-e2e-postgres-1`) rely on.

- [ ] **Step 1: Edit `run.sh`**

Two changes to the existing script (leave everything else byte-identical):

```bash
COMPOSE="docker compose -p homebase-e2e -f docker-compose.yml -f e2e/docker-compose.e2e.yml"

$COMPOSE down -v --remove-orphans 2>/dev/null || true
trap '$COMPOSE down -v --remove-orphans' EXIT
$COMPOSE up -d --build
```

(i.e. `-p homebase-e2e` added to the COMPOSE variable; the `trap` line moved ABOVE the `up -d --build` line.)

- [ ] **Step 2: Update the README bullet**

Replace the bullet at `projects/homebase/README.md:29` with:

```markdown
- `e2e/run.sh` runs in its own isolated compose project (`homebase-e2e`) and tears down only that project's volumes — it cannot touch a production stack running from this directory under the default project name. Still, prefer not to run it on a production host.
```

- [ ] **Step 3: Verify**

Run: `bash -n projects/homebase/e2e/run.sh` → exit 0.
Run the full suite once as the baseline for the tranche: `./projects/homebase/e2e/run.sh` from the worktree root.
Expected: suite green; `docker volume ls | grep homebase-e2e` shows the project-prefixed volumes during the run; after exit, `docker compose -p homebase-e2e ... ps` is empty.

- [ ] **Step 4: Commit**

```bash
git add projects/homebase/e2e/run.sh projects/homebase/README.md
git commit -m "fix(homebase): isolate e2e compose project (-p homebase-e2e) + trap before up (#306)"
```

---

### Task 2: e2e standalone-run — loginOrSetup (#310)

**Files:**
- Modify: `projects/homebase/e2e/e2e_test.go` (helper near `login` ~line 101; `TestRoomsLifecycle` ~line 415; the two sleeps ~lines 310, 553)

**Interfaces:**
- Consumes: existing `client`, `doJSON`, `wantStatus`, `decode`, `drainAndClose`, constants `adminEmail`/`adminPassword`/`setupToken`.
- Produces: `func loginOrSetup(t *testing.T) *client` — later tasks' new e2e tests call it.

- [ ] **Step 1: Add the helper** (directly below `login`)

```go
// loginOrSetup logs in as the admin, bootstrapping the account via /setup
// first when the stack is fresh — lets any test run standalone against a
// fresh stack instead of depending on TestHomebaseLifecycle's setup (#310).
func loginOrSetup(t *testing.T) *client {
	t.Helper()
	anon := &client{t: t}
	resp := anon.doJSON("POST", "/auth/login", map[string]string{
		"email": adminEmail, "password": adminPassword,
	})
	if resp.StatusCode == 401 {
		drainAndClose(resp)
		setupResp := anon.doJSON("POST", "/setup", map[string]string{
			"setup_token": setupToken, "email": adminEmail, "password": adminPassword,
		})
		// 201 on a fresh stack; 403 if something else completed setup first.
		if setupResp.StatusCode != 201 && setupResp.StatusCode != 403 {
			t.Fatalf("setup: status %d", setupResp.StatusCode)
		}
		drainAndClose(setupResp)
		resp = anon.doJSON("POST", "/auth/login", map[string]string{
			"email": adminEmail, "password": adminPassword,
		})
	}
	wantStatus(t, resp, 200)
	body := decode(t, resp)
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("login returned no token")
	}
	return &client{t: t, token: token}
}
```

(Check the actual setup-success status in `TestHomebaseLifecycle`'s setup subtest — the workflow's `respond_success` is 201; if the existing test asserts a different code, match it.)

- [ ] **Step 2: Use it in TestRoomsLifecycle**

`owner := login(t)` (~line 415) becomes `owner := loginOrSetup(t)`.

- [ ] **Step 3: Annotate the sleeps**

Directly above each `time.Sleep(1500 * time.Millisecond)` (~lines 310 and 553), add:

```go
		// #310: deliberate fixed sleep for a 1s TTL. If this ever flakes,
		// replace with a short poll-until-404 loop — never a bigger sleep.
```

- [ ] **Step 4: Verify standalone**

Boot a FRESH manual stack (see "Manual e2e stack" above), then:
`SETUP_TOKEN=e2e-setup-token go test -tags e2e -count=1 -v -run TestRoomsLifecycle ./projects/homebase/e2e/`
Expected: PASS on the fresh stack (this fails at login before the change). Then run the FULL suite against another fresh stack (`-run 'Test'`) to prove the normal path is unchanged. `gofmt -l projects/homebase/e2e/` clean; `go vet -tags e2e ./projects/homebase/e2e/` clean. Tear down.

- [ ] **Step 5: Commit**

```bash
git add projects/homebase/e2e/e2e_test.go
git commit -m "test(homebase): loginOrSetup lets e2e tests run standalone; annotate expiry sleeps (#310)"
```

---

### Task 3: Drops cursor robustness (#303)

**Files:**
- Modify: `projects/homebase/workflows/drops.list.json` (full replacement below)
- Modify: `projects/homebase/routes/drops.list.json` (add `before_id` input)
- Modify: `projects/homebase/workflows/shares.get.json:42` (`has_file` keying)
- Modify/Create: workflow test in `projects/homebase/tests/` (check the directory for an existing drops-list test file first; extend it if present, else create `projects/homebase/tests/drops-list.json` following the shape of the existing test files there)
- Modify: `projects/homebase/e2e/e2e_test.go` (new `TestDropsCursorPagination` + psql helper)

**Interfaces:**
- Consumes: `loginOrSetup` from Task 2; `-p homebase-e2e` container naming from Task 1.
- Produces: API additions — optional `before_id` query param on `GET /drops`; `next_before_id` in the list response. (homebase-web adoption is out of scope.)

- [ ] **Step 1: Rewrite `workflows/drops.list.json`**

```json
{
  "id": "drops-list",
  "name": "Drops: List & search",
  "nodes": {
    "check_cursor": {
      "type": "control.if",
      "config": {
        "condition": "{{ input.before == '' || input.before matches '^\\\\d{4}-\\\\d{2}-\\\\d{2}([T ]\\\\d{2}:\\\\d{2}:\\\\d{2}(\\\\.\\\\d+)?(Z|[+-]\\\\d{2}(:?\\\\d{2})?)?)?$' }}"
      }
    },
    "find": {
      "type": "db.find",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "select": ["id", "text", "file_name", "file_size", "file_mime", "created_at", "updated_at"],
        "where_clause": {
          "query": "(text ILIKE '%' || ? || '%' OR file_name ILIKE '%' || ? || '%') AND (created_at, id) < (COALESCE(NULLIF(?, '')::timestamptz, 'infinity'), ?)",
          "params": ["{{ input.q }}", "{{ input.q }}", "{{ input.before }}", "{{ input.before_id }}"]
        },
        "order": "created_at DESC, id DESC",
        "limit": 50
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "drops": "{{ nodes.find }}",
          "next_before": "{{ len(nodes.find) > 0 ? nodes.find[-1].created_at : nil }}",
          "next_before_id": "{{ len(nodes.find) > 0 ? nodes.find[-1].id : nil }}"
        }
      }
    },
    "respond_bad_cursor": {
      "type": "response.error",
      "config": { "status": 400, "code": "INVALID_CURSOR", "message": "invalid before cursor" }
    }
  },
  "edges": [
    { "from": "check_cursor", "output": "then", "to": "find" },
    { "from": "check_cursor", "output": "else", "to": "respond_bad_cursor" },
    { "from": "find", "to": "respond" }
  ]
}
```

JSON-escaping note: `\\\\d` in the file is `\\d` in the JSON string is `\d` in the regex. Verify the doubling by running the workflow test (Step 3) — if the engine reports a regex compile error, the escaping is wrong.

Semantics notes (why, for the reviewer):
- Empty `before_id` (`''`) sorts below every TEXT uuid, so `(created_at, id) < (ts, '')` is exactly strict `created_at < ts` — timestamp-only callers are bit-for-bit unchanged. `'infinity'` with `''` id keeps the no-cursor case returning everything.
- `order` gains `id DESC` — without a total order, same-timestamp rows page non-deterministically and the tuple cursor can't guarantee no-skip/no-dup.

- [ ] **Step 2: Route + shares.get**

`projects/homebase/routes/drops.list.json` trigger.input becomes:

```json
    "input": {
      "q": "{{ query.q ?? '' }}",
      "before": "{{ query.before ?? '' }}",
      "before_id": "{{ query.before_id ?? '' }}"
    }
```

`projects/homebase/workflows/shares.get.json:42`:
`"has_file": "{{ nodes.get_drop.file_name != nil }}"` → `"has_file": "{{ nodes.get_drop.file_key != nil }}"`.
Check the surrounding `get_drop` node's `select` list — if it selects columns explicitly and `file_key` is absent, add `file_key` to the select (otherwise the expression reads nil forever and `has_file` is always false; the existing `drops.get.json` keying on `file_key` shows the working pattern).

- [ ] **Step 3: Workflow tests (noda test — no docker needed)**

Look at `projects/homebase/tests/` and mirror the existing test-file shape exactly (mock `db.find` with a success output; `control.if` and `response.*` evaluate for real). Cases to cover for `drops-list`:

- `input: {q: "", before: "garbage", before_id: ""}` → expect the `respond_bad_cursor` path (400 / INVALID_CURSOR).
- `input: {q: "", before: "", before_id: ""}` → find path (mock returns `[]`), respond 200 with `next_before: nil`.
- `input: {q: "", before: "2026-07-12T10:00:00.123456Z", before_id: "abc"}` → find path (regex must accept RFC3339Nano).
- `input: {q: "", before: "2026-07-12 10:00:00.123456+00", before_id: ""}` → find path (regex must accept Postgres text form).

Run: `go run ./cmd/noda test --config projects/homebase` (check the exact test-command shape the existing homebase tests use — e.g. a `--filter`/file arg — and use that).
Expected: new cases pass; all pre-existing workflow tests still pass. RED first: run the garbage-cursor case before Step 1's rewrite is applied if practical (on the unmodified workflow it has no 400 path — the case fails), or note in the report that the rewrite landed first and the polarity was verified by temporarily inverting the condition.

- [ ] **Step 4: e2e — cursor behavior against real Postgres**

Add to `projects/homebase/e2e/e2e_test.go`:

```go
// execPsql runs a statement in the e2e Postgres container (project name from
// e2e/run.sh). Returns combined output; fails the test on error when
// mustSucceed is true.
func execPsql(t *testing.T, sql string, mustSucceed bool) (string, error) {
	t.Helper()
	cmd := exec.Command("docker", "exec", "-i", "homebase-e2e-postgres-1",
		"psql", "-U", "noda", "-d", "noda", "-tAc", sql)
	out, err := cmd.CombinedOutput()
	if mustSucceed && err != nil {
		t.Fatalf("psql %q: %v\n%s", sql, err, out)
	}
	return string(out), err
}

func TestDropsCursorPagination(t *testing.T) {
	owner := loginOrSetup(t)

	t.Run("malformed before is 400", func(t *testing.T) {
		resp := owner.do("GET", "/drops?before=garbage", nil, "")
		wantStatus(t, resp, 400)
		drainAndClose(resp)
	})

	t.Run("same-timestamp rows page without skips", func(t *testing.T) {
		// 60 drops, then force one shared timestamp (only reachable via SQL —
		// the API always stamps now()).
		for i := 0; i < 60; i++ {
			resp := owner.doJSON("POST", "/drops", map[string]string{
				"text": fmt.Sprintf("cursor-probe-%02d", i),
			})
			wantStatus(t, resp, 201)
			drainAndClose(resp)
		}
		execPsql(t, `UPDATE drops SET created_at = now() WHERE text LIKE 'cursor-probe-%'`, true)

		respBody := func(before, beforeID string) map[string]any {
			u := "/drops?q=cursor-probe"
			if before != "" {
				u += "&before=" + url.QueryEscape(before) + "&before_id=" + url.QueryEscape(beforeID)
			}
			resp := owner.do("GET", u, nil, "")
			wantStatus(t, resp, 200)
			return decode(t, resp)
		}

		first := respBody("", "")
		p1, _ := first["drops"].([]any)
		if len(p1) != 50 {
			t.Fatalf("page 1 = %d rows, want 50", len(p1))
		}
		nb, _ := first["next_before"].(string)
		nbID, _ := first["next_before_id"].(string)
		if nb == "" || nbID == "" {
			t.Fatalf("missing cursor: next_before=%q next_before_id=%q", nb, nbID)
		}

		page := func(before, beforeID string) []any {
			drops, _ := respBody(before, beforeID)["drops"].([]any)
			return drops
		}

		// Timestamp-only cursor documents the OLD bug: all 60 share one
		// timestamp, so strict created_at < ts returns nothing.
		if rest := page(nb, ""); len(rest) != 0 {
			t.Fatalf("timestamp-only page 2 = %d rows, want 0 (strict-< semantics)", len(rest))
		}

		// Tuple cursor gets the remaining 10, no dups.
		p2 := page(nb, nbID)
		if len(p2) != 10 {
			t.Fatalf("tuple page 2 = %d rows, want 10", len(p2))
		}
		seen := map[string]bool{}
		for _, raw := range append(append([]any{}, p1...), p2...) {
			row, _ := raw.(map[string]any)
			id, _ := row["id"].(string)
			if seen[id] {
				t.Fatalf("duplicate id across pages: %s", id)
			}
			seen[id] = true
		}
		if len(seen) != 60 {
			t.Fatalf("union = %d unique rows, want 60", len(seen))
		}
	})
}
```

Add imports as needed (`fmt`, `net/url`, `os/exec`). If `client.do` differs from this signature, adapt to the real one (it exists — see e2e_test.go:45).

- [ ] **Step 5: Verify on a manual stack**

Fresh stack; `$COMPOSE restart noda` after any workflow tweak. Run:
`SETUP_TOKEN=e2e-setup-token go test -tags e2e -count=1 -v -run TestDropsCursorPagination ./projects/homebase/e2e/` → PASS.
Also `go run ./cmd/noda validate --config projects/homebase` (export the env vars from the Manual-e2e-stack block plus `BODY_LIMIT=1073741824 DATABASE_URL=... FILES_PATH=/tmp/hb SETUP_TOKEN=x`) → valid.
Then run the FULL e2e suite on a fresh stack → green (existing lifecycle drops subtests must be unaffected: they use no `before_id`).

- [ ] **Step 6: Commit**

```bash
git add projects/homebase/workflows/drops.list.json projects/homebase/routes/drops.list.json projects/homebase/workflows/shares.get.json projects/homebase/tests/ projects/homebase/e2e/e2e_test.go
git commit -m "fix(homebase): drops cursor — 400 on malformed before, (created_at,id) tuple pagination, has_file on file_key (#303)"
```

---

### Task 4: Single-admin index closes the /setup race (#304)

**Files:**
- Create: `projects/homebase/migrations/20260712000004_single_admin.up.sql`
- Create: `projects/homebase/migrations/20260712000004_single_admin.down.sql`
- Modify: `projects/homebase/e2e/e2e_test.go` (two new tests)

**Interfaces:**
- Consumes: `execPsql` and `loginOrSetup` from Tasks 2-3.
- Produces: nothing consumed later.

- [ ] **Step 1: Migrations**

`20260712000004_single_admin.up.sql`:

```sql
-- Homebase is single-owner by design: guests join via tokens, never as
-- auth_users rows. This closes the /setup count-then-create race (#304):
-- the loser's INSERT hits the index, auth.create_user maps the unique
-- violation to its "exists" output, and setup.json already answers 403.
CREATE UNIQUE INDEX IF NOT EXISTS auth_users_single_row ON auth_users ((true));
```

`20260712000004_single_admin.down.sql`:

```sql
DROP INDEX IF EXISTS auth_users_single_row;
```

- [ ] **Step 2: e2e — index probe + race smoke**

Add to `e2e_test.go` (adjust the INSERT's column list to match `migrations/20260707000001_auth_tables.up.sql` exactly — check it first):

```go
// TestSingleAdminIndex proves the #304 migration: a second auth_users row is
// impossible at the database level, whatever the workflow does.
func TestSingleAdminIndex(t *testing.T) {
	_ = loginOrSetup(t) // guarantees exactly one admin row exists
	out, err := execPsql(t, `INSERT INTO auth_users (id, email, password_hash, status, roles, metadata, created_at, updated_at)
		VALUES ('race-probe', 'second@example.com', 'x', 'active', '[]', '{}', now(), now())`, false)
	if err == nil {
		execPsql(t, `DELETE FROM auth_users WHERE id = 'race-probe'`, true)
		t.Fatalf("second auth_users row accepted — single-admin index missing? out=%s", out)
	}
	if !strings.Contains(out, "auth_users_single_row") {
		t.Fatalf("insert failed for an unexpected reason: %s", out)
	}
}

// TestSetupRaceNeverTwoAccounts fires two concurrent /setup calls with
// different emails. On a fresh stack exactly one may win; on an
// already-initialized stack both lose. Either way: never two 201s.
func TestSetupRaceNeverTwoAccounts(t *testing.T) {
	codes := make(chan int, 2)
	for i := 0; i < 2; i++ {
		email := fmt.Sprintf("race-%d@example.com", i)
		go func() {
			anon := &client{t: t}
			resp := anon.doJSON("POST", "/setup", map[string]string{
				"setup_token": setupToken, "email": email, "password": "hunter2hunter2",
			})
			codes <- resp.StatusCode
			drainAndClose(resp)
		}()
	}
	a, b := <-codes, <-codes
	if a == 201 && b == 201 {
		t.Fatalf("both concurrent setups returned 201 — race not closed")
	}
}
```

Note: `client.doJSON` calls `t` helpers — confirm it is goroutine-safe for this use (it only fails via the channel-collected codes here; if `doJSON` internally calls `t.Fatalf` on transport errors, that's acceptable for this smoke test but note it).

- [ ] **Step 3: Verify**

FRESH manual stack (`down -v && up -d --build` — the one-shot `migrate` service applies the new migration; check its logs: `$COMPOSE logs migrate | tail -3` shows `20260712000004` applied). Then:
`SETUP_TOKEN=e2e-setup-token go test -tags e2e -count=1 -v -run 'TestSingleAdminIndex|TestSetupRaceNeverTwoAccounts' ./projects/homebase/e2e/` → PASS.
Migration reversibility: `docker exec -i homebase-e2e-postgres-1 psql -U noda -d noda -tAc "\di auth_users_single_row"` shows the index; then verify the down file by applying it manually and re-applying up:
`docker exec -i homebase-e2e-postgres-1 psql -U noda -d noda -f - < projects/homebase/migrations/20260712000004_single_admin.down.sql` (index gone), then the up file (index back). (This validates the SQL itself; the migrate tool's own down-command wiring isn't exercised here.)

- [ ] **Step 4: Commit**

```bash
git add projects/homebase/migrations/20260712000004_single_admin.up.sql projects/homebase/migrations/20260712000004_single_admin.down.sql projects/homebase/e2e/e2e_test.go
git commit -m "fix(homebase): single-admin unique index closes /setup count-then-create race (#304)"
```

---

### Task 5: DOMAIN fail-fast via edge override (#305)

**Files:**
- Modify: `projects/homebase/docker-compose.yml` (remove `caddy` service + `caddy-data` volume)
- Create: `projects/homebase/docker-compose.edge.yml`
- Modify: `projects/homebase/README.md:15` (deploy command) and the deployment-notes section
- Modify: `projects/homebase/.env.example` (DOMAIN comment ~line 4-5)

**Interfaces:** none consumed later.

- [ ] **Step 1: Create `docker-compose.edge.yml`**

```yaml
# Production TLS edge (Caddy). Opt in explicitly:
#   docker compose -f docker-compose.yml -f docker-compose.edge.yml up -d
# DOMAIN is required at parse time — an unset DOMAIN fails fast here instead
# of requesting a certificate for "localhost" and dying later in ACME (#305).
services:
  caddy:
    image: caddy:2
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    environment:
      DOMAIN: ${DOMAIN:?set DOMAIN in .env}
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - ./static:/srv:ro
      - caddy-data:/data

volumes:
  caddy-data:
```

- [ ] **Step 2: Remove from base compose**

Delete the whole `caddy:` service block (docker-compose.yml:69-81, incl. its `profiles: ["edge"]`) and the `caddy-data:` entry from the top-level `volumes:` map. Nothing else changes.

- [ ] **Step 3: README + .env.example**

README line 15: `docker compose --profile edge up -d     # additionally: Caddy with TLS on :443` becomes:

```markdown
docker compose -f docker-compose.yml -f docker-compose.edge.yml up -d   # additionally: Caddy with TLS on :443 (requires DOMAIN)
```

Sweep the README for other `--profile edge` mentions (deployment notes) and update them to the override-file form. `.env.example`: the comment above DOMAIN (`# domain Caddy serves (production edge profile only)`) becomes `# domain Caddy serves (docker-compose.edge.yml override only; required there at parse time)`.

- [ ] **Step 4: Verify the parse matrix**

From `projects/homebase/` with the e2e env vars exported (SETUP_TOKEN etc., see Manual-e2e-stack block) plus `unset DOMAIN`:

```bash
docker compose -f docker-compose.yml config -q                                  # → exit 0 (dev unaffected)
docker compose -f docker-compose.yml -f docker-compose.edge.yml config -q      # → FAILS: "set DOMAIN in .env"
DOMAIN=example.com docker compose -f docker-compose.yml -f docker-compose.edge.yml config -q   # → exit 0
grep -rn "profile" projects/homebase/README.md projects/homebase/docker-compose.yml            # → no stale edge-profile references
```

(Watch out: a `projects/homebase/.env` file on this machine may set DOMAIN — run the failing case with `env -u DOMAIN` AND temporarily point compose away from .env if needed, e.g. `--env-file /dev/null`, and say which you used in the report.)

- [ ] **Step 5: Commit**

```bash
git add projects/homebase/docker-compose.yml projects/homebase/docker-compose.edge.yml projects/homebase/README.md projects/homebase/.env.example
git commit -m "fix(homebase): move Caddy to docker-compose.edge.yml override — DOMAIN fails fast again (#305)"
```

---

### Task 6: CHANGELOG, rebase, full verification, review, PR

**Files:**
- Modify: `CHANGELOG.md` ([Unreleased] → Fixed)

- [ ] **Step 1: CHANGELOG** (homebase-scoped entries, matching the existing style)

Under `### Fixed`:
- homebase: `GET /drops` returns 400 (not a Postgres-cast 500) on a malformed `before` cursor; pagination gains a `(created_at, id)` tuple cursor (`before_id`/`next_before_id`) so same-timestamp rows can't be skipped (#303)
- homebase: concurrent `/setup` can no longer create two accounts — single-row unique index on `auth_users` (#304)
- homebase: Caddy moved to a `docker-compose.edge.yml` override; an unset `DOMAIN` fails at parse time again instead of an opaque ACME error (#305)

```bash
git add CHANGELOG.md
git commit -m "docs: CHANGELOG for homebase hygiene tranche"
```

- [ ] **Step 2: Rebase onto latest main** (PR #312 may have merged; expected overlap only in CHANGELOG — resolve as entry union)

```bash
git fetch origin main && git rebase origin/main
```

- [ ] **Step 3: Full verification**

```bash
go build ./... && go vet ./... && go vet -tags e2e ./projects/homebase/e2e/
gofmt -l .    # only the known pre-existing examples/wasm-helpers hit
go test ./...
./projects/homebase/e2e/run.sh          # full suite, isolated project
# standalone proof on ANOTHER fresh stack (Manual-e2e-stack block):
#   go test -tags e2e -count=1 -run TestRoomsLifecycle ./projects/homebase/e2e/
```

Expected: all green.

- [ ] **Step 4: Whole-branch review** (final code-reviewer over the full branch diff), then PR:

```bash
git push -u origin feat/homebase-hygiene
gh pr create --title "fix(homebase): hygiene tranche — cursor robustness, setup race, edge override, e2e isolation" \
  --body "$(cat <<'EOF'
Tranche 3 of the open-issue backlog (spec + plan on branch under docs/superpowers/).

- drops list: malformed `before` → 400 INVALID_CURSOR (was Postgres-cast 500); `(created_at, id)` tuple cursor with optional `before_id` + `next_before_id` (timestamp-only callers unchanged — empty-string sentinel reproduces strict `<`); `order` now `created_at DESC, id DESC`; shares.get `has_file` keys on `file_key` like everywhere else (#303)
- `/setup` race: single-row unique index on `auth_users` — the loser's insert routes through auth.create_user's `exists` output to the existing 403 (#304)
- Caddy lives in `docker-compose.edge.yml` now; `DOMAIN` unset fails at parse time; plain `docker compose up` never parses the edge service. Runbook: `--profile edge` → `-f docker-compose.yml -f docker-compose.edge.yml` (#305)
- e2e: isolated compose project `-p homebase-e2e` (down -v can't touch prod volumes) + teardown trap installed before `up` (#306); `loginOrSetup` makes `TestRoomsLifecycle` standalone-runnable; expiry sleeps annotated per the recorded decision (#310)

New e2e coverage: same-timestamp pagination (proves the old skip and the new no-skip), malformed-cursor 400, DB-level single-admin probe, concurrent double-setup smoke.

Closes #303
Closes #304
Closes #305
Closes #306
Closes #310

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Wait for the 4 required functional CI checks before merging. Note for the PR description already included: prod redeploy (new compose invocation + `NODA_VERSION` bump when the next image ships) is the user's manual step.
