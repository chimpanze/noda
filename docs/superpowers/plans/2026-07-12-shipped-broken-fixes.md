# Shipped-Broken Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix four shipped defects — the saas-backend upload route that can never receive a file (#302, plus a load-time validation killing the class), the expression cookbook documenting infix-only operators as functions (#308), livekit `applyGrants` silently dropping unknown `canPublishSources` values (#309), and the `participant_update` cleanups (#292).

**Architecture:** Four independent fixes, one task each. The #302 class-killer extends the existing crossrefs route-validation pass (`internal/config/crossrefs.go`); #309 changes `applyGrants` to return an error (sole caller: `token.go`); #292 collapses the dual permission-key enumeration into a single setter table; #308 is a docs fix pinned by a compile-based regression test in `internal/expr`.

**Tech Stack:** Go, expr-lang v1.17.8 (via `internal/expr`), livekit/protocol (auth.VideoGrant, lkproto), testify.

**Spec:** `docs/superpowers/specs/2026-07-12-shipped-broken-fixes-design.md`

## Global Constraints

- Repo-wide sweep already done (2026-07-12): `examples/saas-backend/routes/upload-attachment.json` is the ONLY config violating the files/input rule; `projects/homebase/routes/drops.upload.json` is clean. After Task 1's route fix the tree must pass the new validation with zero errors.
- Existing error-message texts asserted by tests must not change: `unknown permission key %q` and `permission key %q must be a boolean, got %T` (asserted in `plugins/livekit/nodes_test.go` ~:585-605).
- The `//nolint:staticcheck` comment on the `Recorder` assignment must be preserved (no replacement API exists).
- Behavior changes (CHANGELOG, Task 5): lk.token now errors on unknown/non-string `canPublishSources` entries instead of silently minting a locked-out token; lowercase source names now work; `lk.participantUpdate` with empty `permissions: {}` no longer calls GetParticipant nor sends a Permission replace.
- expr-lang facts (issue #308 / homebase rooms T2/T3, verified in practice): `contains`/`startsWith`/`endsWith`/`matches` are binary infix operators; the function-call form fails to compile.
- Local gate before every commit: `gofmt -l .` prints nothing new (`examples/wasm-helpers/wasm/helpers/main.go` is a known pre-existing hit on main — ignore it), `go vet ./...`, plus the task's tests. Conventional commits with trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- PR #311 (server-edge-correctness) may merge to main while this branch is in flight and touches a different region of `crossrefs.go` — Task 5 rebases onto latest main before the PR and resolves any adjacency conflict mechanically.

## Worktree setup (before Task 1)

```bash
git -C /Users/marten/GolandProjects/noda worktree add .worktrees/shipped-broken-fixes -b feat/shipped-broken-fixes main
cd /Users/marten/GolandProjects/noda/.worktrees/shipped-broken-fixes
mkdir -p docs/superpowers/specs docs/superpowers/plans
cp ../../docs/superpowers/specs/2026-07-12-shipped-broken-fixes-design.md docs/superpowers/specs/
cp ../../docs/superpowers/plans/2026-07-12-shipped-broken-fixes.md docs/superpowers/plans/
git add -f docs/superpowers/specs/2026-07-12-shipped-broken-fixes-design.md docs/superpowers/plans/2026-07-12-shipped-broken-fixes.md
git commit -m "docs: spec + plan for shipped-broken fixes tranche (#302, #308, #309, #292)"
```

---

### Task 1: Upload input-mapping fix + crossrefs class-killer (#302)

**Files:**
- Modify: `examples/saas-backend/routes/upload-attachment.json` (trigger.input)
- Modify: `internal/config/crossrefs.go` (new block after the "Validate duration fields in routes" loop, ~line 208-218)
- Test: `internal/config/crossrefs_test.go` (append)

**Interfaces:**
- Consumes: `ValidateCrossRefs(rc *RawConfig) []ValidationError` (existing entry point; `rc.Routes` is `map[string]map[string]any` keyed by file path); `ValidationError{FilePath, JSONPath, Message string}`.
- Produces: no new API — a new validation rule: every string entry in a route's `trigger.files` array must exist as a key in `trigger.input`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/crossrefs_test.go`. Note: `ValidateCrossRefs` on a minimal RawConfig will also emit unrelated errors (e.g. the trigger's workflow reference not existing), so assertions filter on `JSONPath == "/trigger/files"` rather than requiring an empty error list — this mirrors how `TestCrossrefs_ServerScalarValidation` in the same file filters on `/server/` paths (if that test isn't present on this branch's base, follow the construction style of the file's other route tests).

```go
func TestCrossrefs_RouteTriggerFilesInputMapping(t *testing.T) {
	mkrc := func(trigger map[string]any) *RawConfig {
		return &RawConfig{Routes: map[string]map[string]any{
			"routes/upload.json": {"id": "upload", "trigger": trigger},
		}}
	}
	filesErrs := func(errs []ValidationError) []ValidationError {
		var out []ValidationError
		for _, e := range errs {
			if e.JSONPath == "/trigger/files" {
				out = append(out, e)
			}
		}
		return out
	}

	t.Run("files entry with mapping passes", func(t *testing.T) {
		errs := ValidateCrossRefs(mkrc(map[string]any{
			"workflow": "w",
			"files":    []any{"file"},
			"input":    map[string]any{"file": "file"},
		}))
		assert.Empty(t, filesErrs(errs))
	})
	t.Run("files entry without input key errors", func(t *testing.T) {
		errs := filesErrs(ValidateCrossRefs(mkrc(map[string]any{
			"workflow": "w",
			"files":    []any{"file"},
			"input":    map[string]any{"user": "{{ auth.sub }}"},
		})))
		require.Len(t, errs, 1)
		assert.Equal(t, "routes/upload.json", errs[0].FilePath)
		assert.Contains(t, errs[0].Message, `"file"`)
		assert.Contains(t, errs[0].Message, "trigger.input")
	})
	t.Run("files present but input absent entirely errors", func(t *testing.T) {
		errs := filesErrs(ValidateCrossRefs(mkrc(map[string]any{
			"workflow": "w",
			"files":    []any{"file"},
		})))
		require.Len(t, errs, 1)
	})
	t.Run("two files entries one missing errors once", func(t *testing.T) {
		errs := filesErrs(ValidateCrossRefs(mkrc(map[string]any{
			"workflow": "w",
			"files":    []any{"avatar", "doc"},
			"input":    map[string]any{"avatar": "avatar"},
		})))
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Message, `"doc"`)
	})
	t.Run("route without files untouched", func(t *testing.T) {
		errs := ValidateCrossRefs(mkrc(map[string]any{
			"workflow": "w",
			"input":    map[string]any{"x": "y"},
		}))
		assert.Empty(t, filesErrs(errs))
	})
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/config/ -run TestCrossrefs_RouteTriggerFilesInputMapping -v`
Expected: FAIL — the error-expecting subtests find no `/trigger/files` errors.

- [ ] **Step 3: Add the crossrefs block**

In `internal/config/crossrefs.go`, directly after the "Validate duration fields in routes" loop (the `for filePath, route := range rc.Routes` block around lines 208-218), insert:

```go
	// Validate route trigger file mappings. A "files" entry marks which
	// trigger.input keys carry multipart file streams — it does not create
	// them (internal/server/trigger.go getFileFields). A files entry with no
	// matching input key means the stream never reaches the workflow and
	// every real upload fails.
	for filePath, route := range rc.Routes {
		trigger, ok := route["trigger"].(map[string]any)
		if !ok {
			continue
		}
		files, ok := trigger["files"].([]any)
		if !ok {
			continue
		}
		input, _ := trigger["input"].(map[string]any)
		for _, f := range files {
			name, ok := f.(string)
			if !ok {
				continue
			}
			if _, present := input[name]; !present {
				errs = append(errs, ValidationError{
					FilePath: filePath,
					JSONPath: "/trigger/files",
					Message: fmt.Sprintf(
						`files entry %q has no matching trigger.input key — add "%s": "%s" to trigger.input, otherwise the multipart stream never reaches the workflow`,
						name, name, name),
				})
			}
		}
	}
```

(Non-string `files` entries are skipped here — the route JSON schema already types them as strings.)

- [ ] **Step 4: Fix the route**

In `examples/saas-backend/routes/upload-attachment.json`, add the mapping as the first input key:

```json
    "input": {
      "file": "file",
      "workspace_id": "{{ params.workspace_id }}",
      "project_id": "{{ params.project_id }}",
      "uploaded_by": "{{ auth.sub }}"
    }
```

- [ ] **Step 5: Run tests + whole-tree validation sweep**

Run: `go test ./internal/config/ -run TestCrossrefs_RouteTriggerFilesInputMapping -v` → PASS.
Run: `go test ./internal/config/` → PASS (no existing test regressions).
Sweep the repo's real configs through the new rule — the example projects have validation coverage via existing tests/CI, but do a direct check:

```bash
go run ./cmd/noda validate --config examples/saas-backend 2>&1 | tail -5
```

Expected: validation passes (set any `$env` vars the example needs if it complains about missing env — check `grep -rn '\$env' examples/saas-backend/noda.json` and export dummies). If any OTHER config in `examples/`/`testdata/` now fails with a `/trigger/files` error, that's a real latent #302-class bug: fix it the same way (`"<name>": "<name>"`) and record it in your report.

- [ ] **Step 6: Gate and commit**

```bash
gofmt -l . && go vet ./internal/config/
git add internal/config/crossrefs.go internal/config/crossrefs_test.go examples/saas-backend/routes/upload-attachment.json
git commit -m "fix(examples): saas-backend upload route file mapping + validate files/input match (#302)"
```

---

### Task 2: Expression cookbook infix corrections + compile guard (#308)

**Files:**
- Modify: `docs/01-getting-started/expression-cookbook.md` (rows at ~lines 34-36 and 43; note after the table)
- Create: `internal/expr/cookbook_operators_test.go`

**Interfaces:**
- Consumes: `expr.NewCompilerWithFunctions()` (`internal/expr/functions.go:284`) and `(*Compiler).Compile(input string) (*CompiledExpression, error)` (`internal/expr/compiler.go:96`). Check how existing tests in `internal/expr` call `Compile` (bare expression, no `{{ }}` wrapper) and mirror that.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the compile-guard test**

Create `internal/expr/cookbook_operators_test.go`. Literal-only expressions avoid any environment/identifier concerns:

```go
package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Pins the expr-lang behavior the expression cookbook documents (#308):
// contains, startsWith, endsWith, matches are binary infix operators, not
// callable functions. If a case here starts compiling as a function call
// (e.g. after an expr-lang upgrade or a custom function registration), the
// cookbook needs re-checking.
func TestCookbookOperators_InfixCompiles_FunctionFormDoesNot(t *testing.T) {
	c := NewCompilerWithFunctions()

	infix := []string{
		`'abc' contains 'b'`,
		`'/api/x' startsWith '/api'`,
		`'a@company.com' endsWith '@company.com'`,
		`'a@b.c' matches '^[^@]+@[^@]+$'`,
	}
	for _, src := range infix {
		_, err := c.Compile(src)
		require.NoError(t, err, "infix form must compile: %s", src)
	}

	fnForm := []string{
		`contains('abc', 'b')`,
		`startsWith('/api/x', '/api')`,
		`endsWith('a@company.com', '@company.com')`,
		`matches('a@b.c', '^[^@]+@[^@]+$')`,
	}
	for _, src := range fnForm {
		_, err := c.Compile(src)
		assert.Error(t, err, "function-call form must NOT compile (docs say infix-only): %s", src)
	}
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/expr/ -run TestCookbookOperators -v`
Expected: PASS immediately (this pins current behavior; it is a regression guard, not TDD red — the "failing" state it guards against is a future expr-lang/function-registry change). If any function-form case unexpectedly COMPILES, stop: the cookbook may be right for that entry after all — report it instead of adjusting the docs.

- [ ] **Step 3: Fix the cookbook rows**

In `docs/01-getting-started/expression-cookbook.md`, replace the four rows:

```markdown
| `haystack contains needle` | True if string/array contains value | `{{ input.roles contains 'admin' }}` |
| `s startsWith prefix` | True if string starts with prefix | `{{ input.path startsWith '/api' }}` |
| `s endsWith suffix` | True if string ends with suffix | `{{ input.email endsWith '@company.com' }}` |
```

and (line ~43):

```markdown
| `s matches regex` | True if string matches regex | `{{ input.email matches '^[^@]+@[^@]+$' }}` |
```

Immediately after that table, add:

```markdown
> **Note:** `contains`, `startsWith`, `endsWith`, and `matches` are binary **operators**, not callable functions — `startsWith(input.path, '/api')` fails to compile; write `input.path startsWith '/api'`.
```

Do NOT touch historical plan/spec documents under `docs/superpowers/` (they are point-in-time records and some contain the old, wrong form).

- [ ] **Step 4: Verify no other user-facing doc carries the wrong form**

```bash
grep -rn "startsWith(\|endsWith(\|matches(\|contains(" docs/ --include="*.md" | grep -v "docs/superpowers/"
```

Expected: no hits outside `docs/superpowers/` (pre-verified 2026-07-12; if a new hit appears, fix it the same way and note it).

- [ ] **Step 5: Gate and commit**

```bash
gofmt -l . && go vet ./internal/expr/ && go test ./internal/expr/ -run TestCookbookOperators
git add docs/01-getting-started/expression-cookbook.md internal/expr/cookbook_operators_test.go
git commit -m "docs(expressions): contains/startsWith/endsWith/matches are infix operators, with compile guard (#308)"
```

---

### Task 3: applyGrants strictness for canPublishSources (#309)

**Files:**
- Modify: `plugins/livekit/helpers.go` (applyGrants: signature + canPublishSources block; add `fmt`, `strings` imports)
- Modify: `plugins/livekit/token.go:80-84` (propagate error)
- Modify: `docs/03-nodes/lk.token.md:27` (document accepted values + case-insensitivity)
- Test: `plugins/livekit/helpers_test.go` (append; mirror the file's existing applyGrants test style)

**Interfaces:**
- Consumes: `lkproto.TrackSource_value` (map[string]int32; names `UNKNOWN`, `CAMERA`, `MICROPHONE`, `SCREEN_SHARE`, `SCREEN_SHARE_AUDIO`), `auth.VideoGrant.SetCanPublishSources([]lkproto.TrackSource)`. Verify the getter for assertions against the vendored livekit protocol package (`~/go/pkg/mod/github.com/livekit/protocol@*/auth/grants.go`) — use whatever accessor exists (e.g. `vg.GetCanPublishSources()`) rather than guessing a field.
- Produces: `func applyGrants(grants map[string]any, vg *auth.VideoGrant) error` (signature change; sole caller is token.go).

- [ ] **Step 1: Write the failing tests**

Append to `plugins/livekit/helpers_test.go` (adapt the VideoGrant assertion to the accessor found in Step 0 above; if existing tests already assert canPublishSources, reuse their pattern):

```go
func TestApplyGrants_CanPublishSourcesCaseInsensitive(t *testing.T) {
	vg := &auth.VideoGrant{}
	err := applyGrants(map[string]any{
		"canPublishSources": []any{"screen_share", "CAMERA", "Microphone"},
	}, vg)
	require.NoError(t, err)
	// assert the three sources landed (SCREEN_SHARE, CAMERA, MICROPHONE),
	// using the VideoGrant accessor verified against the vendored package
}

func TestApplyGrants_UnknownSourceErrors(t *testing.T) {
	vg := &auth.VideoGrant{}
	err := applyGrants(map[string]any{
		"canPublishSources": []any{"screenshare"}, // no underscore — not an enum name
	}, vg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"screenshare"`)
	assert.Contains(t, err.Error(), "SCREEN_SHARE") // error must list valid values
}

