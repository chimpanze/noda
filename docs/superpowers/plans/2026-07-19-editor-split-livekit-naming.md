# Editor API Extraction + Livekit Snake_Case Rename — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship two independent cleanups from the 2026-07-19 clean-code review: (1) rename the livekit plugin's 17 camelCase node names to snake_case (breaking, clean break, no aliases), and (2) extract the 2,400-line editor API out of `internal/server` into a new `internal/editor` package, with shared helpers landing in a new leaf package `internal/routecfg`.

**Architecture:** Two PRs from one plan; they share zero files. Part A is a mechanical word-boundary rename across `plugins/livekit`, `docs/`, `examples/`, `testdata/` plus 17 doc-file renames and a Breaking CHANGELOG entry. Part B first creates `internal/routecfg` (holding `NormalizeRoutes` + `ExtractMiddlewareConfig`, the two helpers used by BOTH server and editor), then moves the seven `editor_*.go` files and all their tests into `internal/editor` (`EditorAPI`→`editor.API`, `RegisterEditorUI` method→`editor.RegisterUI` free function).

**Tech Stack:** Go, gofiber/fiber/v3, testify. Spec: `docs/superpowers/specs/2026-07-19-editor-split-livekit-naming-design.md`.

## Global Constraints

- Two PRs: Part A and Part B are separate branches and separate PRs. Part A ships on the existing branch (renamed to `livekit-snake-case`), which also carries the spec + this plan. Part B branches from `main` as `editor-api-extract`.
- Livekit `lk` prefix stays; ONLY the casing of node names changes. `lk.token` is already snake_case-conformant and must not change.
- No aliases, no deprecation shim: old camelCase names must fail validation as unknown node types after Part A.
- Part B is a pure refactor: identical routes (`/_noda/*`, `/editor/*`), identical handler logic, no behavior change, NO CHANGELOG entry.
- Go internal symbols (e.g. `roomCreateDescriptor`) stay camelCase per Go convention — the rename only touches node-name strings.
- Before every commit: `gofmt -l .` must print nothing (CI's golangci gofmt is stricter than vet — known gotcha).
- Commit trailer: `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. PR body trailer: `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.

### The 17 renames (Part A reference table — also goes in CHANGELOG verbatim)

| Old | New |
|---|---|
| `lk.roomCreate` | `lk.room_create` |
| `lk.roomList` | `lk.room_list` |
| `lk.roomDelete` | `lk.room_delete` |
| `lk.roomUpdateMetadata` | `lk.room_update_metadata` |
| `lk.participantGet` | `lk.participant_get` |
| `lk.participantList` | `lk.participant_list` |
| `lk.participantRemove` | `lk.participant_remove` |
| `lk.participantUpdate` | `lk.participant_update` |
| `lk.muteTrack` | `lk.mute_track` |
| `lk.sendData` | `lk.send_data` |
| `lk.ingressCreate` | `lk.ingress_create` |
| `lk.ingressDelete` | `lk.ingress_delete` |
| `lk.ingressList` | `lk.ingress_list` |
| `lk.egressStartRoomComposite` | `lk.egress_start_room_composite` |
| `lk.egressStartTrack` | `lk.egress_start_track` |
| `lk.egressStop` | `lk.egress_stop` |
| `lk.egressList` | `lk.egress_list` |

The rename pairs as a shell variable, used by several tasks (word-boundary `\b` protects Go identifiers like `roomCreateDescriptor` — `\broomCreate\b` does not match it):

```bash
PAIRS="roomCreate:room_create roomList:room_list roomDelete:room_delete roomUpdateMetadata:room_update_metadata participantGet:participant_get participantList:participant_list participantRemove:participant_remove participantUpdate:participant_update muteTrack:mute_track sendData:send_data ingressCreate:ingress_create ingressDelete:ingress_delete ingressList:ingress_list egressStartRoomComposite:egress_start_room_composite egressStartTrack:egress_start_track egressStop:egress_stop egressList:egress_list"
```

---

# Part A — Livekit snake_case rename (branch `livekit-snake-case`, PR 1)

### Task A1: Rename node names in `plugins/livekit`

**Files:**
- Modify: every `plugins/livekit/*.go` containing a camelCase node-name string — descriptor `Name()` returns (e.g. `plugins/livekit/room_create.go:14`) and `fmt.Errorf("lk.roomCreate: …")` error prefixes. Test files in the same directory pin the old names and are updated by the same sweep.

**Interfaces:**
- Consumes: nothing.
- Produces: registry node names `lk.room_create` … `lk.egress_list` (see Global Constraints table). Task A2's docs/examples must match these exactly.

- [ ] **Step 1: Rename the current branch (it carries the spec/plan; Part A ships on it)**

```bash
git branch -m editor-split-livekit-naming livekit-snake-case
```

- [ ] **Step 2: Confirm current test expectations pin camelCase (the "failing test" baseline)**

Run: `grep -rn 'roomCreate' plugins/livekit/descriptors_test.go | head -3`
Expected: matches showing `"roomCreate"` — proof the tests will catch a botched rename.

- [ ] **Step 3: Apply the word-boundary rename across plugins/livekit**

(`PAIRS` = the exact string from the Global Constraints table.)

```bash
PAIRS="roomCreate:room_create roomList:room_list roomDelete:room_delete roomUpdateMetadata:room_update_metadata participantGet:participant_get participantList:participant_list participantRemove:participant_remove participantUpdate:participant_update muteTrack:mute_track sendData:send_data ingressCreate:ingress_create ingressDelete:ingress_delete ingressList:ingress_list egressStartRoomComposite:egress_start_room_composite egressStartTrack:egress_start_track egressStop:egress_stop egressList:egress_list"
for p in $PAIRS; do
  old=${p%%:*}; new=${p##*:}
  perl -pi -e "s/\\b${old}\\b/${new}/g" plugins/livekit/*.go
done
gofmt -l plugins/livekit
```

Expected: `gofmt -l` prints nothing.

- [ ] **Step 4: Verify no camelCase node names remain in the plugin**

Run: `grep -rnE 'roomCreate|roomList|roomDelete|roomUpdateMetadata|participantGet|participantList|participantRemove|participantUpdate|muteTrack|sendData|ingressCreate|ingressDelete|ingressList|egressStartRoomComposite|egressStartTrack|egressStop|egressList' plugins/livekit/`
Expected: no output. (Go identifiers like `roomCreateDescriptor` WILL still match this grep — that is expected and correct; check that every remaining hit is a Go identifier, i.e. immediately followed by a letter such as `Descriptor`/`Executor`. If the grep output is only identifier hits, pass.)

Better precision check: `grep -rnE '"(roomCreate|roomList|roomDelete|roomUpdateMetadata|participantGet|participantList|participantRemove|participantUpdate|muteTrack|sendData|ingressCreate|ingressDelete|ingressList|egressStartRoomComposite|egressStartTrack|egressStop|egressList)"|lk\.(roomCreate|roomList|roomDelete|roomUpdateMetadata|participantGet|participantList|participantRemove|participantUpdate|muteTrack|sendData|ingressCreate|ingressDelete|ingressList|egressStartRoomComposite|egressStartTrack|egressStop|egressList)' plugins/livekit/`
Expected: no output at all.

- [ ] **Step 5: Run the plugin tests**

Run: `go test ./plugins/livekit/`
Expected: PASS (descriptor tests, schema audit, nodes tests all updated by the same sweep).

- [ ] **Step 6: Commit**

```bash
git add plugins/livekit
git commit -m "refactor(livekit)!: rename node types to snake_case (lk.roomCreate -> lk.room_create etc.)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task A2: Rename doc files and sweep docs/examples/testdata

**Files:**
- Rename: 17 of the 18 `docs/03-nodes/lk.*.md` files (`lk.token.md` stays).
- Modify: every file under `docs/`, `examples/`, `testdata/` containing an old name (63 files at plan time, including `examples/node-cookbook/livekit/` workflows, `docs/05-examples/video-conferencing.md`, `docs/01-getting-started/services.md`).

**Interfaces:**
- Consumes: the new node names from Task A1.
- Produces: docs pages named `docs/03-nodes/lk.room_create.md` etc. — `TestCookbookCoverage` requires each registered node name to have a matching docs page linking its cookbook example.

- [ ] **Step 1: git mv the 17 doc files**

(`PAIRS` = the exact string from Global Constraints; re-set it in this shell if starting fresh.)

```bash
cd docs/03-nodes
PAIRS="roomCreate:room_create roomList:room_list roomDelete:room_delete roomUpdateMetadata:room_update_metadata participantGet:participant_get participantList:participant_list participantRemove:participant_remove participantUpdate:participant_update muteTrack:mute_track sendData:send_data ingressCreate:ingress_create ingressDelete:ingress_delete ingressList:ingress_list egressStartRoomComposite:egress_start_room_composite egressStartTrack:egress_start_track egressStop:egress_stop egressList:egress_list"
for p in $PAIRS; do
  old=${p%%:*}; new=${p##*:}
  git mv "lk.${old}.md" "lk.${new}.md"
done
cd ../..
```

- [ ] **Step 2: Sweep content across docs, examples, testdata**

Workflow JSON in these trees may also use the camelCase name as a node *ID* and in expressions referencing that ID; the whole-file sweep renames definition and references together, which keeps them consistent.

```bash
for p in $PAIRS; do
  old=${p%%:*}; new=${p##*:}
  files=$(grep -rl "\b${old}\b" docs examples testdata 2>/dev/null || true)
  [ -n "$files" ] && perl -pi -e "s/\\b${old}\\b/${new}/g" $files
done
```

- [ ] **Step 3: Verify zero old names in the swept trees**

Run: `grep -rnE 'roomCreate|roomList|roomDelete|roomUpdateMetadata|participantGet|participantList|participantRemove|participantUpdate|muteTrack|sendData|ingressCreate|ingressDelete|ingressList|egressStartRoomComposite|egressStartTrack|egressStop|egressList' docs examples testdata`
Expected: no output.

- [ ] **Step 4: Run the cookbook coverage gate and livekit-adjacent validation**

Run: `go test ./internal/testing/cookbook/ -run TestCookbookCoverage -v`
Expected: PASS — every registered node (now snake_case) maps to a docs page and cookbook example. (The full livekit cookbook e2e needs a real LiveKit service and runs in CI; the coverage gate is the local proxy.)

Also run (the livekit service config `$env()`-resolves three variables, so set dummies; validation checks node types regardless):

```bash
go build -o /tmp/noda-plan ./cmd/noda
LIVEKIT_URL=ws://localhost:7880 LIVEKIT_API_KEY=devkey LIVEKIT_API_SECRET=devsecret1234567890 \
  /tmp/noda-plan validate --config examples/node-cookbook/livekit
```

Expected: `✓ All config files valid (37 files checked)` — proves the renamed configs reference known node types. (Verified at plan time that this exact invocation passes on main with the old names.)

- [ ] **Step 5: Commit**

```bash
git add docs examples testdata
git commit -m "docs(livekit): rename node docs and example references to snake_case

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task A3: CHANGELOG entry, full verification, PR

**Files:**
- Modify: `CHANGELOG.md` (top of `## [Unreleased]`, new `### Changed` entry — check whether `[Unreleased]` already has a `### Changed` subsection and append to it if so; never create a duplicate subsection heading).

**Interfaces:**
- Consumes: the mapping table from Global Constraints (verbatim).
- Produces: PR 1.

- [ ] **Step 1: Add the Breaking CHANGELOG entry**

Under `## [Unreleased]` → `### Changed` (create or append):

```markdown
- **Breaking:** livekit node types renamed to snake_case for consistency with every other plugin (`lk` prefix unchanged, `lk.token` unchanged). There are no aliases — old names now fail validation as unknown node types. Full mapping:

  | Old | New |
  |---|---|
  | `lk.roomCreate` | `lk.room_create` |
  | `lk.roomList` | `lk.room_list` |
  | `lk.roomDelete` | `lk.room_delete` |
  | `lk.roomUpdateMetadata` | `lk.room_update_metadata` |
  | `lk.participantGet` | `lk.participant_get` |
  | `lk.participantList` | `lk.participant_list` |
  | `lk.participantRemove` | `lk.participant_remove` |
  | `lk.participantUpdate` | `lk.participant_update` |
  | `lk.muteTrack` | `lk.mute_track` |
  | `lk.sendData` | `lk.send_data` |
  | `lk.ingressCreate` | `lk.ingress_create` |
  | `lk.ingressDelete` | `lk.ingress_delete` |
  | `lk.ingressList` | `lk.ingress_list` |
  | `lk.egressStartRoomComposite` | `lk.egress_start_room_composite` |
  | `lk.egressStartTrack` | `lk.egress_start_track` |
  | `lk.egressStop` | `lk.egress_stop` |
  | `lk.egressList` | `lk.egress_list` |
```

- [ ] **Step 2: Repo-wide grep-zero + full verification**

```bash
grep -rnE '"(roomCreate|roomList|roomDelete|roomUpdateMetadata|participantGet|participantList|participantRemove|participantUpdate|muteTrack|sendData|ingressCreate|ingressDelete|ingressList|egressStartRoomComposite|egressStartTrack|egressStop|egressList)"|lk\.(roomCreate|roomList|roomDelete|roomUpdateMetadata|participantGet|participantList|participantRemove|participantUpdate|muteTrack|sendData|ingressCreate|ingressDelete|ingressList|egressStartRoomComposite|egressStartTrack|egressStop|egressList)' --include='*.go' --include='*.md' --include='*.json' -r . | grep -v CHANGELOG.md | grep -v docs/superpowers
```

Expected: no output (CHANGELOG and the spec/plan intentionally keep the old names in their mapping tables). Then:

```bash
go build ./... && go vet ./... && gofmt -l .
go test ./...
```

Expected: build/vet clean, gofmt silent, tests pass. Known flake: `TestWatcher_Debounce` (internal/devmode, issue #347) may fail under load on an unrelated PR — a failure there alone is noise; rerun to confirm.

- [ ] **Step 3: Commit and open PR 1**

```bash
git add CHANGELOG.md
git commit -m "docs: CHANGELOG breaking entry for livekit snake_case rename

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
git push -u origin livekit-snake-case
gh pr create --title "refactor(livekit)!: snake_case node names — the last naming outlier" --body "$(cat <<'EOF'
## Summary
- Renames the livekit plugin's 17 camelCase node types to snake_case (`lk.roomCreate` → `lk.room_create`, full table in CHANGELOG); `lk` prefix and `lk.token` unchanged
- Clean break per 0.0.x convention: no aliases — old names fail validation as unknown node types
- Docs pages renamed, all references in docs/examples/testdata updated; cookbook coverage gate + livekit cookbook CI prove the rename end-to-end
- Carries the design spec + implementation plan for this and the follow-up editor-API extraction PR

## Breaking change
Any config using the old camelCase names must apply the CHANGELOG mapping table (this is also the Homebase migration recipe — it is pinned to a tagged image, so nothing breaks until its next upgrade).

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR opens; the livekit cookbook CI job runs against a real LiveKit service and must go green.

---

# Part B — Editor API extraction (branch `editor-api-extract` from `main`, PR 2)

### Task B1: Create leaf package `internal/routecfg`

**Files:**
- Create: `internal/routecfg/routecfg.go`, `internal/routecfg/routecfg_test.go`
- Modify: `internal/server/middleware.go` (delete `middlewareConfigPaths` var + `extractMiddlewareConfig` func at lines ~98–133; rewire ~3 call sites), `internal/server/session_middleware.go`, `internal/server/validate_middleware.go` (call sites), `internal/server/editor_nodes.go:277`, `internal/server/openapi.go:66`, `internal/server/editor_codegen.go` (delete `normalizeRoutes` at lines 428–443; rewire line 291), `internal/server/editor_helpers_test.go` (remove the three `TestNormalizeRoutes_*` tests), `internal/server/coverage_test.go` (move the `extractMiddlewareConfig` branch tests, section starting ~line 28)

**Interfaces:**
- Consumes: nothing (leaf package — imports nothing from this module).
- Produces: `routecfg.NormalizeRoutes(data map[string]any) []map[string]any` and `routecfg.ExtractMiddlewareConfig(name string, rootConfig map[string]any) map[string]any`. Tasks B2/B3 and `internal/server` compile against these exact names.

- [ ] **Step 1: Create the branch from main**

```bash
git fetch origin && git switch -c editor-api-extract origin/main
```

- [ ] **Step 2: Write the package with tests first**

`internal/routecfg/routecfg.go` — the two function bodies are verbatim moves from `internal/server/editor_codegen.go:428-443` and `internal/server/middleware.go:98-133`, renamed to exported:

```go
// Package routecfg holds pure helpers over route and middleware config maps,
// shared by the HTTP server (OpenAPI generation, middleware setup) and the
// editor API (codegen, middleware listing).
package routecfg

// NormalizeRoutes returns the route objects in a route file, which can be a
// single route object or a group file with routes under arbitrary keys.
func NormalizeRoutes(data map[string]any) []map[string]any {
	if _, hasMethod := data["method"]; hasMethod {
		return []map[string]any{data}
	}
	var routes []map[string]any
	for _, v := range data {
		if rm, ok := v.(map[string]any); ok {
			if _, hasMethod := rm["method"]; hasMethod {
				routes = append(routes, rm)
			}
		}
	}
	return routes
}

// middlewareConfigPaths maps middleware names to alternative config lookup paths.
// Each path is a sequence of nested keys in the root config.
// The "middleware" section is always checked first for all middleware.
var middlewareConfigPaths = map[string][]string{
	"security.cors":    {"security", "cors"},
	"security.headers": {"security", "headers"},
	"security.csrf":    {"security", "csrf"},
	"auth.jwt":         {"security", "jwt"},
	"auth.oidc":        {"security", "oidc"},
	"auth.session":     {"security", "session"},
	"casbin.enforce":   {"security", "casbin"},
	"livekit.webhook":  {"security", "livekit"},
}

// ExtractMiddlewareConfig extracts the config block for a specific middleware.
func ExtractMiddlewareConfig(name string, rootConfig map[string]any) map[string]any {
	if mw, ok := rootConfig["middleware"].(map[string]any); ok {
		if cfg, ok := mw[name].(map[string]any); ok {
			return cfg
		}
	}
	if path, ok := middlewareConfigPaths[name]; ok {
		cfg := rootConfig
		for _, key := range path {
			next, ok := cfg[key].(map[string]any)
			if !ok {
				return nil
			}
			cfg = next
		}
		return cfg
	}
	return nil
}
```

`internal/routecfg/routecfg_test.go` — move the three `TestNormalizeRoutes_*` tests verbatim from `internal/server/editor_helpers_test.go:15-35` and the `extractMiddlewareConfig` branch tests from `internal/server/coverage_test.go` (section `// --- Middleware: extractMiddlewareConfig branches ---`, starts ~line 28; find its end by the next `// ---` section marker), with `normalizeRoutes`→`NormalizeRoutes`, `extractMiddlewareConfig`→`ExtractMiddlewareConfig`, `package routecfg`. Example shape (first moved test):

```go
package routecfg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeRoutes_SingleRoute(t *testing.T) {
	data := map[string]any{"method": "GET", "path": "/test"}
	routes := NormalizeRoutes(data)
	require.Len(t, routes, 1)
	assert.Equal(t, "GET", routes[0]["method"])
}
```

- [ ] **Step 3: Run the new package's tests**

Run: `go test ./internal/routecfg/ -v`
Expected: PASS.

- [ ] **Step 4: Rewire internal/server and delete the originals**

Delete `normalizeRoutes` from `editor_codegen.go` and `middlewareConfigPaths`+`extractMiddlewareConfig` from `middleware.go`. Add `"github.com/chimpanze/noda/internal/routecfg"` to importers. Rewire every remaining reference:

Known call sites at plan time: `middleware.go`, `session_middleware.go`, `validate_middleware.go`, `editor_nodes.go:277`, `openapi.go:66`, `editor_codegen.go:291`, plus test files. Re-list at execution time and rewrite every hit:

```bash
files=$(grep -rln 'normalizeRoutes\|extractMiddlewareConfig' internal/server/)
perl -pi -e 's/\bnormalizeRoutes\(/routecfg.NormalizeRoutes(/g; s/\bextractMiddlewareConfig\(/routecfg.ExtractMiddlewareConfig(/g' $files
goimports -w internal/server/
```

Also delete the moved tests from `editor_helpers_test.go` and `coverage_test.go` (they now live in routecfg_test.go).

- [ ] **Step 5: Verify**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./internal/server/ ./internal/routecfg/`
Expected: clean build, silent gofmt, tests pass. `grep -rn 'func normalizeRoutes\|func extractMiddlewareConfig' internal/server/` → no output.

- [ ] **Step 6: Commit**

```bash
git add internal/routecfg internal/server
git commit -m "refactor(server): extract route/middleware config helpers into internal/routecfg

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task B2: Move the editor API to `internal/editor` and rewire cmd/noda

**Files:**
- Rename (git mv, `internal/server` → `internal/editor`): `editor.go`→`api.go`, `editor_nodes.go`→`nodes.go`, `editor_files.go`→`files.go`, `editor_schemas.go`→`schemas.go`, `editor_validation.go`→`validation.go`, `editor_codegen.go`→`codegen.go`, `editor_static.go`→`static.go`, `editor_codegen_test.go`→`codegen_test.go`, `editor_schemas_test.go`→`schemas_test.go`, `editor_validation_test.go`→`validation_test.go`, `editor_helpers_test.go`→`helpers_test.go`
- Create: `internal/editor/handlers_test.go` (the `EditorAPI` tests extracted from `internal/server/coverage_test.go`), plus a test-helper copy of `buildTestNodeRegistry`
- Modify: `internal/server/coverage_test.go` (delete the moved sections), `cmd/noda/main.go:436` and `:455`

**Interfaces:**
- Consumes: `routecfg.NormalizeRoutes` / `routecfg.ExtractMiddlewareConfig` from Task B1; `Server.App() *fiber.App` (already exists at `internal/server/server.go:177`); `rtCtx.Logger` (`*slog.Logger`) in cmd/noda.
- Produces: `editor.API` struct, `editor.NewAPI(root pathutil.Root, envFlag string, reloader *devmode.Reloader, plugins *registry.PluginRegistry, nodes *registry.NodeRegistry, services *registry.ServiceRegistry, compiler *nodaexpr.Compiler, sm *secrets.Manager) *API` (same signature as old `NewEditorAPI`), `(*API).Register(app *fiber.App)`, and `editor.RegisterUI(app *fiber.App, logger *slog.Logger)`.

- [ ] **Step 1: git mv the files**

```bash
mkdir -p internal/editor
git mv internal/server/editor.go internal/editor/api.go
git mv internal/server/editor_nodes.go internal/editor/nodes.go
git mv internal/server/editor_files.go internal/editor/files.go
git mv internal/server/editor_schemas.go internal/editor/schemas.go
git mv internal/server/editor_validation.go internal/editor/validation.go
git mv internal/server/editor_codegen.go internal/editor/codegen.go
git mv internal/server/editor_static.go internal/editor/static.go
git mv internal/server/editor_codegen_test.go internal/editor/codegen_test.go
git mv internal/server/editor_schemas_test.go internal/editor/schemas_test.go
git mv internal/server/editor_validation_test.go internal/editor/validation_test.go
git mv internal/server/editor_helpers_test.go internal/editor/helpers_test.go
```

- [ ] **Step 2: Rename package and de-stutter the API type**

```bash
perl -pi -e 's/^package server$/package editor/' internal/editor/*.go
perl -pi -e 's/\bNewEditorAPI\b/NewAPI/g; s/\bEditorAPI\b/API/g' internal/editor/*.go
```

Update the doc comment on the type in `internal/editor/api.go`:

```go
// API provides endpoints for the visual editor (dev mode only).
type API struct {
```

and on the constructor: `// NewAPI creates the editor API handler for dev mode.`

- [ ] **Step 3: Convert RegisterEditorUI to a free function**

In `internal/editor/static.go`, change the signature and body references (`s.app`→`app`, `s.logger`→`logger`); everything else in the body is unchanged, including the `trace.RegisterNoOpTraceWebSocket(app)` call at the end:

```go
// RegisterUI serves the embedded editor SPA at /editor/.
// If the binary was built without the embed_editor tag, a placeholder is shown.
// In production mode a no-op trace WebSocket is registered so the editor
// connects without errors; in dev mode the real trace endpoint is registered
// separately via trace.RegisterTraceWebSocket.
func RegisterUI(app *fiber.App, logger *slog.Logger) {
```

Add `"log/slog"` to static.go's imports.

- [ ] **Step 4: Move the EditorAPI tests out of coverage_test.go**

The sections to move from `internal/server/coverage_test.go` into a new `internal/editor/handlers_test.go` (`package editor`; locate exact boundaries by their `// ---` section markers at execution time):

- `// --- editor.go: findUpstreamNodes ---` (~line 2131, `TestFindUpstreamNodes` + siblings using `e := &EditorAPI{}`)
- `TestEditorAPI_ResolvedConfig_WithReloader` and neighbors (~lines 2472–2520)
- `// --- editor_static.go: RegisterEditorUI (no embedded FS) ---` (~line 2522)
- `// --- EditorAPI handler tests ---` (~line 2948 through the last `TestEditorAPI_*`, ~line 3490) including the `setupEditorApp` helper

Apply the same renames as Step 2 to the moved code (`NewEditorAPI`→`NewAPI`, `EditorAPI`→`API`). Tests are in-package (`package editor`), so direct field access like `editorAPI.rc = rc` keeps working.

Rewrite `TestRegisterEditorUI_NoEmbeddedFS` for the free function (it previously built a full `*Server` via `newTestServer`):

```go
func TestRegisterUI_NoEmbeddedFS(t *testing.T) {
	app := fiber.New()
	// editorfs.FS is nil in builds without the embed_editor tag,
	// so RegisterUI serves the placeholder routes.
	RegisterUI(app, slog.Default())

	req := httptest.NewRequest("GET", "/editor", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Editor not embedded")
}
```

(Keep whatever assertions the existing test makes if they differ — the rewrite only changes how the app is constructed.)

Add the test-registry helper to `handlers_test.go` — copied verbatim from `internal/server/routes_test.go:30-41` because test helpers can't be imported across packages:

```go
func buildTestNodeRegistry() *registry.NodeRegistry {
	nodeReg := registry.NewNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&control.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&transform.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&util.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&workflow.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&response.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&dbplugin.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&cacheplugin.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&event.Plugin{})
	return nodeReg
}
```

Then delete all moved sections from `internal/server/coverage_test.go` and run `goimports -w internal/editor/ internal/server/`.

- [ ] **Step 5: Rewire cmd/noda**

`cmd/noda/main.go:436` (add import `"github.com/chimpanze/noda/internal/editor"`):

```go
editorAPI := editor.NewAPI(root, envFlag, reloader, rtCtx.Plugins, rtCtx.Bootstrap.Nodes, rtCtx.Bootstrap.Services, rtCtx.Bootstrap.Compiler, rtCtx.SecretsManager)
editorAPI.Register(srv.App())
```

`cmd/noda/main.go:455`:

```go
editor.RegisterUI(srv.App(), rtCtx.Logger)
```

- [ ] **Step 6: Build and test everything**

Run: `go build ./... && go vet ./... && gofmt -l . && go test ./internal/editor/ ./internal/server/ ./internal/routecfg/ ./cmd/...`
Expected: clean build, silent gofmt, all tests pass in their new homes.

- [ ] **Step 7: Commit**

```bash
git add -A internal/editor internal/server cmd/noda
git commit -m "refactor(server): extract editor API into internal/editor

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task B3: Full verification, smoke test, PR

**Files:** none (verification only).

**Interfaces:**
- Consumes: everything from B1/B2.
- Produces: PR 2.

- [ ] **Step 1: Grep-zero and layering checks**

```bash
grep -rn 'NewEditorAPI\|RegisterEditorUI' --include='*.go' .        # expect: no output
grep -rn 'editorfs' internal/server/ | grep -v _test                # expect: no output
go list -f '{{join .Imports "\n"}}' ./internal/editor | grep chimpanze  # expect: config, devmode, expr, pathutil, registry, routecfg, secrets, trace, editorfs, pkg/api — and NOT internal/server
```

- [ ] **Step 2: Full test suite**

Run: `go test ./...`
Expected: PASS (same flake caveat: `TestWatcher_Debounce`, #347).

- [ ] **Step 3: Behavior smoke — the editor UI and API still serve**

```bash
go build -o /tmp/noda-plan ./cmd/noda
cd examples/init-example        # noda.json sets "port": 3000
/tmp/noda-plan dev &
sleep 2
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:3000/_noda/nodes   # expect 200
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:3000/editor        # expect 200
kill %1
cd ../..
```

(The dev server logs `dev server starting port=…` — if it differs from 3000, use the logged port. The point is `/_noda/nodes` and `/editor` respond exactly as before the move.)

- [ ] **Step 4: Push and open PR 2**

```bash
git push -u origin editor-api-extract
gh pr create --title "refactor(server): extract editor API into internal/editor + routecfg leaf" --body "$(cat <<'EOF'
## Summary
- Moves the seven editor_*.go files (2,400 lines) and all their tests out of internal/server into a new internal/editor package; EditorAPI -> editor.API, RegisterEditorUI method -> editor.RegisterUI free function
- New leaf package internal/routecfg for the two helpers shared by server and editor (NormalizeRoutes, ExtractMiddlewareConfig) — no lateral server<->editor dependency
- Pure refactor: identical routes (/_noda/*, /editor/*), identical handler logic, no behavior change, no CHANGELOG entry
- Design spec + plan ship on the livekit-snake-case PR

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR opens, CI green.

---

## Self-Review Notes (done at plan time)

- Spec coverage: PR 1 = Tasks A1–A3 (rename, docs, CHANGELOG/verify); PR 2 = Tasks B1–B3 (routecfg, move+rewire, verify/PR). The spec's "server gains a fiber-app accessor if needed" is moot — `Server.App()` already exists (`internal/server/server.go:177`).
- Deviation from spec, user-approved during planning: the split needs `internal/routecfg` because coupling runs BOTH ways (`editor_nodes.go` uses server's `extractMiddlewareConfig`; server's `openapi.go` uses editor's `normalizeRoutes`) — the spec had only noted `RegisterEditorUI`. `convertPath`/`extractPathParams` stay in the editor package (openapi.go has its own `fiberToOpenAPIPath`).
- The `EditorAPI` tests living inside `coverage_test.go` (~60 tests, not just the 4 dedicated `editor_*_test.go` files) were discovered at plan time; Task B2 Step 4 covers them.
- `tools/docverify/groundtruth/main_test.go` pins `lk.token` only — unaffected by the rename; the final repo-wide grep in A3 confirms.
