# Review Closeout (Tranche G) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the last three unshipped findings of the 2026-07-05 review: realtime-6 (lk.participantUpdate silently revokes omitted permissions), auth-3 (reset-password burns the token before validating), auth-4 (login timing oracle under argon2 param drift).

**Architecture:** Three independent fixes. (1) `lk.participantUpdate` fetches current permissions and merges before sending LiveKit's full-replace `Permission`. (2) `auth.set_password` gains an optional `token` config that consumes a reset token and updates the password in ONE transaction; the scaffolded reset-password workflow collapses to that single node. (3) The scaffolded login workflow pads its invalid path to a fixed 500 ms wall-clock deadline (same pattern as the shipped request-password-reset template).

**Tech Stack:** Go, gofiber project layout, gorm (map[string]any), LiveKit protocol v1.45.0 / server-sdk-go v2.16.0, `google.golang.org/protobuf/proto` (Clone), sqlite in-memory unit tests, the internal workflow-test runner for scaffolded suites.

**Spec:** `docs/superpowers/specs/2026-07-07-review-closeout-design.md` (approved).

## Global Constraints

- Worktree `.worktrees/review-closeout`, branch `review-closeout` off main. `docs/superpowers/*` is gitignored — `git add -f` the spec and this plan as the first commit.
- Gate for EVERY task before commit: `gofmt -l .` (empty output), `go vet ./...`, `golangci-lint run`, and `go test -race` on the packages the task touches.
- Tasks 2–4 additionally run `go test -tags=integration ./plugins/auth/` — the committed `testdata/auth` fixture (old workflow shapes) must stay green; it is our backward-compat proof. **Do NOT regenerate `testdata/auth`** — fixture drift is a known issue; a follow-up issue gets filed at finish.
- Commit messages: conventional commits (`fix(livekit): …`, `fix(auth): …`), each ending with the `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>` trailer.
- CHANGELOG: single `### Fixed` / `### Security` sections already exist under `[Unreleased]` — append bullets there (Task 5), never add duplicate category headers.
- `REVIEW-FINDINGS-2026-07-05.md` Shipped-notes are added at FINISH time (PR number needed), not in these tasks.
- Password copy rule: validation errors keep the exact strings `auth: password must be at least 8 characters` / `auth: password must be at most 512 characters`.
- The pad deadline constant is **500** (ms) in shipped templates — test suites mock the pad nodes, so the constant never slows unit tests.

---

### Task 1: realtime-6 — merge-then-send permissions in `lk.participantUpdate`

**Files:**
- Modify: `plugins/livekit/participant_update.go`
- Test: `plugins/livekit/nodes_test.go` (append after `TestParticipantUpdateNode_WithPermissions`, ~line 492)
- Modify: `docs/03-nodes/lk.participantUpdate.md`
- Modify: `go.mod`/`go.sum` (via `go mod tidy` — `google.golang.org/protobuf` becomes a direct dep)

**Interfaces:**
- Consumes: `RoomClient.GetParticipant` (already on the interface, `plugins/livekit/interfaces.go:15`); mock default returns a `ParticipantInfo` with nil `Permission` (`nodes_test.go:80-85`).
- Produces: unexported `mergedPermissions(ctx, svc, room, identity, perms)` — internal only, no cross-task consumers.

**Background for the implementer:** LiveKit's `UpdateParticipantRequest.Permission` is a FULL REPLACE (vendored proto comment: "set to update the participant's permissions", `livekit_room.pb.go:766`). The current code builds a fresh `&lkproto.ParticipantPermission{}` and sets only config-present booleans, so omitted fields are sent as `false` and revoked. `ParticipantPermission` has 9 fields; only 5 are config-settable — the merge preserves the other 4.

- [ ] **Step 1: Write the failing tests**

Append to `plugins/livekit/nodes_test.go` (imports `context`, `fmt`, `lkproto`, `assert`, `require` already present):

```go
func TestParticipantUpdateNode_PartialPermissionsMergeCurrent(t *testing.T) {
	svc := testService()
	var sentReq *lkproto.UpdateParticipantRequest
	svc.Room = &mockRoomClient{
		getParticipantFn: func(_ context.Context, req *lkproto.RoomParticipantIdentity) (*lkproto.ParticipantInfo, error) {
			assert.Equal(t, "r", req.Room)
			assert.Equal(t, "u", req.Identity)
			return &lkproto.ParticipantInfo{
				Sid: "PA_1", Identity: req.Identity,
				Permission: &lkproto.ParticipantPermission{
					CanPublish:        true,
					CanSubscribe:      true,
					CanPublishData:    true,
					CanUpdateMetadata: true, // not config-settable; must survive the merge
				},
			}, nil
		},
		updateParticipantFn: func(_ context.Context, req *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error) {
			sentReq = req
			return &lkproto.ParticipantInfo{Sid: "PA_1", Identity: req.Identity}, nil
		},
	}
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":        "r",
			"identity":    "u",
			"permissions": map[string]any{"canPublish": false},
		},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	require.NotNil(t, sentReq)
	require.NotNil(t, sentReq.Permission)
	assert.False(t, sentReq.Permission.CanPublish, "explicitly-set key must be applied")
	assert.True(t, sentReq.Permission.CanSubscribe, "omitted key must keep its current value")
	assert.True(t, sentReq.Permission.CanPublishData, "omitted key must keep its current value")
	assert.True(t, sentReq.Permission.CanUpdateMetadata, "non-settable field must survive the merge")
}

func TestParticipantUpdateNode_PermissionsNilCurrentUsesZeroBase(t *testing.T) {
	svc := testService()
	var sentReq *lkproto.UpdateParticipantRequest
	svc.Room = &mockRoomClient{
		// default getParticipantFn returns ParticipantInfo with nil Permission
		updateParticipantFn: func(_ context.Context, req *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error) {
			sentReq = req
			return &lkproto.ParticipantInfo{Sid: "PA_1", Identity: req.Identity}, nil
		},
	}
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":        "r",
			"identity":    "u",
			"permissions": map[string]any{"canSubscribe": true},
		},
		testServices(svc))
	require.NoError(t, err)
	require.NotNil(t, sentReq)
	require.NotNil(t, sentReq.Permission)
	assert.True(t, sentReq.Permission.CanSubscribe)
	assert.False(t, sentReq.Permission.CanPublish)
}

func TestParticipantUpdateNode_UnknownPermissionKeyErrors(t *testing.T) {
	svc := testService()
	rpcCalled := false
	svc.Room = &mockRoomClient{
		getParticipantFn: func(_ context.Context, _ *lkproto.RoomParticipantIdentity) (*lkproto.ParticipantInfo, error) {
			rpcCalled = true
			return &lkproto.ParticipantInfo{}, nil
		},
		updateParticipantFn: func(_ context.Context, _ *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error) {
			rpcCalled = true
			return &lkproto.ParticipantInfo{}, nil
		},
	}
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":        "r",
			"identity":    "u",
			"permissions": map[string]any{"canpublish": false}, // typo: lowercase p
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown permission key "canpublish"`)
	assert.False(t, rpcCalled, "no RPC may be sent for a rejected permissions map")
}

