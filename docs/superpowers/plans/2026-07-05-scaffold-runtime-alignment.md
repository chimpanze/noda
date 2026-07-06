# Scaffold / Runtime Contract Alignment (Tranche C) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 7 scaffold/runtime-mismatch findings from `REVIEW-FINDINGS-2026-07-05.md` (3 High, 4 Medium) so Noda's generators, examples, and CLI agree with the runtime — configs that pass `noda validate` must also run.

**Architecture:** Touches `internal/server` (route context), `internal/mcp` (scaffold + examples), `cmd/noda` (init/auth-init/migrate), `internal/generate` (CRUD), `internal/expr` (strict env). No public API break.

**Tech Stack:** Go (go1.25), Fiber v3, expr-lang, cobra, mcp-go.

## Global Constraints

- Go module floor: **go1.25**.
- cmd-misc-1: runtime gets a `request` alias on route triggers **and** the MCP generators emit canonical `params`/`body`. **Do NOT change WebSocket connection `channels.pattern` fields** — `request.*` is legitimately valid there (e.g. `examples.go:255`); only route `trigger.input` expressions change.
- cmd-misc-6: scaffolders refuse to overwrite existing files (all-or-nothing pre-check); `noda init` gets `--force`; the MCP scaffold has no force.
- CRUD scope value comes only from the URL path param, never the request body.
- jwt_sign real fields: required `claims` + `secret`; expiry key is `expiry`; secrets via `{{ secrets.X }}`. `$ref` is the object form `{"$ref": "schemas/Name"}`.
- All touched packages' tests run under `-race`.
- Pre-push gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/... ./cmd/... ./plugins/core/...`.

**Worktree:** `.worktrees/scaffold-runtime-alignment`, branch `feat/scaffold-runtime-alignment` off `main`. Spec + this plan force-added.

## File map

- `internal/server/trigger.go` — `request` alias in `buildRawRequestContext` (Task 1).
- `internal/mcp/tools.go` — `scaffoldSampleRoute` const canonicalized (Task 2); overwrite pre-check in `scaffoldProjectHandler` (Task 6).
- `internal/mcp/examples.go` — route-trigger `request.*` → canonical (Task 2); jwt_sign + `$ref` fixes (Task 5).
- `cmd/noda/dbservice.go` *(new)* — `isDatabaseService` (Task 3).
- `cmd/noda/auth_init.go`, `cmd/noda/migrate_service.go` — use `isDatabaseService` (Task 3).
- `internal/generate/crud.go` — scope threading (Task 4).
- `cmd/noda/init.go` — overwrite pre-check + `--force` (Task 6).
- `internal/expr/compiler.go` — `$item`/`$index` in `knownContextEnv` (Task 7).

---

### Task 1: `request` route-context alias (cmd-misc-1 runtime)

**Files:**
- Modify: `internal/server/trigger.go` (`buildRawRequestContext`)
- Test: `internal/server/trigger_test.go` (create if absent)

**Interfaces:**
- Produces: `buildRawRequestContext` output now contains a `request` key mirroring `body/params/query/headers/method/path` (+ `auth`).

- [ ] **Step 1: Write the failing test**

```go
// trigger_test.go — buildRawRequestContext is package-internal; test in package server.
func TestBuildRawRequestContext_HasRequestAlias(t *testing.T) {
	app := fiber.New()
	var got map[string]any
	app.Post("/x/:name", func(c fiber.Ctx) error {
		got = buildRawRequestContext(c)
		return c.SendStatus(200)
	})
	req := httptest.NewRequest("POST", "/x/alice?q=1", strings.NewReader(`{"k":"v"}`))
	req.Header.Set("Content-Type", "application/json")
	_, _ = app.Test(req)

	require.Contains(t, got, "request")
	reqMap, ok := got["request"].(map[string]any)
	require.True(t, ok)
	// request.params mirrors top-level params
	require.Equal(t, got["params"], reqMap["params"])
	require.Equal(t, got["body"], reqMap["body"])
	require.Equal(t, got["query"], reqMap["query"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestBuildRawRequestContext_HasRequestAlias`
Expected: FAIL — no `request` key.

- [ ] **Step 3: Add the alias**

In `buildRawRequestContext` (trigger.go), build the members once and add a `request` alias. Replace the map literal so the parsed values are reused:

```go
	body := parseBody(c)
	params := parseParams(c)
	query := parseQuery(c)
	headers := parseHeaders(c)
	method := c.Method()
	path := c.Path()

	ctx := map[string]any{
		"body": body, "params": params, "query": query,
		"headers": headers, "method": method, "path": path,
	}
	// request.* alias: unifies HTTP route triggers with WebSocket connection
	// channel patterns (where request.* is already valid) and matches the
	// namespace AI agents reach for. Same members as the top-level keys.
	request := map[string]any{
		"body": body, "params": params, "query": query,
		"headers": headers, "method": method, "path": path,
	}
	ctx["request"] = request
```

In the auth block below, add the auth map to BOTH: `ctx["auth"] = authMap` and `request["auth"] = authMap`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/server/ -run TestBuildRawRequestContext_HasRequestAlias`
Expected: PASS.

- [ ] **Step 5: Full server suite**

Run: `go test ./internal/server/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/server/trigger.go internal/server/trigger_test.go
git commit -m "fix(server): add request.* alias to route trigger context (cmd-misc-1)"
```

---

### Task 2: Canonical namespace in MCP generators (cmd-misc-1 generators)

**Files:**
- Modify: `internal/mcp/tools.go` (`scaffoldSampleRoute` const, line ~986)
- Modify: `internal/mcp/examples.go` (route-trigger `request.*` only)
- Test: `internal/mcp/*_test.go`

**Interfaces:** none; generated JSON changes.

- [ ] **Step 1: Write the failing test** — no generated ROUTE trigger input uses `request.`; connection patterns may.

```go
// In internal/mcp (a *_test.go file). Assumes helpers to get scaffold + examples output.
func TestGeneratedRouteTriggersUseCanonicalNamespace(t *testing.T) {
	// scaffoldSampleRoute is a const in this package.
	require.NotContains(t, scaffoldSampleRoute, "request.", "scaffold route trigger must use canonical params/body")
	// examples.go route triggers: check the crud/auth example strings the tool returns.
	for _, ex := range exampleConfigs() { // existing accessor used by noda_get_examples
		// route trigger inputs must not reference request.*; connection channel
		// patterns are exempt (request.* is valid there).
		assertNoRequestInRouteTriggers(t, ex)
	}
}
```

(If no `exampleConfigs()` accessor exists, assert on the specific example const/strings that `noda_get_examples` returns. The essential assertion: the scaffold route + the crud/auth/websocket-POST route trigger inputs contain no `request.` — while the WS `channels.pattern` string may.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestGeneratedRouteTriggersUseCanonicalNamespace`
Expected: FAIL — `scaffoldSampleRoute` and examples contain `request.`.

- [ ] **Step 3: Canonicalize route triggers only**

In `tools.go` `scaffoldSampleRoute` (line 986): `"{{ request.params.name }}"` → `"{{ params.name }}"`.

In `examples.go`, change ONLY route `trigger.input` expressions (these lines): 31, 32, 101, 102, 187, 188, 189, 272, 273 — `request.body.X` → `body.X`, `request.params.X` → `params.X`.

**DO NOT change line 255** (`"pattern": "board.{{ request.params.room_id }}"`) — that is a WebSocket connection `channels.pattern`, where `request.*` is valid. Verify by context (it's under a connection/`pattern` key, not a route `trigger.input`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/mcp/ -run TestGeneratedRouteTriggersUseCanonicalNamespace`
Expected: PASS.

- [ ] **Step 5: Full mcp suite**

Run: `go test ./internal/mcp/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/tools.go internal/mcp/examples.go internal/mcp/*_test.go
git commit -m "fix(mcp): canonical params/body in generated route triggers (cmd-misc-1)"
```

---

### Task 3: Shared database-service matcher (cmd-misc-2 + cmd-misc-3)

**Files:**
- Create: `cmd/noda/dbservice.go`
- Modify: `cmd/noda/auth_init.go` (`findServicesByPlugin` DB path), `cmd/noda/migrate_service.go` (`postgresServiceNames`)
- Test: `cmd/noda/dbservice_test.go`

**Interfaces:**
- Produces: `isDatabaseService(svc map[string]any) bool`.

- [ ] **Step 1: Write the failing test**

```go
// dbservice_test.go
func TestIsDatabaseService(t *testing.T) {
	require.True(t, isDatabaseService(map[string]any{"plugin": "db"}))
	require.True(t, isDatabaseService(map[string]any{"plugin": "postgres"}))
	require.False(t, isDatabaseService(map[string]any{"plugin": "cache"}))
	require.False(t, isDatabaseService(map[string]any{}))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/noda/ -run TestIsDatabaseService`
Expected: FAIL — undefined.

- [ ] **Step 3: Add helper and route both matchers through it**

`cmd/noda/dbservice.go`:

```go
package main

// isDatabaseService reports whether a service config uses the Postgres/db
// plugin. The db plugin's Name() is "postgres" and its Prefix() is "db";
// scaffolders and hand-written configs use either, so both are accepted.
func isDatabaseService(svc map[string]any) bool {
	p, _ := svc["plugin"].(string)
	return p == "db" || p == "postgres"
}
```

In `auth_init.go` `findServicesByPlugin`: when the requested `pluginName` is the database plugin, match via `isDatabaseService(svc)` instead of `svc["plugin"] != pluginName`. Simplest: add a branch — if `pluginName == "db"`, use `isDatabaseService(svc)`; else keep the exact `svc["plugin"] == pluginName` match (email path unchanged). Update the two DB error strings to say `plugin "db"/"postgres"`.

In `migrate_service.go` `postgresServiceNames`: replace `m["plugin"] == "postgres"` with `isDatabaseService(m)`.

- [ ] **Step 4: Write integration tests for both commands**

```go
// dbservice_test.go — auth init accepts a postgres-plugin service; migrate accepts a db-plugin service.
func TestFindServicesByPlugin_AcceptsPostgresForDB(t *testing.T) {
	services := map[string]any{"maindb": map[string]any{"plugin": "postgres", "config": map[string]any{"driver": "postgres"}}}
	names, _ := findServicesByPlugin(services, "db")
	require.Equal(t, []string{"maindb"}, names)
}

func TestPostgresServiceNames_AcceptsDBPlugin(t *testing.T) {
	services := map[string]any{"maindb": map[string]any{"plugin": "db", "config": map[string]any{"url": "postgres://x"}}}
	require.Equal(t, []string{"maindb"}, postgresServiceNames(services))
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./cmd/noda/ -run 'TestIsDatabaseService|TestFindServicesByPlugin_AcceptsPostgres|TestPostgresServiceNames_AcceptsDBPlugin' -race`
Expected: PASS. Then `go test ./cmd/noda/... -race` (existing auth_init/migrate tests stay green).

- [ ] **Step 6: Commit**

```bash
git add cmd/noda/dbservice.go cmd/noda/auth_init.go cmd/noda/migrate_service.go cmd/noda/dbservice_test.go
git commit -m "fix(cli): auth-init and migrate accept both db and postgres plugin names (cmd-misc-2, cmd-misc-3)"
```

---

### Task 4: CRUD tenant-scope threading (cmd-misc-4)

**Files:**
- Modify: `internal/generate/crud.go`
- Test: `internal/generate/crud_test.go`

**Interfaces:** none; generated route/workflow JSON changes when `ScopeCol`/`ScopeParam` set.

- [ ] **Step 1: Write the failing test**

`GenerateCRUD(model map[string]any, opts CRUDOptions) CRUDResult` — the model carries `table` + `columns` (a map); `CRUDResult.Files` is `map[string]map[string]any`; `CRUDOptions` fields are `Service, BasePath, Operations, Artifacts, ScopeCol, ScopeParam` (no Table/Columns). Singular of `items` is `item`.

```go
func TestGenerateCRUD_ScopeThreadedAndBodyExcluded(t *testing.T) {
	model := map[string]any{
		"table": "items",
		"columns": map[string]any{
			"id":    map[string]any{"type": "uuid", "primary_key": true},
			"org_id": map[string]any{"type": "uuid"},
			"title": map[string]any{"type": "text"},
		},
	}
	opts := CRUDOptions{
		BasePath: "/api/orgs/:org_id/items", Service: "maindb",
		Operations: []string{"create", "list", "get", "update", "delete"},
		Artifacts:  []string{"routes", "workflows"},
		ScopeCol:   "org_id", ScopeParam: "org_id",
	}
	res := GenerateCRUD(model, opts)

	// Every scoped route threads the scope param from the URL.
	for _, name := range []string{"routes/create-item.json", "routes/update-item.json", "routes/get-item.json", "routes/delete-item.json"} {
		input := res.Files[name]["trigger"].(map[string]any)["input"].(map[string]any)
		require.Equal(t, "{{ params.org_id }}", input["org_id"], name+" must thread scope from params")
	}
	// Create body input must NOT take the scope column from the body.
	createInput := res.Files["routes/create-item.json"]["trigger"].(map[string]any)["input"].(map[string]any)
	require.NotEqual(t, "{{ body.org_id }}", createInput["org_id"])

	// Update workflow data excludes id and the scope column, keeps title.
	nodes := res.Files["workflows/update-item.json"]["nodes"].(map[string]any)
	updateData := nodes["update"].(map[string]any)["config"].(map[string]any)["data"].(map[string]any)
	require.NotContains(t, updateData, "org_id")
	require.NotContains(t, updateData, "id")
	require.Contains(t, updateData, "title")
}
```

(Confirm the workflow's node map key for the update node is `"update"` per `generateUpdateWorkflow` — it is, crud.go:507. `Column`'s `PrimaryKey`/`Name` fields come from `parseColumns`; the body loops already reference `col.PrimaryKey`/`col.Name`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generate/ -run TestGenerateCRUD_ScopeThreadedAndBodyExcluded`
Expected: FAIL — scope param never threaded; `input.org_id` nil; update data is `expr("input")`.

- [ ] **Step 3: Thread the scope param, exclude it from the body, make update data explicit**

In `crud.go`, for each route that has `opts.ScopeCol != "" && opts.ScopeParam != ""`, add to the route's `inputMap`:

```go
	if opts.ScopeCol != "" && opts.ScopeParam != "" {
		inputMap[opts.ScopeParam] = expr("params." + opts.ScopeParam)
	}
```

Apply to create (after the body loop ~78), update (~168), get (~136 input map), delete (~195 input map), and list (~111 input map). For get/delete/list whose input is a literal `map[string]any{...}`, add the scope entry the same way.

In the create and update body loops (lines 73-78 and 157-158), add the scope column to the skip condition:

```go
		if col.PrimaryKey || col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" || col.Name == opts.ScopeCol {
			continue
		}
```

In `generateUpdateWorkflow` (crud.go:507-513), replace `"data": expr("input")` with an explicit data map of mutable body columns (excluding PK, timestamps, and the scope column). Add a `columns []Column` parameter if the function doesn't already receive them (it takes `table, singular, opts` — thread `columns` through from the caller at crud.go:95/generateUpdateWorkflow call site):

```go
	data := map[string]any{}
	for _, col := range columns {
		if col.PrimaryKey || col.Name == "created_at" || col.Name == "updated_at" || col.Name == "deleted_at" || col.Name == opts.ScopeCol {
			continue
		}
		data[col.Name] = expr("input." + col.Name)
	}
	nodes["update"] = map[string]any{
		"type": "db.update",
		"config": map[string]any{"table": table, "where": where, "data": data},
		"services": map[string]any{"database": opts.Service},
	}
```

(Also confirm `generateCreateWorkflow`'s `data[opts.ScopeCol] = expr("input."+opts.ScopeParam)` at ~357 still works — now `input.<ScopeParam>` resolves to the URL param, so the created row's scope column is set from the path. Keep it.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/generate/ -run TestGenerateCRUD_ScopeThreadedAndBodyExcluded -race`
Expected: PASS.

- [ ] **Step 5: Full generate suite**

Run: `go test ./internal/generate/... -race`
Expected: PASS (unscoped-CRUD tests unchanged).

- [ ] **Step 6: Commit**

```bash
git add internal/generate/crud.go internal/generate/crud_test.go
git commit -m "fix(generate): thread CRUD tenant scope from URL, keep it out of the body (cmd-misc-4)"
```

---

### Task 5: Valid MCP example configs (cmd-misc-5)

**Files:**
- Modify: `internal/mcp/examples.go` (jwt_sign + `$ref`)
- Test: `internal/mcp/*_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestExamples_JWTSignAndRefAreValid(t *testing.T) {
	// The jwt_sign example must include the required secret and use the "expiry" key.
	require.NotContains(t, authExampleString(), `"expires_in"`, "jwt_sign uses 'expiry', not 'expires_in'")
	require.Contains(t, authExampleString(), `"expiry"`)
	require.Contains(t, authExampleString(), `secrets.JWT_SECRET`)
	// Schema refs must be the object form, not the $ref(...) string.
	require.NotContains(t, crudExampleString(), `$ref(`)
}
```

(Use the accessors that back `noda_get_examples`; if the examples are Go string consts, assert on those consts directly.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestExamples_JWTSignAndRefAreValid`
Expected: FAIL — `expires_in`, missing secret, `$ref(...)` present.

- [ ] **Step 3: Fix the examples**

In `examples.go`:
- `"schema": "$ref(schemas/user.json)"` (line 42) → `"schema": map[string]any{"$ref": "schemas/User"}` (object form; if the example is a raw JSON string, write `"schema": {"$ref": "schemas/User"}`).
- The `util.jwt_sign` example config (lines 137-143): add `"secret": "{{ secrets.JWT_SECRET }}"`; rename `"expires_in": "24h"` → `"expiry": "24h"`.
- If any example uses `$env('...')` for a secret, change to `{{ secrets.X }}`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/mcp/ -run TestExamples_JWTSignAndRefAreValid`
Expected: PASS.

- [ ] **Step 5: Full mcp suite**

Run: `go test ./internal/mcp/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/mcp/examples.go internal/mcp/*_test.go
git commit -m "fix(mcp): valid jwt_sign (secret+expiry) and \$ref object form in examples (cmd-misc-5)"
```

---

### Task 6: Scaffold overwrite protection (cmd-misc-6)

**Files:**
- Modify: `cmd/noda/init.go` (`scaffoldProject` pre-check + `--force`), `internal/mcp/tools.go` (`scaffoldProjectHandler` pre-check)
- Test: `cmd/noda/init_test.go`, `internal/mcp/*_test.go`

**Interfaces:**
- `scaffoldProject(name string, force bool) error` (signature change).

- [ ] **Step 1: Write the failing test**

```go
// init_test.go
func TestScaffoldProject_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "app")
	require.NoError(t, os.MkdirAll(proj, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "noda.json"), []byte("{}"), 0644))

	err := scaffoldProject(proj, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "noda.json")
	// existing file untouched, no partial scaffold
	b, _ := os.ReadFile(filepath.Join(proj, "noda.json"))
	require.Equal(t, "{}", string(b))
}

func TestScaffoldProject_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "app")
	require.NoError(t, os.MkdirAll(proj, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "noda.json"), []byte("{}"), 0644))
	require.NoError(t, scaffoldProject(proj, true))
	b, _ := os.ReadFile(filepath.Join(proj, "noda.json"))
	require.NotEqual(t, "{}", string(b)) // overwritten with the template
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/noda/ -run TestScaffoldProject`
Expected: FAIL — `scaffoldProject` takes one arg and overwrites unconditionally.

- [ ] **Step 3: Add the pre-check + `--force`**

Change `scaffoldProject(name string)` → `scaffoldProject(name string, force bool)`. Before writing, when `!force`, walk the templates first and collect the output paths that already exist; if any exist, return `fmt.Errorf("refusing to overwrite existing files (use --force): %s", strings.Join(conflicts, ", "))` and write nothing. Then run the existing write walk. Update `newInitCmd` to add a `--force` bool flag and pass it:

```go
	var force bool
	cmd := &cobra.Command{ ... RunE: func(_ *cobra.Command, args []string) error {
		return scaffoldProject(args[0], force)
	}}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing files")
	return cmd
```

In `internal/mcp/tools.go` `scaffoldProjectHandler`: before the write loop (tools.go:781), stat each target `fullPath`; if any exists, return an mcp error result listing them and write nothing (no force option).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/noda/ -run TestScaffoldProject -race`
Expected: PASS. Fix any other `scaffoldProject(` call sites the signature change touches (e.g. `newInitCmd`) so the package builds.

- [ ] **Step 5: Full cmd + mcp suites**

Run: `go test -race ./cmd/noda/... ./internal/mcp/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/noda/init.go cmd/noda/init_test.go internal/mcp/tools.go internal/mcp/*_test.go
git commit -m "fix(scaffold): refuse to overwrite existing files; noda init --force (cmd-misc-6)"
```

---

### Task 7: Strict-mode `$item`/`$index` (config-expr-1)

**Files:**
- Modify: `internal/expr/compiler.go` (`knownContextEnv`)
- Test: `internal/expr/compiler_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestStrictMode_AllowsLoopVars(t *testing.T) {
	c := NewCompiler(WithStrictMode(true))
	_, err := c.Compile("{{ $item.value }}")
	require.NoError(t, err, "strict mode must accept $item")
	_, err = c.Compile("{{ $index }}")
	require.NoError(t, err, "strict mode must accept $index")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/expr/ -run TestStrictMode_AllowsLoopVars`
Expected: FAIL — `unknown name $item`.

- [ ] **Step 3: Add the loop vars to the strict env**

In `compiler.go` `knownContextEnv` (line 66):

```go
var knownContextEnv = map[string]any{
	"input":   map[string]any{},
	"auth":    map[string]any{},
	"trigger": map[string]any{},
	"nodes":   map[string]any{},
	"secrets": map[string]any{},
	"$item":   map[string]any{}, // loop/map/filter iteration value (control.loop, transform.map/filter)
	"$index":  0,                 // loop/map/filter iteration index
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/expr/ -run TestStrictMode_AllowsLoopVars`
Expected: PASS.

- [ ] **Step 5: Full expr suite**

Run: `go test ./internal/expr/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/expr/compiler.go internal/expr/compiler_test.go
git commit -m "fix(expr): accept \$item/\$index loop vars in strict mode (config-expr-1)"
```

---

### Task 8: CHANGELOG + full gate

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: CHANGELOG entry**

Add under `### Fixed`: "Scaffold/runtime alignment: generated route triggers now run (added a `request.*` alias and switched generators to canonical `params`/`body`); `noda auth init` and `noda migrate` accept both `db` and `postgres` plugin names; generated multi-tenant CRUD scopes by the URL path param (no cross-tenant write via request body); MCP example configs are valid (`util.jwt_sign` secret/expiry, `$ref` object form); `noda init` and the MCP scaffold refuse to overwrite existing files (`noda init --force` to override); strict expression mode accepts `$item`/`$index`."

- [ ] **Step 2: Full gate**

Run: `gofmt -l . && go vet ./... && golangci-lint run && go test -race ./internal/... ./cmd/... ./plugins/core/...`
Expected: clean, all pass. Fix any lint issues introduced by this branch (they block CI); leave pre-existing/unrelated ones (note them).

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(scaffold): changelog for scaffold/runtime alignment"
```

---

## Self-review notes

- **Spec coverage:** cmd-misc-1 → Tasks 1+2; cmd-misc-2/3 → Task 3; cmd-misc-4 → Task 4; cmd-misc-5 → Task 5; cmd-misc-6 → Task 6; config-expr-1 → Task 7; changelog/gate → Task 8. All seven covered.
- **Type consistency:** `isDatabaseService(map[string]any) bool` (Task 3) used by auth_init + migrate. `scaffoldProject(name string, force bool)` (Task 6) — update `newInitCmd` call site. `buildRawRequestContext` alias (Task 1) is what the canonical/`request.*` generators (Task 2) rely on at runtime. Tasks 2 and 5 both edit `examples.go` but disjoint lines (route triggers vs jwt/`$ref`); run in order.
- **Key precision:** Task 2 must NOT touch WS connection `channels.pattern` (`examples.go:255`) — only route `trigger.input`. `noda init` templates already use canonical form, so only the MCP generators change for cmd-misc-1.
- **Deferred (out of scope):** `env`/`$env` in `variableRe`; config-expr-2..17.