func TestApplyGrants_NonStringSourceErrors(t *testing.T) {
	vg := &auth.VideoGrant{}
	err := applyGrants(map[string]any{
		"canPublishSources": []any{float64(3)},
	}, vg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "canPublishSources[0]")
}

func TestApplyGrants_ValidGrantsStillReturnNil(t *testing.T) {
	vg := &auth.VideoGrant{}
	err := applyGrants(map[string]any{"roomJoin": true, "canPublish": false}, vg)
	require.NoError(t, err)
	assert.True(t, vg.RoomJoin)
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./plugins/livekit/ -run TestApplyGrants -v`
Expected: FAIL to compile (`applyGrants(...)` used as value / too many return values) — the signature change is part of the fix.

- [ ] **Step 3: Implement**

In `plugins/livekit/helpers.go`: change the signature to `func applyGrants(grants map[string]any, vg *auth.VideoGrant) error`, add `return nil` at the end, leave every existing bool block untouched, and replace the `canPublishSources` block with:

```go
	if v, ok := grants["canPublishSources"].([]any); ok {
		sources := make([]lkproto.TrackSource, 0, len(v))
		for i, src := range v {
			s, ok := src.(string)
			if !ok {
				return fmt.Errorf("canPublishSources[%d]: expected string, got %T", i, src)
			}
			val, exists := lkproto.TrackSource_value[strings.ToUpper(s)]
			if !exists {
				return fmt.Errorf("canPublishSources[%d]: unknown track source %q (valid, case-insensitive: CAMERA, MICROPHONE, SCREEN_SHARE, SCREEN_SHARE_AUDIO)", i, s)
			}
			sources = append(sources, lkproto.TrackSource(val))
		}
		vg.SetCanPublishSources(sources)
	}
	return nil
```

Add `"fmt"` and `"strings"` to the imports.

In `plugins/livekit/token.go` (the `grants` block around line 80):

```go
	if raw, ok := config["grants"]; ok {
		if grantsMap, ok := raw.(map[string]any); ok {
			if err := applyGrants(grantsMap, vg); err != nil {
				return "", nil, fmt.Errorf("lk.token: %w", err)
			}
		}
	}
```

(`fmt` is already imported in token.go.)

In `docs/03-nodes/lk.token.md` line ~27, extend the `canPublishSources` row description: `Allowed track source types: CAMERA, MICROPHONE, SCREEN_SHARE, SCREEN_SHARE_AUDIO (case-insensitive). Unknown values are an error.`

- [ ] **Step 4: Run the package**

Run: `go test ./plugins/livekit/`
Expected: PASS — new tests plus all existing token/grant tests (if an existing test passed a lowercase or unknown source relying on silent-skip, it now fails: fix the TEST's fixture only if it used a genuinely invalid name; report if the assertion itself depended on silent-skip semantics).

- [ ] **Step 5: Gate and commit**

```bash
gofmt -l . && go vet ./plugins/livekit/
git add plugins/livekit/helpers.go plugins/livekit/helpers_test.go plugins/livekit/token.go docs/03-nodes/lk.token.md
git commit -m "fix(livekit): canPublishSources case-insensitive + error on unknown values (#309)"
```

---

### Task 4: participant_update cleanups (#292)

**Files:**
- Modify: `plugins/livekit/participant_update.go` (permissions branch ~line 76-84; `mergedPermissions` ~line 100-137)
- Test: `plugins/livekit/nodes_test.go` (extend `TestParticipantUpdateNode_NonBoolPermissionValueErrors` ~line 591; add empty-permissions test near the other participant-update tests)

**Interfaces:**
- Consumes: existing test infra in `nodes_test.go`: `testService()`, `testServices(svc)`, `mockExecCtx{resolveFunc: identityResolve}`, `mockRoomClient{getParticipantFn, updateParticipantFn}`.
- Produces: no API change. `mergedPermissions` keeps its signature; error messages keep their exact current text (`unknown permission key %q`, `permission key %q must be a boolean, got %T`).

- [ ] **Step 1: Write the failing test (empty-map skip)**

Add to `plugins/livekit/nodes_test.go` near the other participant-update tests:

```go
func TestParticipantUpdateNode_EmptyPermissionsSkipsPermissionMerge(t *testing.T) {
	svc := testService()
	getCalled := false
	var gotReq *lkproto.UpdateParticipantRequest
	svc.Room = &mockRoomClient{
		getParticipantFn: func(_ context.Context, _ *lkproto.RoomParticipantIdentity) (*lkproto.ParticipantInfo, error) {
			getCalled = true
			return &lkproto.ParticipantInfo{}, nil
		},
		updateParticipantFn: func(_ context.Context, req *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error) {
			gotReq = req
			return &lkproto.ParticipantInfo{Identity: "u"}, nil
		},
	}
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":        "r",
			"identity":    "u",
			"permissions": map[string]any{},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.False(t, getCalled, "empty permissions must not trigger the GetParticipant merge read")
	require.NotNil(t, gotReq)
	assert.Nil(t, gotReq.Permission, "empty permissions must not send a Permission full-replace")
}
```

(If the existing participant-update tests construct the executor/context differently — e.g. a shared helper — mirror them exactly.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./plugins/livekit/ -run TestParticipantUpdateNode_EmptyPermissions -v`
Expected: FAIL — `getCalled` is true and `gotReq.Permission` non-nil today (empty map still merges).

- [ ] **Step 3: Implement**

In `plugins/livekit/participant_update.go`:

(a) Change the permissions branch condition from `} else if perms != nil {` to `} else if len(perms) > 0 {` and note why:

```go
	if perms, err := plugin.ResolveOptionalMap(nCtx, config, "permissions"); err != nil {
		return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
	} else if len(perms) > 0 {
		// empty {} would otherwise cost a GetParticipant + full-replace
		// Permission send of unchanged values
		perm, err := mergedPermissions(ctx, svc, room, identity, perms)
		if err != nil {
			return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
		}
		req.Permission = perm
	}
```

(b) Replace the validation `switch` and the five overlay `if` blocks in `mergedPermissions` with a single table (package-level, above `mergedPermissions`):

```go
// permissionSetters is the single source of truth for the boolean permission
// keys lk.participantUpdate accepts — validation and overlay both iterate it,
// so a new key cannot be added to one and forgotten in the other.
var permissionSetters = map[string]func(*lkproto.ParticipantPermission, bool){
	"canPublish":     func(p *lkproto.ParticipantPermission, v bool) { p.CanPublish = v },
	"canSubscribe":   func(p *lkproto.ParticipantPermission, v bool) { p.CanSubscribe = v },
	"canPublishData": func(p *lkproto.ParticipantPermission, v bool) { p.CanPublishData = v },
	"hidden":         func(p *lkproto.ParticipantPermission, v bool) { p.Hidden = v },
	"recorder": func(p *lkproto.ParticipantPermission, v bool) {
		p.Recorder = v //nolint:staticcheck // no replacement available in ParticipantPermission; ParticipantInfo.kind is not settable here
	},
}
```

and in `mergedPermissions`, keeping the doc comment and the GetParticipant/Clone section as-is:

```go
	for key, val := range perms {
		if _, known := permissionSetters[key]; !known {
			return nil, fmt.Errorf("unknown permission key %q", key)
		}
		if _, ok := val.(bool); !ok {
			return nil, fmt.Errorf("permission key %q must be a boolean, got %T", key, val)
		}
	}
	// ... existing GetParticipant + proto.Clone block unchanged ...
	for key, val := range perms {
		permissionSetters[key](perm, val.(bool))
	}
	return perm, nil
```

(c) In `TestParticipantUpdateNode_NonBoolPermissionValueErrors`, add the fidelity comment and a second, realistic case:

```go
	// Note: in production, string map values resolve as expressions before
	// reaching the node, so "true" would arrive as boolean true and be
	// accepted. identityResolve passes the raw string through, standing in
	// for any non-bool resolved value; the numeric case below is a value
	// type that genuinely survives resolution.
	_, _, err = exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":        "r",
			"identity":    "u",
			"permissions": map[string]any{"canPublish": 42},
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `permission key "canPublish" must be a boolean`)
```

(Reuse the existing test's `svc`/`exec`/`nCtx`; rename `err` handling as needed to avoid shadowing.)

- [ ] **Step 4: Run the package**

Run: `go test ./plugins/livekit/`
Expected: PASS — new/extended tests plus every existing participant-update test (the unknown-key and non-bool error messages are unchanged; the five-key overlay behavior is pinned by existing tests).

- [ ] **Step 5: Gate and commit**

```bash
gofmt -l . && go vet ./plugins/livekit/
git add plugins/livekit/participant_update.go plugins/livekit/nodes_test.go
git commit -m "refactor(livekit): single-source permission table + empty-permissions skip in participantUpdate (#292)"
```

---

### Task 5: CHANGELOG, rebase, whole-branch verification, review, PR

**Files:**
- Modify: `CHANGELOG.md` ([Unreleased])

- [ ] **Step 1: CHANGELOG**

Fold into existing `[Unreleased]` subsections (match entry style):

- **Fixed:** `examples/saas-backend` upload-attachment route never delivered the multipart file (missing `"file"` input mapping) (#302).
- **Fixed:** `lk.token` `canPublishSources` values are now case-insensitive; unknown values error instead of silently minting a token that cannot publish (#309).
- **Added:** config validation rejects route triggers whose `files` entries have no matching `trigger.input` key — the silent-upload-failure class behind #302.
- **Changed:** `lk.participantUpdate` with empty `permissions: {}` no longer performs a GetParticipant + Permission full-replace round-trip (#292).

```bash
git add CHANGELOG.md
git commit -m "docs: CHANGELOG for shipped-broken fixes tranche"
```

- [ ] **Step 2: Rebase onto latest main** (PR #311 may have merged — its crossrefs change is in the server-scalar section, ours in the route section; resolve adjacency conflicts mechanically, keeping both blocks)

```bash
git fetch origin main && git rebase origin/main
go build ./... && go vet ./... && go test ./internal/config/ ./plugins/livekit/ ./internal/expr/
```

- [ ] **Step 3: Full suite**

```bash
go test ./...
gofmt -l .   # only the known pre-existing examples/wasm-helpers hit may appear
```

Expected: all green.

- [ ] **Step 4: Whole-branch review** (per convention: final code-reviewer over the full branch diff), then PR:

```bash
git push -u origin feat/shipped-broken-fixes
gh pr create --title "fix: shipped-broken fixes — upload mapping + validation, cookbook infix ops, livekit source grants" \
  --body "$(cat <<'EOF'
Tranche 2 of the open-issue backlog (spec + plan on branch under docs/superpowers/).

- saas-backend upload-attachment route: adds the missing "file" trigger.input mapping — every real upload 4xxed (#302); plus a crossrefs validation so a files entry without a matching input key fails `noda validate` (the class has shipped twice).
- Expression cookbook: contains/startsWith/endsWith/matches documented as the infix operators they are; compile-based regression test pins it (#308).
- lk.token: canPublishSources now case-insensitive, unknown values error instead of silently locking the token out of publishing (#309).
- lk.participantUpdate: single-source permission setter table, empty-permissions skip, test-fidelity fix (#292).

Closes #302
Closes #308
Closes #309
Closes #292

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Wait for the 4 required functional CI checks before merging.
