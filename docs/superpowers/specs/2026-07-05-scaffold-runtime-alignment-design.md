# Scaffold / Runtime Contract Alignment (Tranche C) — Design

Date: 2026-07-05
Source: `REVIEW-FINDINGS-2026-07-05.md` — cmd-misc-1/2/3/4/5/6 + config-expr-1 (3 High, 4 Medium).
Branch/worktree (planned): `feat/scaffold-runtime-alignment` in `.worktrees/scaffold-runtime-alignment`, off `main`.

## Why

The clean-slate Go review found a cluster where Noda's own generators, examples, and CLI commands disagree with the runtime they target. The worst offenders produce configs that **pass `noda validate` green, then fail at runtime** — an AI agent (or new user) scaffolds a project, it validates, and every generated route 500s. This tranche makes the scaffold surface tell the truth: what the generators emit is what the runtime accepts.

**Decisions (user-approved):**
- cmd-misc-1: add a `request` alias to the runtime route context **and** fix the generators to the canonical namespace (both sides).
- cmd-misc-6: refuse to overwrite existing files; `noda init` gets `--force`; MCP scaffold has no force.
- One PR (matches tranches A/B).

## Findings in scope

| ID | Sev | Summary |
|---|---|---|
| cmd-misc-1 | High | Scaffold + example route triggers use a nonexistent `request.*` namespace → routes 500 while validate passes |
| cmd-misc-2 | High | `noda auth init` matches only plugin `"db"`, but scaffolders emit `"postgres"` → auth init fails on scaffolded projects |
| cmd-misc-3 | Med | `noda migrate` auto-detect matches only `"postgres"`, not the equally valid `"db"` (mirror of cmd-misc-2) |
| cmd-misc-4 | High | GenerateCRUD tenant scope never threaded from route into input → broken scoping; update takes scope from request body (cross-tenant write) |
| cmd-misc-5 | Med | MCP example configs invalid: `util.jwt_sign` missing `secret` + wrong `expires_in` key; crud example uses `$ref(...)` string syntax |
| cmd-misc-6 | Med | `noda init` and MCP `noda_scaffold_project` silently overwrite existing files |
| config-expr-1 | Med | `expression_strict_mode` makes control.loop/transform.map/filter fail bootstrap (`knownContextEnv` omits `$item`/`$index`) |

## Verified facts

- Runtime route trigger context — `internal/server/trigger.go buildRawRequestContext` returns top-level keys `body, params, query, headers, method, path` (+ `auth` when JWT claims present). No `request` key. `MapTrigger` (trigger.go:24) resolves trigger-input expressions at **request time** via `expr.NewResolver(compiler, rawCtx)`; this path is **non-strict** (undefined vars allowed), which is why `request.*` compiled fine but resolved to nil → 500, rather than being rejected at validate.
- `request.*` IS a valid namespace for WebSocket connection channel patterns (`internal/connmgr/pattern.go`), so aliasing it on routes unifies the two.
- MCP scaffold: `scaffoldSampleRoute` const (`internal/mcp/tools.go:979`) contains `"{{ request.params.name }}"` (line 986); `scaffoldNodaJSON` const emits `"plugin": "postgres"` (line 933). Scaffold writes files via `os.WriteFile` in `scaffoldProjectHandler` (tools.go:781). `variableRe` (tools.go:293) already lists `request`.
- db plugin: `Name()="postgres"`, `Prefix()="db"` (`plugins/db/plugin.go:20-21`). `auth init` matches literal `"db"` via `findServicesByPlugin(services, "db")` (`cmd/noda/auth_init.go:56,198` `svc["plugin"] != pluginName`). `noda migrate` matches literal `"postgres"` via `postgresServiceNames` (`cmd/noda/migrate_service.go:33`).
- CRUD generator (`internal/generate/crud.go`): route input maps build `{id: params.id}` + `body.<col>` only (lines 72-85 create, 151-169 update) — no `params.<ScopeParam>`. Workflows reference `input.<ScopeParam>` in WHERE (357/397/456/494/542) and update writes `data: expr("input")` (512).
- `util.jwt_sign` real schema (`plugins/core/util/jwt.go:17-42`): required `claims`, `secret`; expiry field is `expiry` (not `expires_in`). Secrets accessed via `{{ secrets.X }}` (shipped-example convention). `$ref` real form is a JSON object `{"$ref": "schemas/Name"}` (crud.go:88, examples/saas-backend).
- `internal/expr/compiler.go`: `knownContextEnv` (line 66) = `input/auth/trigger/nodes/secrets`; used with `expr.Env()` in strict mode (line 125). `control.loop` overlays `$item`/`$index` via `ResolveWithVars` (`plugins/core/control/loop.go:133-134`).
- Write sites for overwrite protection: MCP `tools.go:781`; `noda init` `cmd/noda/init.go:90` (in `scaffoldProject`).

## Design

### Unit 1 — `request` route-context alias + canonical generators (cmd-misc-1)