func TestParticipantUpdateNode_NonBoolPermissionValueErrors(t *testing.T) {
	svc := testService()
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":        "r",
			"identity":    "u",
			"permissions": map[string]any{"canPublish": "true"},
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `permission key "canPublish" must be a boolean`)
}

func TestParticipantUpdateNode_GetParticipantErrorSurfaces(t *testing.T) {
	svc := testService()
	updateCalled := false
	svc.Room = &mockRoomClient{
		getParticipantFn: func(_ context.Context, _ *lkproto.RoomParticipantIdentity) (*lkproto.ParticipantInfo, error) {
			return nil, fmt.Errorf("room not found")
		},
		updateParticipantFn: func(_ context.Context, _ *lkproto.UpdateParticipantRequest) (*lkproto.ParticipantInfo, error) {
			updateCalled = true
			return &lkproto.ParticipantInfo{}, nil
		},
	}
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"room":        "r",
			"identity":    "u",
			"permissions": map[string]any{"canPublish": true},
		},
		testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "room not found")
	assert.False(t, updateCalled)
}

func TestParticipantUpdateNode_MetadataOnlySkipsGetParticipant(t *testing.T) {
	svc := testService()
	getCalled := false
	svc.Room = &mockRoomClient{
		getParticipantFn: func(_ context.Context, _ *lkproto.RoomParticipantIdentity) (*lkproto.ParticipantInfo, error) {
			getCalled = true
			return &lkproto.ParticipantInfo{}, nil
		},
	}
	exec := &participantUpdateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"room": "r", "identity": "u", "metadata": "m"},
		testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.False(t, getCalled, "metadata-only update must not fetch permissions")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./plugins/livekit/ -run 'TestParticipantUpdateNode_' -v`
Expected: the six new tests FAIL (merge/unknown-key/non-bool behavior not implemented); `TestParticipantUpdateNode_Success` and `_WithPermissions` still PASS.

- [ ] **Step 3: Implement merge-then-send**

In `plugins/livekit/participant_update.go`, replace the permissions block (current lines 75–95) with:

```go
	if perms, err := plugin.ResolveOptionalMap(nCtx, config, "permissions"); err != nil {
		return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
	} else if perms != nil {
		perm, err := mergedPermissions(ctx, svc, room, identity, perms)
		if err != nil {
			return "", nil, fmt.Errorf("lk.participantUpdate: %w", err)
		}
		req.Permission = perm
	}
```

And add below `Execute`:

```go
// mergedPermissions fetches the participant's current permission set and
// overlays the boolean keys present in perms. LiveKit's
// UpdateParticipantRequest.Permission is a full replace: sending only the
// changed fields would silently revoke every omitted permission. The read
// and the update are two RPCs, so a concurrent permission change between
// them can be lost.
func mergedPermissions(ctx context.Context, svc *Service, room, identity string, perms map[string]any) (*lkproto.ParticipantPermission, error) {
	for key, val := range perms {
		switch key {
		case "canPublish", "canSubscribe", "canPublishData", "hidden", "recorder":
			if _, ok := val.(bool); !ok {
				return nil, fmt.Errorf("permission key %q must be a boolean, got %T", key, val)
			}
		default:
			return nil, fmt.Errorf("unknown permission key %q", key)
		}
	}

	current, err := svc.Room.GetParticipant(ctx, &lkproto.RoomParticipantIdentity{Room: room, Identity: identity})
	if err != nil {
		return nil, fmt.Errorf("get current permissions: %w", err)
	}
	perm := &lkproto.ParticipantPermission{}
	if p := current.GetPermission(); p != nil {
		perm = proto.Clone(p).(*lkproto.ParticipantPermission)
	}
	if v, ok := perms["canPublish"].(bool); ok {
		perm.CanPublish = v
	}
	if v, ok := perms["canSubscribe"].(bool); ok {
		perm.CanSubscribe = v
	}
	if v, ok := perms["canPublishData"].(bool); ok {
		perm.CanPublishData = v
	}
	if v, ok := perms["hidden"].(bool); ok {
		perm.Hidden = v
	}
	if v, ok := perms["recorder"].(bool); ok {
		perm.Recorder = v //nolint:staticcheck // no replacement available in ParticipantPermission; ParticipantInfo.kind is not settable here
	}
	return perm, nil
}
```

Add `"google.golang.org/protobuf/proto"` to the imports. Update the descriptor's `permissions` schema description to `"Permission overrides (merged with the participant's current permissions; unknown or non-boolean keys are rejected)"`.

Note: `proto.Clone` on a deprecated-field-carrying message is fine — only direct field access trips SA1019, and the clone copies `Recorder`/`Agent` without naming them.

Behavior changes (both approved in the spec, the second a direct corollary): unknown keys and non-boolean values in `permissions` now error instead of being silently ignored — both were exactly the destructive-typo class the merge exists to kill.

- [ ] **Step 4: Tidy the module and run the tests**

Run: `go mod tidy && go test ./plugins/livekit/ -race -v -run 'TestParticipantUpdate'`
Expected: all PASS (including the two pre-existing tests). `git diff go.mod` should show `google.golang.org/protobuf` moved out of the `// indirect` block — nothing else.

