# Follow-up Closeout (#339–#347) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close all nine open follow-up issues (#339–#347) from the #331/#332 tranches in one branch/PR. See `docs/superpowers/specs/2026-07-17-followup-closeout-design.md` for the decisions.

**Architecture:** Seven independent small tasks: Go resolver leniency, parseBody edges, flake fix, registry polish, validate-parity (editor+MCP), editor React pair, docs sweep. No task depends on another's code; they share only the branch.

**Tech Stack:** Go (fiber v3, testify), React/TypeScript (RJSF v6, vitest per editor CI), tools/docverify groundtruth dump.

## Global Constraints

- Branch: `feat/followup-closeout`, worktree `.worktrees/followup-closeout`, cut from `origin/main` after `git fetch`.
- Before every commit: `gofmt -l .` from the repo ROOT prints nothing; `go vet ./...` clean. For editor tasks: `cd editor && npm run lint && npm test -- --run && npm run build` (match the CI job's scripts — check editor/package.json for exact names).
- Commit messages end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- gopls diagnostics inside `.worktrees/` are noise; trust real `go build`/`go test`.
- Issue references: each task's commit message names the issue(s) it closes.

---

### Task 1: #340 — int resolvers accept numeric strings

**Files:**
- Modify: `internal/plugin/resolve.go` (`ResolveOptionalInt` ~line 124, `ResolveRawInt` ~line 185, `ToInt64` ~line 172)
- Test: `internal/plugin/resolve_test.go` (follow existing table idiom)
- Modify: `CHANGELOG.md` `[Unreleased]` `### Changed`

**Interfaces:** signatures unchanged; behavior widened.

- [ ] **Step 1: Write failing tests** — table cases: `ResolveOptionalInt` with config value `"20"` (literal numeric string) → `(20, true, nil)`; with an expression resolving to `"20"` (use the package's existing fake ExecutionContext idiom) → `(20, true, nil)`; with value resolving to `"abc"` → error still contains `expected int`; `ResolveRawInt` same three shapes; `ToInt64("42")` → `(42, true)`; `ToInt64("x")` → `(0, false)`.
- [ ] **Step 2: Run** `go test ./internal/plugin/ -run 'ResolveOptionalInt|ResolveRawInt|ToInt64' -v` → new cases FAIL (string → error / false today).
- [ ] **Step 3: Implement** — in `ResolveOptionalInt`'s inner `switch n := val.(type)`, replace the fallthrough error with: try `ToInt(val)`; if ok return it; else keep the existing `fmt.Errorf("field %q resolved to %T, expected int", ...)`. Same pattern in `ResolveRawInt`. Add to `ToInt64`: `case string:` parse via `fmt.Sscanf(n, "%d", &i)` returning `(i, true)` on success (mirror `ToInt`). Update the three doc comments ("numeric strings accepted").
- [ ] **Step 4: Run** package tests → PASS. Also `go test ./plugins/db/ ./plugins/core/upload/ ./internal/server/ -count=1` (nearest consumers).
- [ ] **Step 5: CHANGELOG** `### Changed`: `- Int-typed node config fields (db.find limit/offset, upload.handle max_size, image dimensions, …) now accept numeric strings — {{ query.limit ?? '20' }}-style computed defaults work without toInt(...) (#340).`
- [ ] **Step 6: Commit** `fix(plugin): int-typed resolvers accept numeric strings (#340)`.

### Task 2: #339 — parseBody edge coverage

**Files:**
- Modify: `internal/server/trigger.go` (`parseBody`) only if the uppercase-multipart test fails
- Test: `internal/server/trigger_test.go` (reuse `triggerTestRaw` helper)

- [ ] **Step 1: Write the two tests** — (a) uppercase multipart: build a real multipart body with `mime/multipart.Writer`, send with Content-Type header uppercased (`MULTIPART/FORM-DATA; boundary=...` — uppercase the type only, keep the boundary token verbatim), map `{{ body.field }}`, expect the field value; (b) partial-parse: `triggerTestRaw` with body `a=1&b=%zz`, content type `application/x-www-form-urlencoded`, input `{"a": "{{ body.a }}"}` → expect `1` (coerced form value).
- [ ] **Step 2: Run them.** (b) must PASS already (regression pin for the `len(values) > 0` gate). (a) is empirical: if PASS, fasthttp handles uppercase multipart → keep the test as a pin, done. If FAIL:
- [ ] **Step 3 (only if (a) failed): Fix** — in `parseBody`'s multipart path, when `c.MultipartForm()` errors/returns nil but the lowercased Content-Type contains `multipart`, parse manually: `mediatype, params, err := mime.ParseMediaType(contentType)`; if `err == nil && strings.HasPrefix(mediatype, "multipart/")`, `mr := multipart.NewReader(bytes.NewReader(body), params["boundary"])`, read parts into the same `form` map shape (values as `[]any` when repeated, matching the urlencoded convention). Keep the raw-string fallback as the error path.
- [ ] **Step 4: Run** `go test ./internal/server/ -count=1` → PASS.
- [ ] **Step 5: Commit** `test(server): parseBody uppercase-multipart + partial-parse coverage (#339)` (or `fix(server): …` if Step 3 ran — say which in the report).

### Task 3: #347 — de-flake TestWatcher_Debounce

**Files:**
- Test: `internal/devmode/devmode_test.go:78-102`

- [ ] **Step 1: Rework the test** — set `w.debounce = 500 * time.Millisecond`; write the 5 files at 5 ms intervals (write window 25 ms ≪ 500 ms debounce, so only a runner stall > 475 ms can double-fire vs the old > 80 ms); replace the fixed `time.Sleep(300ms)+assert` with `require.Eventually(t, func() bool { return called.Load() >= 1 }, 3*time.Second, 20*time.Millisecond)` then `time.Sleep(700 * time.Millisecond)` (one more debounce window + margin) and `assert.Equal(t, int32(1), called.Load())`.
- [ ] **Step 2: Stress** `go test ./internal/devmode/ -run TestWatcher_Debounce -count=30` → 30/30 PASS.
- [ ] **Step 3: Commit** `test(devmode): widen debounce margins so TestWatcher_Debounce survives loaded runners (#347)`.

### Task 4: #346 — registry validator polish

**Files:**
- Modify: `internal/registry/configschema.go` (`checkVocab` type case), `internal/registry/validator.go` (staticFieldsByNodeType)
- Test: `internal/registry/configschema_test.go`

- [ ] **Step 1: Failing tests** — (a) `CheckSchemaVocabulary` on `{"type": "integr"}` → error containing `unknown type "integr"`; on `{"type": []any{"integer", "strng"}}` → error; all seven valid names (`object,array,string,boolean,null,number,integer`) still pass; (b) vocab schema with `{"oneOf": []any{ {"type":"object","properties":{...},"minimum":1} }}` → error naming `minimum` (pins recursion into branches).
- [ ] **Step 2: Run** → (a) FAILS (shape ok, name unchecked), (b) may already pass — if so it's a pin, note it.
- [ ] **Step 3: Implement** — in `checkVocab`'s `case k == "type"` branch, check each name against a `knownTypeNames` set (the `matchesType` vocabulary); error `schema keyword "type" at %q: unknown type %q`.
- [ ] **Step 4: Delete** the `"transform.merge": {"mode", "type"}` entry's `"type"` element in `internal/registry/validator.go` `staticFieldsByNodeType` (keep `"mode"`), with a one-line comment: `match.type is nested; single-segment static lookup never matched it and strict root keys reject a top-level "type"`.
- [ ] **Step 5: Run** `go test ./internal/registry/ ./cmd/noda/ -count=1` → PASS (vocab guard over all 81 schemas must stay green).
- [ ] **Step 6: Commit** `feat(registry): type-name vocabulary check, oneOf-branch vocab pin, drop dead static-field entry (#346)`.

### Task 5: #345 — validate parity (editor + MCP)

**Files:**
- Modify: `internal/server/editor_validation.go` (`validateAll`, `validateFile`), `internal/mcp/tools.go` (validate handler ~line 690)
- Test: `internal/server/editor_validation_test.go` (or the file's existing test home), `internal/mcp/tools_test.go`

**Interfaces:** consumes `registry.CollectDeferredServices(rc)`, `registry.ValidateStartupDryRun(rc, plugins, nodes, compiler, deferred)`; EditorAPI fields `e.plugins`, `e.nodes`, `e.compiler` (already populated — internal/server/editor.go:14-24); MCP's own `corePlugins()` (internal/mcp/plugins.go:31).

- [ ] **Step 1: Failing tests** — editor: POST `/validate/all` against a fixture project whose workflow has a node config violating an audited schema (e.g. a `response.error` node with config `{}` — missing required `code`/`message`) but passing file-level schema validation → response must be `valid:false` with an error mentioning `missing required config field`. Follow the file's existing test-server setup idiom (EditorAPI has an `rc` static-fallback field for tests — check how sibling editor tests construct it). MCP: call the validate handler on a temp dir copy of the same fixture → `valid:false` with the same error class.
- [ ] **Step 2: Run** → both FAIL (today they return valid:true).
- [ ] **Step 3: Implement editor** — in `validateAll` (and the shared path of `validateFile`), when `config.ValidateAll` returns a non-nil rc with zero errors AND `e.plugins != nil && e.nodes != nil && e.compiler != nil`: `deferred, dErrs := registry.CollectDeferredServices(rc)`, append `dErrs` and `registry.ValidateStartupDryRun(rc, e.plugins, e.nodes, e.compiler, deferred)` results into the same error list (`file` best-effort: the workflow name is in the error string; use `"file": ""` and put the full message in `message`). Keep 200-shape identical.
- [ ] **Step 4: Implement MCP** — after the existing `valid:true` path's prerequisites (rc built, no errs): `plugins := registry.NewPluginRegistry()`, register `corePlugins()` (mirror how internal/mcp builds its list; on register error return a tool error), `_, bootErrs := registry.Bootstrap(ctx, rc, plugins, registry.BootstrapOptions{DryRun: true})`, and fold `bootErrs` into the same `valid:false`/`errors` JSON shape (message-only entries).
- [ ] **Step 5: Run** `go test ./internal/server/ ./internal/mcp/ ./cmd/noda/ -count=1` → PASS (the cmd/noda projects gate proves examples still validate through the widened paths — MCP tools_test references examples/ too).
- [ ] **Step 6: Commit** `feat(server,mcp): validate endpoints run the DryRun startup validation like noda validate (#345)`.

### Task 6: #341 + #344 — editor pair (+ auth branch titles)

**Files:**
- Modify: `editor/src/components/views/RouteFormPanel.tsx` (~line 350, after the Raw Body block), `editor/src/components/views/RoutesView.tsx` (~line 204 clean logic), `editor/src/components/panels/NodeConfigPanel.tsx` (~line 132 uiSchema builder)
- Modify: `plugins/auth/get_user.go`, `plugins/auth/revoke_session.go`, `plugins/auth/set_password.go` (add `"title"` to each oneOf branch map)
- Test: editor test files colocated per the repo's vitest idiom (check `editor/src/**/*.test.tsx` for the pattern; if RouteFormPanel/NodeConfigPanel have no existing tests, add targeted ones only if a test harness for components exists — otherwise rely on lint/build + a plugins/auth Go test asserting branch titles)

- [ ] **Step 1: Coerce checkbox** — after the Raw Body block in RouteFormPanel.tsx add the same `<label><input type="checkbox"/></label>` idiom: label `Coerce numeric inputs`, `checked={route.trigger?.coerce !== false}`, `onChange={(e) => updateTrigger({ coerce: e.target.checked ? undefined : false })}`. In RoutesView.tsx clean logic add: `if (clean.trigger.coerce !== false) delete clean.trigger.coerce;`. Add `coerce?: boolean;` to the trigger type at RouteFormPanel.tsx:36.
- [ ] **Step 2: Root-oneOf uiSchema** — in NodeConfigPanel.tsx, before the `if (schema?.properties)` loop, derive the property map: `const schemaProps = schema?.properties ?? (Array.isArray(schema?.oneOf) ? Object.assign({}, ...schema.oneOf.map((b: Record<string, unknown>) => (b as { properties?: object }).properties ?? {})) : undefined);` and iterate `schemaProps` instead of `schema.properties` (keep all widget-assignment rules unchanged).
- [ ] **Step 3: Branch titles** — in the three auth descriptors, each oneOf branch map gains `"title"`: get_user: `"By user_id"`/`"By email"`; revoke_session: `"By token"`/`"By session_id"`/`"By user_id"`; set_password: `"By user_id"`/`"By reset token"`. Extend each node's entry in `plugins/auth/schema_audit_test.go` is NOT needed (annotation keyword, vocabulary-legal — but run the vocab guard to prove it).
- [ ] **Step 4: Gates** — `go test ./plugins/auth/ ./cmd/noda/ -count=1` (vocab guard green with titles); `cd editor && npm run lint && npm test -- --run && npm run build` all green; `gofmt -l .` empty.
- [ ] **Step 5: Commit** `feat(editor): trigger.coerce toggle + root-oneOf config forms; auth branch titles (#341, #344)`.

### Task 7: #342 + #343 — docs sweep

**Files:**
- Modify: `docs/05-examples/rest-api.md` (rows bullet: `db.query` → `db.find`, raw-SQL snippet → config-level limit/offset matching `examples/rest-api/workflows/list-tasks.json`)
- Modify: `docs/03-nodes/*.md` (Required/type cells that disagree with audited schemas), incl. replacing `db.insert` with `db.create` in `util.timestamp.md`, `util.uuid.md`, `upload.handle.md`, `docs/04-guides/testing-and-debugging.md` (~line 355)
- Tooling: `go run ./tools/docverify/groundtruth` (check its README/flags — it dumps every node's ConfigSchema incl. `required`)

- [ ] **Step 1: Dump ground truth** — run the groundtruth tool; for each of the 81 node types extract `required` + property types.
- [ ] **Step 2: Sweep** — for every `docs/03-nodes/<type>.md` field table, compare the Required column and type column against the dump; fix mismatches (the audited schema is truth; where a field is required-by-misbehavior the docs may keep "yes" ONLY if the schema also says required — the schema decides). Record every changed cell in the report (file, field, old→new).
- [ ] **Step 3: db.insert snippets** — replace the four illustrative `db.insert` occurrences with `db.create` (verify each snippet's config keys match db.create's schema: `table`, `data`).
- [ ] **Step 4: rest-api.md** — fix the rows-node description per #342.
- [ ] **Step 5: Gate** — if `tools/docverify/snippets` (the snippet validator) has a runnable mode, run it over the touched docs and report; extend it for node-type existence ONLY if it's a ≤30-line change, else note-and-skip in the report. `go build ./...` still clean.
- [ ] **Step 6: Commit** `docs: 03-nodes required/type sweep vs audited schemas; db.insert→db.create snippets; rest-api rows node (#342, #343)`.

---

### Task 8 (controller): rebase check, full suite, final review, PR

- [ ] `git fetch origin main`; rebase/merge if main moved.
- [ ] `go build ./... && go test ./... -count=1` + `gofmt -l .` + editor CI trio.
- [ ] Final whole-branch review (most capable model) with all deferred minors; fix wave if needed.
- [ ] `git add -f` this spec+plan; push; `gh pr create` — body lists per-issue outcomes; footer `Closes #339 … Closes #347` (nine `Closes` lines) + `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