**Runtime** (`server/trigger.go buildRawRequestContext`): after building the top-level `ctx`, add `ctx["request"] = map[string]any{...}` with the same members (`body`, `params`, `query`, `headers`, `method`, `path`, and `auth` when present). Build it from the already-parsed values (don't re-parse). Both `params.id` and `request.params.id` then resolve identically.

**Generators:** change `scaffoldSampleRoute` (tools.go:986) and every `request.*` in `examples.go` route triggers to the canonical top-level form (`{{ params.name }}`, `{{ body.x }}`). `variableRe` keeps `request` (now valid at runtime). Rationale: canonical matches shipped docs/examples (`{{ body.title }}`, `{{ params.id }}`); the runtime alias is the safety net for agent-written `request.*`.

### Unit 2 — Shared database-service matcher (cmd-misc-2 + cmd-misc-3)

Add `isDatabaseService(svc map[string]any) bool` (returns true when `svc["plugin"]` is `"db"` or `"postgres"`) in a shared spot in `cmd/noda`. Route both matchers through it: `findServicesByPlugin`'s DB path (auth_init.go — when the requested pluginName is the DB plugin) and `postgresServiceNames` (migrate_service.go). Preserve the email path in `findServicesByPlugin` (only the DB match broadens). Both `auth init` and `migrate` then accept either name; error messages updated to say `"db"/"postgres"`.

### Unit 3 — CRUD tenant-scope threading (cmd-misc-4)

In `internal/generate/crud.go`, when `opts.ScopeCol != "" && opts.ScopeParam != ""`:
- **Every scoped route** (create/list/get/update/delete) maps the scope param from the URL: `inputMap[opts.ScopeParam] = expr("params." + opts.ScopeParam)`.
- **Body loops exclude the scope column** (`if col.Name == opts.ScopeCol { continue }` alongside the existing PK/timestamp skips) so the tenant value never comes from the request body.
- **Update's data becomes explicit**: build a `data` map of the mutable body columns only (excluding `id`, the scope column, PK, and timestamps) instead of `data: expr("input")`. The WHERE `{id: input.id, ScopeCol: input.<ScopeParam>}` scopes to the URL tenant; the data payload can no longer carry a client-supplied scope/id.

Unscoped CRUD (no ScopeCol/ScopeParam) is unchanged.

### Unit 4 — Valid MCP example configs (cmd-misc-5)

In `internal/mcp/examples.go`:
- `"schema": "$ref(schemas/user.json)"` → `"schema": map[...]{"$ref": "schemas/User"}` (object form; path without `.json`, matching crud.go / saas-backend).
- `util.jwt_sign` example: add the required `"secret": "{{ secrets.JWT_SECRET }}"`; rename `"expires_in": "24h"` → `"expiry": "24h"`.
- Any other `$env('...')` secret access in the examples → `{{ secrets.X }}`.

### Unit 5 — Scaffold overwrite protection (cmd-misc-6)

- **MCP** (`scaffoldProjectHandler`, tools.go): before writing any file, stat every target path; if any already exists, return an error listing the conflicts and write nothing (all-or-nothing).
- **`noda init`** (`scaffoldProject`, init.go): same pre-check; add a `force bool` parameter and a `--force` cobra flag on the init command that skips the check. Without `--force`, refuse and list conflicts.

### Unit 6 — Strict-mode `$item`/`$index` (config-expr-1)

Add `"$item"` and `"$index"` entries to `knownContextEnv` (`expr/compiler.go:66`). This lets strict mode accept the loop/map/filter variables. Trade-off (documented): they are accepted globally, not only inside loop bodies — a minor loosening of strict mode, chosen over a context-sensitive env, since `$item`/`$index` are legitimate names a strict config should not reject.

## Testing (per finding)

- **cmd-misc-1:** unit test that `buildRawRequestContext` includes a `request` map mirroring the top-level fields; a test that resolves the scaffold's route trigger expression (`{{ params.name }}`) and a `request.*` expression against a built context with no error. (Guards both the alias and the canonical rewrite.)
- **cmd-misc-2/3:** table tests for `isDatabaseService` (db→true, postgres→true, cache→false); auth_init finds a `postgres`-plugin service; migrate auto-detects a `db`-plugin service.
- **cmd-misc-4:** generate scoped CRUD; assert each route's trigger input contains `<ScopeParam>: {{ params.<ScopeParam> }}`, the create/update body input does NOT contain the scope column, and the update workflow's `data` excludes `id` and the scope column.
- **cmd-misc-5:** parse the `noda_get_examples` crud/auth output and assert the jwt_sign node has `secret` + `expiry` (not `expires_in`) and the schema ref is the object form; ideally run it through the config validator.
- **cmd-misc-6:** scaffolding into a dir with a pre-existing file returns an error and leaves files untouched (both MCP and `noda init`); `noda init --force` overwrites.
- **config-expr-1:** compile `{{ $item.x }}` / `{{ $index }}` under strict mode → no error (RED against current `knownContextEnv`).

Gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/... ./cmd/... ./plugins/core/...`.

## Mechanics

- Worktree `.worktrees/scaffold-runtime-alignment`, branch `feat/scaffold-runtime-alignment` off `main`.
- Subagent-driven execution per task: implementer → spec-compliance reviewer → code-quality reviewer.
- Spec + plan force-added to the branch.
- CHANGELOG "Fixed" entry (generated routes now run; auth-init/migrate accept both db plugin names; CRUD tenant scoping fixed; scaffold no longer overwrites).
- At merge: add a "Shipped 2026-07-05" note for these findings to `REVIEW-FINDINGS-2026-07-05.md` (on review PR #262's branch).

## Out of scope

`env`/`$env` remaining in `variableRe` (not a shipped-config break like `request`), other config-expr findings (config-expr-2..17), and any redesign of the scaffold template set beyond making it valid.