- [ ] **Step 5: Update the node doc**

In `docs/03-nodes/lk.participantUpdate.md`:
1. Permission Keys table: add the missing `recorder` row (`boolean | Mark as a recorder instance (deprecated upstream)`), and under the table add:

```markdown
Permissions are **merged**: the node reads the participant's current permission
set (one extra `GetParticipant` call) and overlays only the keys you provide,
so omitted keys keep their current values. Unknown or non-boolean keys are
rejected with an error. The read and the write are two calls — a concurrent
permission change in between can be lost.
```

2. Behavior section: replace the last sentence ("At least one of…") with:

```markdown
At least one of `metadata` or `permissions` should be provided. When
`permissions` is present the node first fetches the participant's current
permissions and merges your overrides into them (LiveKit replaces the whole
permission set on update — before this merge, omitting a key silently revoked
it). Fires `success` with the updated participant object.
```

- [ ] **Step 6: Gate and commit**

Run: `gofmt -l . && go vet ./... && golangci-lint run && go test -race ./plugins/livekit/`
Expected: gofmt silent, vet/lint clean, tests PASS.

```bash
git add plugins/livekit/participant_update.go plugins/livekit/nodes_test.go docs/03-nodes/lk.participantUpdate.md go.mod go.sum
git commit -m "fix(livekit): merge participant permissions before update (realtime-6)

lk.participantUpdate now fetches the participant's current permissions and
overlays only the config-provided keys before sending LiveKit's full-replace
Permission message — a partial map like {\"canPublish\": false} no longer
silently revokes canSubscribe/canPublishData/hidden. Unknown or non-boolean
permission keys are rejected instead of silently ignored.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: auth-3 (plugin side) — atomic token mode for `auth.set_password` + rune-based validation

**Files:**
- Modify: `plugins/auth/helpers.go:77-85` (`validatePassword`)
- Modify: `plugins/auth/one_time_tokens.go` (extract `consumeTokenInTx`, rewire `consume_token`)
- Modify: `plugins/auth/set_password.go` (token mode, `invalid` output, single transaction)
- Create: `plugins/auth/helpers_test.go`
- Test: `plugins/auth/set_password_test.go` (extend; update `TestSetPasswordDescriptor`)
- Modify: `docs/03-nodes/auth.set_password.md`

**Interfaces:**
- Consumes: `HashToken(raw string) string`, `PurposeResetPassword` const, `plugin.ResolveOptionalString(nCtx, config, key) (string, bool, error)`, test helpers `newTestDB`/`testService`/`testServices`/`fakeCtx` (`schema_test.go`), `seedUser` (`verify_credentials_test.go:14`), `newCreateTokenExecutor` (mints a real token, returns `{token, expires_at}`).
- Produces: `consumeTokenInTx(tx *gorm.DB, hash, purpose string, now time.Time) (userID string, invalid bool, err error)`; `auth.set_password` node contract consumed by Task 3's template: optional config `token` (mutually exclusive with `user_id`), outputs `success` / `invalid` / `error`.

- [ ] **Step 1: Write the failing validatePassword tests**

Create `plugins/auth/helpers_test.go`:

```go
package auth

import (
	"strings"
	"testing"
)

