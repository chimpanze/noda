# Design: Editor API Extraction + Livekit Node Naming

**Date:** 2026-07-19
**Status:** Approved
**Origin:** Clean-code review of the codebase (structure/consistency pass). The review
found two actionable outliers: the editor API living as a 2,400-line file cluster
inside `internal/server`, and the livekit plugin being the only plugin with
camelCase node names. Everything else was assessed as clean and consistent.

Two independent changes, one plan, **two PRs** (they share zero files):

1. **PR 1 — livekit snake_case rename.** Breaking config-namespace change; gets
   its own CHANGELOG entry and revert point.
2. **PR 2 — editor API extraction to `internal/editor`.** Pure mechanical
   refactor, no behavior change.

Explicitly rejected: promoting `internal/plugin` helpers to `pkg/` — Noda will
only ever ship in-repo plugins, so the helper layer stays internal.

---

## PR 1: Livekit node names → snake_case (breaking)

Every plugin except livekit uses snake_case node names (`db.find_one`,
`auth.create_user`, `util.jwt_sign`). Livekit's 17 camelCase names are renamed;
`lk.token` is already conformant. The `lk` prefix stays: name≠prefix is an
established pattern (`postgres`/`db`) — only the casing is the outlier.

**Migration strategy: clean break.** No aliases, no deprecation shim. Old names
fail validation as unknown node types. This matches the 0.0.x precedent (sync
envelope v2-only, inline-connections rejection).

### Name mapping

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
| `lk.token` | `lk.token` (unchanged) |

### Changes

- **`plugins/livekit/*.go`** — descriptor `Name()` return values and the
  `fmt.Errorf("lk.roomCreate: …")` error prefixes (convention:
  `<prefix>.<node>: %w`). Go filenames are already snake_case; internal Go
  symbols (`roomCreateDescriptor` etc.) stay camelCase per Go convention.
- **`docs/03-nodes/`** — 18 per-node doc files renamed
  (`lk.roomCreate.md → lk.room_create.md`) plus all cross-links to them.
- **All referencing config/doc files** — 63 files across `docs/`, `examples/`
  (including the `examples/node-cookbook/livekit/` project), and `testdata/`.
- **CHANGELOG** — `[Unreleased]` gets a **Breaking** entry containing the full
  mapping table above. That table doubles as the migration recipe for Homebase
  (separate repo, pinned to a tagged image — nothing breaks until its next
  noda upgrade; out of scope for this PR).

### What follows automatically

The cookbook coverage gate (`TestCookbookCoverage`) and the editor node palette
read node names from the registry, so they track the rename with no code change.
The livekit cookbook project runs against the real LiveKit service in CI, which
proves the rename end-to-end.

### Verification

- `go build ./... && go vet ./...`, full test suite.
- Livekit cookbook CI job green.
- Grep-zero for all 17 old names across the repo (code, docs, examples,
  testdata).

---

## PR 2: Editor API extraction to `internal/editor` (mechanical)

`internal/server` is the largest internal package (~5.4k lines); 2,400 of those
lines are the seven `editor_*.go` files. The editor API is already self-contained:
`EditorAPI` is constructed only from `cmd/noda/main.go` and never touches the
`Server` struct. The only genuine coupling is `RegisterEditorUI` (SPA serving),
a `Server` method that uses just `s.app` and `s.logger`.

**Location: `internal/editor`** — a top-level sibling of `server`/`devmode`,
not a subpackage of `server`, because the code has zero dependency on `Server`
and nesting would imply coupling that doesn't exist. The Go import path does not
clash with the `editor/` frontend directory or `editorfs/`.

### Changes

- **Move** the seven `editor_*.go` files and five `editor_*_test.go` files from
  `internal/server` to `internal/editor`, dropping the now-redundant filename
  prefix: `editor.go → api.go`, `editor_nodes.go → nodes.go`,
  `editor_files.go → files.go`, `editor_schemas.go → schemas.go`,
  `editor_validation.go → validation.go`, `editor_codegen.go → codegen.go`,
  `editor_static.go → static.go` (tests follow the same pattern).
- **Rename for stutter:** `server.EditorAPI` → `editor.API`,
  `server.NewEditorAPI` → `editor.NewAPI`. Constructor signature unchanged.
  Single call site: `cmd/noda/main.go:436`.
- **`RegisterEditorUI`** becomes a free function
  `editor.RegisterUI(app *fiber.App, logger *slog.Logger)`; the
  `trace.RegisterNoOpTraceWebSocket` call inside moves with it. `Server` gains a
  fiber-app accessor if one doesn't already exist, and `cmd/noda` calls
  `editor.RegisterUI(srv.App(), …)` where it previously called
  `srv.RegisterEditorUI()`.

### Resulting structure

`internal/server` drops ~2,400 lines and its `editorfs` import. The new
package's imports — `config`, `devmode`, `expr`, `pathutil`, `registry`,
`secrets`, `trace`, `editorfs` — keep the import graph acyclic; `internal/editor`
sits at the same layer as `server` (consumed only by `cmd/noda`).

### Verification

- `go build ./... && go vet ./...`, full test suite (moved tests included).
- Grep-zero for `NewEditorAPI` / `RegisterEditorUI` outside `internal/editor`.
- No behavior change expected: identical routes (`/_noda/*`, `/editor/*`),
  identical handler logic. Manual smoke: `noda dev` serves the editor UI and
  `/_noda/nodes` responds.
