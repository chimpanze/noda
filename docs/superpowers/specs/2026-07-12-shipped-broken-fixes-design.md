# Shipped-Broken Fixes — Design

**Date:** 2026-07-12
**Issues:** #302 (saas-backend upload route), #308 (expression cookbook infix operators), #309 (livekit applyGrants silent drop), #292 (participant_update cleanups)
**Branch:** `feat/shipped-broken-fixes`

## Problem

Four small defects, all found during the homebase cycles, all "shipped and
wrong today":

- **#302:** `examples/saas-backend/routes/upload-attachment.json` declares
  `"files": ["file"]` but its `trigger.input` has no `"file"` key. The runtime
  (`internal/server/trigger.go getFileFields`) only injects a multipart stream
  for keys present in `input` — `files` marks which input keys are file
  fields, it does not create them. Every real upload on that route 4xxes with
  `field "file" not found in input`. Same bug class as the Critical caught on
  homebase PR #299.
- **#308:** `docs/01-getting-started/expression-cookbook.md` documents
  `startsWith(s, prefix)`, `endsWith(s, suffix)`, `matches(s, regex)`,
  `contains(haystack, needle)` with function-call signatures. In expr-lang
  v1.17.8 these are INFIX OPERATORS only — the function-call form fails to
  compile. This has derailed implementation plans twice (homebase rooms T2/T3).
- **#309:** `plugins/livekit/helpers.go applyGrants` maps `canPublishSources`
  strings through `lkproto.TrackSource_value` and silently skips non-matches.
  Enum names are uppercase (`CAMERA`, `MICROPHONE`, `SCREEN_SHARE`,
  `SCREEN_SHARE_AUDIO`), so a natural config like `["screen_share"]` produces
  `SetCanPublishSources([])` — total publish lockout for the token holder,
  with no error and no log.
- **#292:** three Minors in `plugins/livekit/participant_update.go` from the
  tranche-G final review: dual enumeration of the five permission keys
  (validation switch + overlay both list them), empty `permissions: {}` still
  performs GetParticipant + UpdateParticipant round-trip, and a test-fidelity
  gap in `TestParticipantUpdateNode_NonBoolPermissionValueErrors`.

## Decisions (user-approved 2026-07-12)

1. **#302 gets the route fix AND a load-time class-killer:** a crossrefs
   validation that every route-trigger `files` entry has a matching
   `trigger.input` key. The class has shipped twice; `noda validate` should
   catch the third.
2. All four issues in one tranche/PR (approved in the tranche plan).

## Design

### 1. Upload input-mapping fix + validation (#302)

**Route fix:** add `"file": "file"` to the `trigger.input` map of
`examples/saas-backend/routes/upload-attachment.json` (mirrors
`projects/homebase/routes/drops.upload.json:11`).

**Class-killer:** in `internal/config/crossrefs.go`, alongside the existing
per-route validations: for each route with a `trigger.files` array, every
string entry must exist as a key in `trigger.input` (a map). Violation →
`ValidationError{FilePath: <route file>, JSONPath: "/trigger/files",
Message: 'files entry "<name>" has no matching trigger.input key — add
"<name>": "<name>" to trigger.input or the multipart stream never reaches
the workflow'}`. Routes without `files` are untouched. No runtime behavior
change — validation-only.

All existing example/testdata/project routes must pass the new check (the
saas-backend fix makes the tree clean; if any other config trips it, that is
a real latent bug — fix it the same way and note it).

### 2. Expression cookbook corrections (#308)

In `docs/01-getting-started/expression-cookbook.md`:

- The rows for `contains`, `startsWith`, `endsWith` (lines ~34-36) and
  `matches` (line ~43) change their signature column from function-call form
  to infix form: `haystack contains needle`, `s startsWith prefix`,
  `s endsWith suffix`, `s matches regex`.
- The `startsWith` and `endsWith` examples change from
  `{{ startsWith(input.path, '/api') }}` / `{{ endsWith(input.email, ...) }}`
  to infix (`{{ input.path startsWith '/api' }}`, etc.); the `matches`
  example likewise. The `contains` example is already infix — keep it.
- Add one short note near those rows: these are binary operators in
  expr-lang, not callable functions — `startsWith(x, 'y')` does not compile.