// validatePassword must count runes (code points), not bytes, so it agrees
// exactly with the scaffolded route schemas' JSON-Schema minLength/maxLength
// (which count code points). Divergence burns real requests: a password that
// passes the route 400-check but fails here surfaces as a 500 — and, before
// set_password consumed tokens atomically, burned the reset token (auth-3).
func TestValidatePasswordCountsRunes(t *testing.T) {
	cases := []struct {
		name    string
		pw      string
		wantErr error
	}{
		{"7 ascii too short", "abcdefg", errPasswordTooShort},
		{"8 ascii ok", "abcdefgh", nil},
		{"4 emoji (16 bytes) too short by runes", "😀😀😀😀", errPasswordTooShort},
		{"8 emoji (32 bytes) ok", "😀😀😀😀😀😀😀😀", nil},
		{"512 two-byte runes (1024 bytes) ok", strings.Repeat("é", 512), nil},
		{"513 runes too long", strings.Repeat("a", 513), errPasswordTooLong},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePassword(tc.pw)
			if tc.wantErr == nil && err != nil {
				t.Fatalf("want nil error, got %v", err)
			}
			if tc.wantErr != nil && err != tc.wantErr {
				t.Fatalf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./plugins/auth/ -run TestValidatePasswordCountsRunes -v`
Expected: FAIL on "4 emoji (16 bytes) too short by runes" (byte-count 16 passes today) and "512 two-byte runes (1024 bytes) ok" (byte-count 1024 fails today).

- [ ] **Step 3: Switch validatePassword to runes**

In `plugins/auth/helpers.go`, add `"unicode/utf8"` to imports and replace the function:

```go
// validatePassword counts runes, matching the code-point semantics of the
// scaffolded routes' JSON-Schema minLength/maxLength — the two layers must
// agree or a schema-passing password fails here after side effects ran.
func validatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < 8 {
		return errPasswordTooShort
	}
	if n > 512 {
		return errPasswordTooLong
	}
	return nil
}
```

- [ ] **Step 4: Run rune tests + existing suite**

Run: `go test ./plugins/auth/ -race`
Expected: all PASS (existing tests use ASCII passwords; behavior unchanged for them).

- [ ] **Step 5: Write the failing set_password token-mode tests**

Append to `plugins/auth/set_password_test.go`:

```go
// mintResetToken creates a real reset_password token for userID and returns
// the raw token string.
func mintResetToken(t *testing.T, db *gorm.DB, userID string) string {
	t.Helper()
	create := newCreateTokenExecutor(nil)
	out, data, err := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": PurposeResetPassword,
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("mint token: out=%q err=%v", out, err)
	}
	return data.(map[string]any)["token"].(string)
}

func TestSetPasswordWithTokenConsumesAtomically(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	oldHash, _ := svc.HashPassword("oldpassword")
	userID := seedUser(t, db, "alice@example.com", oldHash, "active")
	token := mintResetToken(t, db, userID)

	set := newSetPasswordExecutor(nil)
	out, data, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "password": "newpassword123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if _, ok := data.(map[string]any)["revoked_sessions"]; !ok {
		t.Fatal("success output must include revoked_sessions")
	}

	verify := newVerifyCredentialsExecutor(nil)
	out, _, _ = verify.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "newpassword123",
	}, testServices(db))
	if out != api.OutputSuccess {
		t.Fatal("new password must verify")
	}

	// The token is single-use: a second attempt must be invalid.
	out, _, err = set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "password": "anotherpassword1",
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("reused token: out=%q err=%v", out, err)
	}
}

func TestSetPasswordWithUnknownTokenIsInvalid(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	oldHash, _ := svc.HashPassword("oldpassword")
	seedUser(t, db, "alice@example.com", oldHash, "active")

	set := newSetPasswordExecutor(nil)
	out, _, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": "never-minted", "password": "newpassword123",
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("out=%q err=%v", out, err)
	}

	verify := newVerifyCredentialsExecutor(nil)
	out, _, _ = verify.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "oldpassword",
	}, testServices(db))
	if out != api.OutputSuccess {
		t.Fatal("password must be unchanged after an invalid token")
	}
}

// TestSetPasswordTokenSurvivesBadPassword is the auth-3 regression test: a
// rejected new password must NOT burn the reset token.
func TestSetPasswordTokenSurvivesBadPassword(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	oldHash, _ := svc.HashPassword("oldpassword")
	userID := seedUser(t, db, "alice@example.com", oldHash, "active")
	token := mintResetToken(t, db, userID)

	set := newSetPasswordExecutor(nil)
	out, _, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "password": "short",
	}, testServices(db))
	if err == nil {
		t.Fatalf("bad password must error, got out=%q", out)
	}

	// Same token must still work with a valid password.
	out, _, err = set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "password": "newpassword123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("token must survive a rejected password: out=%q err=%v", out, err)
	}
}

func TestSetPasswordUserIDTokenMutuallyExclusive(t *testing.T) {
	db := newTestDB(t)
	set := newSetPasswordExecutor(nil)

	if _, _, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": "u1", "token": "tok", "password": "newpassword123",
	}, testServices(db)); err == nil {
		t.Fatal("user_id + token together must error")
	}
	if _, _, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"password": "newpassword123",
	}, testServices(db)); err == nil {
		t.Fatal("neither user_id nor token must error")
	}
}
```

Also update `TestSetPasswordDescriptor` (line 76): the properties count changes `3` → `4`, the outputs count `2` → `3`. Add `"gorm.io/gorm"` to the file's imports (needed by `mintResetToken`).

- [ ] **Step 6: Run to verify failure**

Run: `go test ./plugins/auth/ -run 'TestSetPassword' -v`
Expected: the four new tests FAIL (token config unsupported → today `token` is silently ignored and `user_id` missing resolves to error paths); `TestSetPasswordDescriptor` FAILS on the new counts.

- [ ] **Step 7: Extract consumeTokenInTx and rewire consume_token**

In `plugins/auth/one_time_tokens.go`, add above `consumeTokenDescriptor`:

```go
// consumeTokenInTx atomically claims the (hash, purpose) token inside tx and
// returns its owner. The WHERE guard on consumed_at makes concurrent
// consumption impossible — exactly one UPDATE can match. invalid reports
// unknown/expired/wrong-purpose/already-consumed without distinguishing them.
func consumeTokenInTx(tx *gorm.DB, hash, purpose string, now time.Time) (userID string, invalid bool, err error) {
	res := tx.Table("auth_tokens").
		Where("token_hash = ? AND purpose = ? AND consumed_at IS NULL AND expires_at > ?", hash, purpose, now).
		Update("consumed_at", now)
	if res.Error != nil {
		return "", false, res.Error
	}
	if res.RowsAffected == 0 {
		return "", true, nil
	}
	if err := tx.Table("auth_tokens").
		Where("token_hash = ?", hash).Pluck("user_id", &userID).Error; err != nil {
		return "", false, err
	}
	if userID == "" {
		return "", false, fmt.Errorf("consumed token row disappeared")
	}
	return userID, false, nil
}
```

Rewire `consumeTokenExecutor.Execute`'s transaction body (current lines 171–200) to use it, keeping the existing atomicity comment above the `Transaction` call and the verify_email side effect:

```go
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		uid, inv, err := consumeTokenInTx(tx, hash, purpose, now)
		if err != nil {
			return err
		}
		if inv {
			invalid = true
			return nil
		}
		userID = uid

		if purpose == PurposeVerifyEmail {
			if err := tx.Table("auth_users").Where("id = ?", userID).
				Updates(map[string]any{"email_verified_at": now, "updated_at": now}).Error; err != nil {
				return fmt.Errorf("mark verified: %w", err)
			}
		}
		return nil
	})
```

Run: `go test ./plugins/auth/ -run 'TestConsume|TestCreateToken|TestOneTime' -v` — the pre-existing token tests must still PASS (pure refactor).

- [ ] **Step 8: Implement set_password token mode**

Replace `plugins/auth/set_password.go`'s descriptor pieces and `Execute`:

Descriptor changes:
```go
func (d *setPasswordDescriptor) Description() string {
	return "Sets a new password (argon2id), optionally consuming a reset token atomically, and revokes the user's sessions"
}
```
`ConfigSchema` properties gain:
```go
			"token": map[string]any{"type": "string", "description": "Password-reset token to consume atomically in the same transaction (expression); mutually exclusive with user_id"},
```
and `"required"` changes from `[]any{"user_id", "password"}` to `[]any{"password"}` (mutual exclusion is enforced at execute time — JSON Schema in this codebase doesn't express oneOf, and the editor treats the schema as advisory).

```go
func (d *setPasswordDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "{revoked_sessions} count",
		"invalid": "Reset token unknown, expired, or already used (token mode only)",
		"error":   "Infrastructure error, unknown user, or invalid new password",
	}
}
```

Executor:
```go
func (e *setPasswordExecutor) Outputs() []string { return []string{"success", "invalid", "error"} }

func (e *setPasswordExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	svc, err := plugin.GetService[*Service](services, "auth")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	userID, hasUserID, err := plugin.ResolveOptionalString(nCtx, config, "user_id")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	token, hasToken, err := plugin.ResolveOptionalString(nCtx, config, "token")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	if hasUserID == hasToken {
		return "", nil, fmt.Errorf("auth.set_password: exactly one of user_id or token must be set")
	}
	password, err := plugin.ResolveString(nCtx, config, "password")
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	// Validate before any DB write: in token mode a rejected password must
	// not consume the token (auth-3).
	if err := validatePassword(password); err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	revoke := true
	if v, ok := config["revoke_sessions"].(bool); ok {
		revoke = v
	}

	hash, err := svc.HashPassword(password)
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	now := time.Now().UTC()

	var revoked int64
	invalid := false
	// Token consumption, the password update, and session revocation commit
	// or fail together: any failure rolls back the consume, so the token
	// stays usable for a retry.
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		uid := userID
		if hasToken {
			var inv bool
			var err error
			uid, inv, err = consumeTokenInTx(tx, HashToken(token), PurposeResetPassword, now)
			if err != nil {
				return err
			}
			if inv {
				invalid = true
				return nil
			}
		}
		res := tx.Table("auth_users").Where("id = ?", uid).
			Updates(map[string]any{"password_hash": hash, "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("user not found")
		}
		if revoke {
			r := tx.Table("auth_sessions").
				Where("user_id = ? AND revoked_at IS NULL", uid).
				Update("revoked_at", now)
			if r.Error != nil {
				return fmt.Errorf("revoke sessions: %w", r.Error)
			}
			revoked = r.RowsAffected
		}
		return nil
	})
	if err != nil {
		return "", nil, fmt.Errorf("auth.set_password: %w", err)
	}
	if invalid {
		return "invalid", map[string]any{}, nil
	}
	return api.OutputSuccess, map[string]any{"revoked_sessions": revoked}, nil
}
```

Note the wrapping change for existing callers: `user not found` now surfaces as `auth.set_password: user not found` (same text as before — the wrap moved from the statement to the transaction return). `TestSetPasswordUnknownUser` only checks `err != nil`, so it stays green.

- [ ] **Step 9: Run the full auth package**

Run: `go test ./plugins/auth/ -race`
Expected: all PASS, including the four new token-mode tests, the rune tests, and every pre-existing test (user_id mode is behavior-identical apart from now running inside one transaction).

- [ ] **Step 10: Backward-compat gate (integration fixture)**

Run: `go test -tags=integration ./plugins/auth/`
Expected: PASS. The committed `testdata/auth` fixture still uses the OLD two-node reset workflow (`consume` → `set_password` with `user_id`) — it passing proves user_id mode is untouched. Do NOT regenerate the fixture.

- [ ] **Step 11: Update the node doc**

Rewrite `docs/03-nodes/auth.set_password.md`:
- Config table: `user_id` Required column becomes `one of` with description "User id (expression); mutually exclusive with `token`"; add row `token | string (expr) | one of | Password-reset token to consume atomically; mutually exclusive with user_id`.
- Outputs: `success`, `invalid`, `error`; add: "`invalid` is returned only in token mode when the token is unknown, expired, or already used (undifferentiated, mirroring `auth.consume_token`)."
- Behavior: change "8–512 characters" to "8–512 characters (Unicode code points, matching the scaffolded route schemas)" and append:

```markdown
In **token mode** (`token` instead of `user_id`) the node consumes a
`reset_password` one-time token and updates the password in a single
transaction: the password is validated *before* any write, and a failure at
any later step rolls the consumption back, so a valid reset token is never
burned by a rejected password or an infrastructure error. The user is the
token's owner; no `user_id` is needed.
```

- "With data flow" example: replace the two-node snippet with the new single-node shape:

```json
{
  "set_password": {
    "type": "auth.set_password",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "token": "{{ input.token }}", "password": "{{ input.password }}" }
  }
}
```
and update the surrounding sentence to say the scaffolded `auth-reset-password` workflow wires `invalid` → a 400 response.

- [ ] **Step 12: Gate and commit**

Run: `gofmt -l . && go vet ./... && golangci-lint run && go test -race ./plugins/auth/`
Expected: clean/PASS.

```bash
git add plugins/auth/helpers.go plugins/auth/helpers_test.go plugins/auth/one_time_tokens.go plugins/auth/set_password.go plugins/auth/set_password_test.go docs/03-nodes/auth.set_password.md
git commit -m "fix(auth): atomic token mode for set_password; validate password by runes (auth-3)