Old plan documents under `docs/superpowers/` retain their historical
(incorrect) text — they are point-in-time records, not user docs.

**Regression guard:** a Go test (location per plan; `internal/expr` package
test or a doc-focused test file) compiles each of the four corrected
expression forms with the project's expression compiler and asserts the
function-call forms FAIL to compile — pinning both the docs' claim and the
expr-lang behavior it rests on.

### 3. applyGrants strictness (#309)

`plugins/livekit/helpers.go`:

- `applyGrants(grants map[string]any, vg *auth.VideoGrant)` changes signature
  to return `error`.
- `canPublishSources` handling: each entry must be a string (non-string →
  error naming the index and type); normalize with `strings.ToUpper` before
  the `lkproto.TrackSource_value` lookup (so `"screen_share"`,
  `"camera"` work); a name still unknown after normalization → error listing
  the valid values. No silent skips remain.
- Sole caller `plugins/livekit/token.go:82` propagates the error (node
  returns it as a normal node error).

Behavior change: configs with misspelled/unknown source names now fail the
token node loudly instead of minting a locked-out token. Lowercase names that
previously caused lockout now work. CHANGELOG entries under Fixed (plus a
Changed note for the new error).

If livekit node docs (`docs/03-nodes/lk.*.md` or equivalent) enumerate
`canPublishSources` values, state the accepted names and the
case-insensitivity there.

### 4. participant_update cleanups (#292)

`plugins/livekit/participant_update.go mergedPermissions`:

- **(a) Single source of truth:** replace the validation `switch` + five
  overlay `if` blocks with one table,
  `var permissionSetters = map[string]func(*lkproto.ParticipantPermission, bool)`,
  covering `canPublish`, `canSubscribe`, `canPublishData`, `hidden`,
  `recorder` (keep the existing `//nolint:staticcheck` on the recorder
  setter). Validation iterates `perms` and errors on keys absent from the
  table or non-bool values; the overlay applies via the same table. Error
  messages keep their current text (existing tests assert on them).
- **(b) Empty-map skip:** in the caller, when the resolved `permissions` map
  is non-nil but empty, skip `mergedPermissions` entirely (no GetParticipant,
  no Permission field on the update request) — `permissions: {}` becomes a
  no-op on permissions rather than a two-RPC full-replace of unchanged values.
- **(c) Test fidelity:** in
  `TestParticipantUpdateNode_NonBoolPermissionValueErrors`, add a comment
  noting that in production, string values resolve as expressions (so
  `"true"` would arrive as boolean `true`), and add a resolved-value-typed
  case (e.g. `"canPublish": 42`) proving the guard rejects genuinely non-bool
  resolved values.

### Testing

- **Crossrefs (#302):** table test — files-with-mapping passes; files entry
  without input key errors with the expected JSONPath/message; route without
  `files` untouched; `files` present with `input` absent entirely errors.
- **Cookbook guard (#308):** compile-based test as described above.
- **applyGrants (#309):** lowercase names normalize and produce the expected
  TrackSource values; unknown name errors; non-string entry errors; existing
  uppercase configs unchanged. Token-node test asserting the error surfaces.
- **participant_update (#292):** empty-map case asserts NO RoomClient calls
  (mock records calls); each of the five keys still overlays correctly
  through the table; unknown-key and non-bool errors keep their messages.
- Existing livekit tests must pass unchanged except where the issue text
  explicitly changes behavior.

## Execution shape

Standing conventions: worktree `.worktrees/shipped-broken-fixes`, branch
`feat/shipped-broken-fixes` off main; spec + plan `git add -f`'d onto the
branch; subagent-driven per task (4 implementation tasks + final
verification/review); whole-branch review before PR. PR closes #302, #308,
#309, #292.

Branch is cut from main before PR #311 (server-edge-correctness) merges.
Overlap check done: #311 touches `crossrefs.go` only in the server-scalar
section; this tranche adds a separate route-trigger block — textual conflict
possible but semantically independent; whichever merges second rebases
trivially.

## Out of scope

- Runtime changes to multipart handling (`internal/server/trigger.go`) — the
  validation is load-time only.
- Rewriting historical plan/spec docs under `docs/superpowers/`.
- Other livekit nodes beyond `token` (applyGrants' only consumer) and
  `participantUpdate`.