auth.set_password gains an optional token config that consumes a
reset_password one-time token and updates the password in one transaction
(new 'invalid' output). The password is validated before any write and any
later failure rolls back the consumption, so a rejected password or DB error
no longer burns a valid reset token. validatePassword now counts runes,
matching the scaffolded route schemas' code-point semantics.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: auth-3 (scaffold side) — collapse the reset-password template to the atomic node

**Files:**
- Modify: `cmd/noda/auth_templates/workflows/auth.reset-password.json.tmpl`
- Modify: `cmd/noda/auth_templates/tests/test-auth-reset-password.json`
- Test: `cmd/noda/auth_init_test.go` (add one wrapper test after `TestAuthScaffold_ResendVerificationIsConstantTime`, line ~241)

**Interfaces:**
- Consumes: Task 2's `auth.set_password` contract — config `{token, password}`, outputs `success`/`invalid`/`error`. Test helper `runScaffoldedAuthSuite(t, suiteID)` (`auth_init_test.go:206`) which scaffolds a temp project and runs one scaffolded suite through the real workflow-test runner.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Add the failing Go wrapper test**

In `cmd/noda/auth_init_test.go` after line 241:

```go
func TestAuthScaffold_ResetPasswordIsAtomic(t *testing.T) {
	runScaffoldedAuthSuite(t, "test-auth-reset-password")
}
```

- [ ] **Step 2: Run to see it pass against the OLD template (baseline)**

Run: `go test ./cmd/noda/ -run TestAuthScaffold_ResetPasswordIsAtomic -v`
Expected: PASS (the old suite mocks the old `consume` node). This step is the baseline proving the wrapper wiring works; the template/suite swap below must keep it green.

- [ ] **Step 3: Collapse the workflow template**

Replace the whole of `cmd/noda/auth_templates/workflows/auth.reset-password.json.tmpl` with:

```json
{
  "id": "auth-reset-password",
  "name": "Auth: Reset Password",
  "nodes": {
    "set_password": {
      "type": "auth.set_password",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "token": "{{ input.token }}", "password": "{{ input.password }}" }
    },
    "respond_invalid": {
      "type": "response.json",
      "config": { "status": 400, "body": { "error": "invalid or expired token" } }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 200, "body": { "message": "password updated" } }
    }
  },
  "edges": [
    { "from": "set_password", "to": "respond" },
    { "from": "set_password", "output": "invalid", "to": "respond_invalid" }
  ]
}
```

- [ ] **Step 4: Update the scaffolded test suite**

Replace `cmd/noda/auth_templates/tests/test-auth-reset-password.json` with:

```json
{
  "id": "test-auth-reset-password",
  "workflow": "auth-reset-password",
  "tests": [
    {
      "name": "valid token resets the password",
      "input": { "token": "reset-tok", "password": "newpassword123" },
      "mocks": {
        "set_password": { "output": { "revoked_sessions": 1 } },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    },
    {
      "name": "invalid or expired token is rejected",
      "input": { "token": "bad-tok", "password": "newpassword123" },
      "mocks": {
        "set_password": { "output_name": "invalid", "output": {} },
        "respond_invalid": { "output": { "status": 400 } }
      },
      "expect": { "status": "success", "output": { "respond_invalid.status": 400 } }
    }
  ]
}
```

- [ ] **Step 5: Run the scaffold tests**

Run: `go test ./cmd/noda/ -race -run 'TestAuthScaffold|TestAuthInit'`
Expected: all PASS — in particular `TestAuthScaffold_ResetPasswordIsAtomic` (new suite against new template), `TestAuthInitScaffold` (file list unchanged), `TestAuthInitOutputValidates` (rendered config still validates), and `TestAuthInitRegisterRouteEnforcesPasswordLength` (route untouched).

- [ ] **Step 6: Gate and commit**

Run: `gofmt -l . && go vet ./... && golangci-lint run && go test -race ./cmd/noda/ && go test -tags=integration ./plugins/auth/`
Expected: clean/PASS (integration fixture still on the old shape — stays green).

```bash
git add cmd/noda/auth_templates/workflows/auth.reset-password.json.tmpl cmd/noda/auth_templates/tests/test-auth-reset-password.json cmd/noda/auth_init_test.go
git commit -m "fix(auth-scaffold): reset-password flow consumes the token atomically (auth-3)

The scaffolded auth-reset-password workflow collapses to a single
auth.set_password node in token mode: token consumption and the password
update commit or roll back together, so a failure after consumption can no
longer burn the reset token and strand the user.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: auth-4 — wall-clock floor on the scaffolded login flow's invalid path

**Files:**
- Modify: `cmd/noda/auth_templates/workflows/auth.login.json.tmpl`
- Modify: `cmd/noda/auth_templates/tests/test-auth-login.json`
- Test: `cmd/noda/auth_init_test.go` (two wrapper tests)
- Modify: `plugins/auth/crypto.go:99-102` (VerifyDummy comment only)
- Modify: `docs/04-guides/authentication.md` (~line 357, the constant-time bullet)

**Interfaces:**
- Consumes: `util.timestamp` (`format: "unix_ms"`), `util.delay` (per-request `timeout` expression — shipped in the auth tranche), `runScaffoldedAuthSuite`, `scaffoldAuthProject(t) string` (`auth_init_test.go:195`).
- Produces: nothing consumed by later tasks.

**Background for the implementer:** `VerifyDummy` burns a hash at the *configured* argon2 params when the email is unknown. If a deployment raises argon2 cost, existing users' stored hashes verify at their embedded OLD (cheaper) params, so wrong-password-on-real-account is measurably FASTER than unknown-email — a login timing oracle (auth-4). CPU-equalization can't fix this (hash costs are heterogeneous until rehash-on-login converges), so the scaffold pads the entire invalid path to a fixed wall-clock deadline instead — the exact pattern `auth.request-password-reset.json.tmpl` already uses.

- [ ] **Step 1: Write the failing tests**

In `cmd/noda/auth_init_test.go`, after the reset-password wrapper from Task 3:

```go
func TestAuthScaffold_LoginPadsInvalidPath(t *testing.T) {
	runScaffoldedAuthSuite(t, "test-auth-login")
}

// TestAuthInitLoginPadExpressionWiring guards the pad expression's node
// references in the rendered login workflow. The scaffolded suite mocks
// pad_invalid, so a typo like `nodes.now_ts_invalld` would pass the suite
// and only explode at runtime (see
// TestScratch_PasswordResetPadExpressionResolvesUnmocked for why the
// expression itself is known-good).
func TestAuthInitLoginPadExpressionWiring(t *testing.T) {
	dir := scaffoldAuthProject(t)
	b, err := os.ReadFile(filepath.Join(dir, "workflows", "auth.login.json"))
	require.NoError(t, err)
	s := string(b)
	assert.Contains(t, s, "nodes.start_ts + 500")
	assert.Contains(t, s, "nodes.now_ts_invalid")
}
```

- [ ] **Step 2: Run to verify the wiring test fails**

Run: `go test ./cmd/noda/ -run 'TestAuthScaffold_LoginPadsInvalidPath|TestAuthInitLoginPadExpressionWiring' -v`
Expected: `TestAuthInitLoginPadExpressionWiring` FAILS (no pad nodes yet); `TestAuthScaffold_LoginPadsInvalidPath` PASSES against the old template (baseline — must stay green after the swap).

- [ ] **Step 3: Add the pad chain to the login template**

Replace the whole of `cmd/noda/auth_templates/workflows/auth.login.json.tmpl` with:

```json
{
  "id": "auth-login",
  "name": "Auth: Login",
  "nodes": {
    "start_ts": { "type": "util.timestamp", "config": { "format": "unix_ms" } },
    "verify": {
      "type": "auth.verify_credentials",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "email": "{{ input.email }}", "password": "{{ input.password }}" }
    },
    "now_ts_invalid": { "type": "util.timestamp", "config": { "format": "unix_ms" } },
    "pad_invalid": { "type": "util.delay", "config": { "timeout": "{{ (nodes.start_ts + 500) > nodes.now_ts_invalid ? (nodes.start_ts + 500 - nodes.now_ts_invalid) : 0 }}ms" } },
    "respond_invalid": {
      "type": "response.json",
      "config": { "status": 401, "body": { "error": "invalid credentials" } }
    },
    "session": {
      "type": "auth.create_session",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "user_id": "{{ nodes.verify.id }}" }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "user": "{{ nodes.verify }}", "token": "{{ nodes.session.token }}" },
        "cookies": "{{ [nodes.session.cookie] }}"
      }
    }
  },
  "edges": [
    { "from": "start_ts", "to": "verify" },
    { "from": "verify", "to": "session" },
    { "from": "verify", "output": "invalid", "to": "now_ts_invalid" },
    { "from": "now_ts_invalid", "to": "pad_invalid" },
    { "from": "pad_invalid", "to": "respond_invalid" },
    { "from": "session", "to": "respond" }
  ]
}
```

(Unknown-email and wrong-password both exit `verify` via `invalid`, so both flatten to the same ~500 ms deadline; the success path is unpadded — someone holding the correct password is not enumerating.)

- [ ] **Step 4: Update the scaffolded login suite**

Replace `cmd/noda/auth_templates/tests/test-auth-login.json` with (both cases now mock `start_ts`; the invalid case mocks the pad chain, mirroring `test-auth-request-password-reset.json`):

```json
{
  "id": "test-auth-login",
  "workflow": "auth-login",
  "tests": [
    {
      "name": "valid credentials get a session",
      "input": { "email": "alice@example.com", "password": "password123" },
      "mocks": {
        "start_ts": { "output": 1000 },
        "verify": { "output": { "id": "user-1", "email": "alice@example.com", "roles": ["user"] } },
        "session": { "output": { "token": "tok", "session_id": "s1", "cookie": { "name": "noda_session", "value": "tok" } } },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    },
    {
      "name": "invalid credentials get 401 at the fixed deadline",
      "input": { "email": "alice@example.com", "password": "wrong" },
      "mocks": {
        "start_ts": { "output": 1000 },
        "verify": { "output_name": "invalid", "output": {} },
        "now_ts_invalid": { "output": 1100 },
        "pad_invalid": { "output": null },
        "respond_invalid": { "output": { "status": 401 } }
      },
      "expect": { "status": "success", "output": { "respond_invalid.status": 401 } }
    }
  ]
}
```

- [ ] **Step 5: Run the scaffold tests**

Run: `go test ./cmd/noda/ -race -run 'TestAuthScaffold|TestAuthInit'`
Expected: all PASS, including both Step-1 tests against the new template.

- [ ] **Step 6: Rewrite the VerifyDummy comment**

In `plugins/auth/crypto.go`, replace the comment above `VerifyDummy` (lines 99–102) with:

```go
// VerifyDummy is called when no user matches, so response timing does not
// reveal account existence. The dummy hash is derived from the service's
// *currently configured* params, which only equalizes CPU time while stored
// hashes carry those same params. After a cost raise, existing hashes verify
// at their embedded (cheaper) params until rehash-on-login upgrades them, so
// a wrong password on a legacy account completes faster than this dummy — a
// drift oracle no single dummy can close, because stored hash costs are
// heterogeneous. The scaffolded login flow closes it with a wall-clock pad
// on the invalid path (util.timestamp + util.delay to a fixed deadline);
// custom login flows should copy that pattern.
```

- [ ] **Step 7: Extend the authentication guide**

In `docs/04-guides/authentication.md`, the constant-time bullet (~line 357) covers password-reset and resend-verification. Directly after it, add:

```markdown
- **Login pads invalid responses to the same fixed deadline.** `auth.verify_credentials` burns a dummy argon2 hash for unknown emails, but the dummy uses the *currently configured* params: after you raise argon2 cost, existing users' stored hashes still verify at their old (cheaper) embedded params until each user logs in successfully and is rehashed — so a wrong password on a real account would return faster than an unknown email, re-opening enumeration. The login template therefore pads the whole `invalid` branch to a ~500 ms deadline with the same `util.timestamp` + `util.delay` chain. If your argon2 params are heavy enough that verification alone approaches 500 ms, raise the deadline in `pad_invalid` — a pad that clamps to 0 protects nothing. Custom login flows should copy this pattern; projects scaffolded before this template change should add it manually.
```

- [ ] **Step 8: Gate and commit**

Run: `gofmt -l . && go vet ./... && golangci-lint run && go test -race ./cmd/noda/ ./plugins/auth/ && go test -tags=integration ./plugins/auth/`
Expected: clean/PASS (crypto.go change is comment-only; the integration fixture's old login workflow has no pad and stays green).

```bash
git add cmd/noda/auth_templates/workflows/auth.login.json.tmpl cmd/noda/auth_templates/tests/test-auth-login.json cmd/noda/auth_init_test.go plugins/auth/crypto.go docs/04-guides/authentication.md
git commit -m "fix(auth-scaffold): pad login's invalid path to a fixed deadline (auth-4)

Wrong-password and unknown-email responses now flatten to a ~500 ms
wall-clock deadline in the scaffolded login flow, closing the timing oracle
that re-opens account enumeration after an argon2 cost raise (stored hashes
verify at their embedded old params while unknown emails burn the new,
heavier dummy). Also rewrites the VerifyDummy comment, which reasoned only
about the opposite drift direction, and documents the caveat + pattern for
custom flows.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: CHANGELOG entry

**Files:**
- Modify: `CHANGELOG.md` (the existing `### Fixed` and `### Security` sections under `[Unreleased]` — append bullets; do NOT create new category headers)

**Interfaces:**
- Consumes: the shipped behavior of Tasks 1–4.
- Produces: nothing.

- [ ] **Step 1: Append the bullets**

At the END of the existing `### Fixed` list under `[Unreleased]`:

```markdown
- `lk.participantUpdate` now merges `permissions` with the participant's current permission set (one extra `GetParticipant` call) instead of full-replacing it — a partial map like `{"canPublish": false}` no longer silently revokes `canSubscribe`/`canPublishData`/`hidden`. Unknown or non-boolean permission keys are now rejected instead of silently ignored.
- `auth.set_password` gained an optional `token` config that consumes a `reset_password` one-time token and updates the password in a single transaction (new `invalid` output); the scaffolded reset-password flow uses it, so a failure after token consumption (rejected password, DB error) no longer burns the reset token. Password length validation now counts characters (runes) instead of bytes, matching the scaffolded route schemas' code-point semantics.
```

At the END of the existing `### Security` list under `[Unreleased]`:

```markdown
- The scaffolded login flow now pads invalid-credential responses to a fixed ~500 ms deadline (`util.timestamp` + `util.delay`, same pattern as password-reset/resend-verification), closing a timing oracle that re-opened account enumeration after an argon2 cost raise: stored hashes verify at their embedded old params while unknown emails burn the new, heavier dummy hash, so wrong-password-on-real-account responded measurably faster than unknown-email. Projects scaffolded earlier should add the pad manually (see the authentication guide); if argon2 verification alone approaches 500 ms, raise the deadline.
```

- [ ] **Step 2: Gate and commit**

Run: `go build ./... && go vet ./...`
Expected: clean (docs-only change; cheap sanity gate).

```bash
git add CHANGELOG.md
git commit -m "docs(changelog): review-closeout tranche (realtime-6, auth-3, auth-4)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## After all tasks (finishing checklist — not part of task execution)

1. Final whole-branch review **on opus** (auth-sensitive tranche).
2. `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./plugins/livekit/ ./plugins/auth/ ./cmd/noda/`, `go test -tags=integration ./plugins/auth/` — one last full pass on the branch tip.
3. Push, create PR (squash-merge; UNSTABLE = pending non-required `benchmark` check is safe once the 4 functional checks are green).
4. Add **✅ Shipped PR #N** notes to realtime-6, auth-3, auth-4 in `REVIEW-FINDINGS-2026-07-05.md` (inline, matching the existing format), update the header's "not yet bucketed" list (line 9) to remove all three, and note the tranche in the "Suggested tranches" block — commit on the PR branch before merge.
5. File follow-up issues:
   - `testdata/auth` fixture drift: it was generated at #247 and never regenerated — it lacks resend-verification, the pad chains, and (after this tranche) the atomic reset shape, so the integration e2e exercises stale workflows. Regenerate + add a guard or a documented regeneration step.
   - Any items deferred during review.
6. Memory update (tranche file + MEMORY.md index line).
