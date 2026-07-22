# Auth SQL Error Classification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `plugins/auth`'s SQL failures produce the same typed API errors that `plugins/db` has produced since #403, so an identical driver condition means the same thing at every call site.

**Architecture:** Export the existing `classifyOr` helper from `internal/dberr` so both plugins share one definition, apply it at the nine `plugins/auth` gorm call sites, and teach the session middleware to honor `ServiceUnavailableError`/`TimeoutError` instead of hardcoding 500.

**Tech Stack:** Go 1.26, gorm v1.31.1, `github.com/mattn/go-sqlite3` (via `gorm.io/driver/sqlite`), `github.com/jackc/pgx/v5/pgconn`, testify, fiber v3.

**Spec:** `docs/superpowers/specs/2026-07-22-auth-error-classification-design.md`

## Global Constraints

- Do **not** modify the `go` or `toolchain` directives in `go.mod`. Raising them hard-fails the wasm-guests CI job (TinyGo pin coupling).
- No new dependencies. Everything needed is already in `go.mod`.
- `resource` strings passed to `ClassifyOr` must be logical names (`user`, `session`, `token`), never table names — `MapErrorToHTTP` renders `conflict on %s` to production clients (`internal/server/errors.go:68`).
- Node output edges must not change. No `"success"` / `"invalid"` / `"exists"` / `"not_found"` behavior changes anywhere in this plan.
- `docs/superpowers/` is gitignored (`.gitignore:64`). Commit files there with `git add -f`, as all 73 existing specs/plans were.
- Run `gofmt -l .` before every commit; CI's golangci-lint catches formatting that `go vet` does not.

## Three invariants that must survive every task

Copied verbatim from the spec. These are load-bearing security behaviors from the anti-enumeration tranche.

1. **`create_user`'s `"exists"` output stays on `IsUniqueViolation`, never `Classify`.** `Classify` also matches foreign-key violations, so switching would convert an FK bug into a false "email already registered" *and* turn the anti-enumeration `"exists"` branch into a 409.
2. **`verify_credentials`' `"invalid"` branches are untouched.** Unknown email, unusable `password_hash`, wrong password, and inactive status keep returning `"invalid"` with flat timing.
3. **Best-effort writes stay ignored.** The `last_used_at` touch (`session_auth.go:59`) and the rehash upgrade (`verify_credentials.go:106`) discard their errors by design. Classifying them would turn a cosmetic write failure into a failed login.

## File Structure

| File | Responsibility | Task |
|---|---|---|
| `internal/dberr/dberr.go` | gains exported `ClassifyOr` | 1 |
| `internal/dberr/classify_or_test.go` | relocated from `plugins/db/errors_test.go` | 1 |
| `plugins/db/errors.go` | **deleted** | 1 |
| `plugins/db/errors_test.go` | **deleted** (moved) | 1 |
| `plugins/db/{create,update,delete,find,find_one,count,upsert,query,exec}.go` | call `dberr.ClassifyOr` | 1 |
| `plugins/auth/classify_test.go` | **new** — injection harness + call-site tests | 2, 3 |
| `plugins/auth/{create_session,revoke_session,get_user,verify_credentials,set_password}.go` | classify 5 sites | 2 |
| `plugins/auth/{one_time_tokens,create_user}.go` | classify 4 sites | 3 |
| `plugins/auth/session_auth.go` | classify 1 site | 4 |
| `internal/server/session_middleware.go` | honor 503/504 | 4 |
| `internal/server/session_middleware_test.go` | middleware status tests | 4 |
| `plugins/auth/classify_integration_test.go` | **new** — cross-driver coverage | 5 |
| `CHANGELOG.md` | behavior-change entries | 5 |

---

### Task 1: Export `ClassifyOr` from `internal/dberr`

**Files:**
- Modify: `internal/dberr/dberr.go` (append function + `fmt` import)
- Create: `internal/dberr/classify_or_test.go`
- Delete: `plugins/db/errors.go`, `plugins/db/errors_test.go`
- Modify: `plugins/db/create.go:76`, `update.go:79`, `delete.go:68`, `find.go:86`, `find_one.go:95`, `count.go:86`, `upsert.go:91`, `query.go:70`, `exec.go:69`

**Interfaces:**
- Consumes: existing `dberr.Classify(err error, resource string) error`
- Produces: `dberr.ClassifyOr(err error, resource, op string) error` — returns a typed `*api.ConflictError` / `*api.ValidationError` / `*api.ServiceUnavailableError` / `*api.TimeoutError` when `Classify` matches, otherwise `fmt.Errorf("%s: %w", op, err)`. Every later task calls this.

- [ ] **Step 1: Write the failing test**

Create `internal/dberr/classify_or_test.go`. This is the content of the deleted `plugins/db/errors_test.go`, repointed at the exported name:

```go
package dberr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/chimpanze/noda/internal/dberr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A classifiable driver error becomes a typed api error, and the node's
// context string is deliberately dropped in favour of the typed error.
func TestClassifyOr_TypedWins(t *testing.T) {
	driverErr := &pgconn.PgError{Code: "23505", Message: "nonstandard wording"}
	got := dberr.ClassifyOr(driverErr, "users", "db.create")

	var ce *api.ConflictError
	require.True(t, errors.As(got, &ce), "want ConflictError, got %v", got)
	assert.Equal(t, "users", ce.Resource)
	assert.True(t, errors.As(got, new(*pgconn.PgError)), "cause must stay recoverable")
}

// An unclassifiable error keeps today's behavior: wrapped with the node's
// context string, so existing messages and %w chains are unchanged.
func TestClassifyOr_FallsThroughWithContext(t *testing.T) {
	base := errors.New("connection reset")
	got := dberr.ClassifyOr(base, "users", "db.update")

	assert.EqualError(t, got, "db.update: connection reset")
	assert.ErrorIs(t, got, base)
}

// Class 42 is an author bug, not a caller fault, so it must fall through
// to the wrapped form and stay a 500.
func TestClassifyOr_Class42FallsThrough(t *testing.T) {
	driverErr := fmt.Errorf("boom: %w", &pgconn.PgError{Code: "42703", Message: "no column"})
	got := dberr.ClassifyOr(driverErr, "users", "db.find")

	var ce *api.ConflictError
	var ve *api.ValidationError
	assert.False(t, errors.As(got, &ce))
	assert.False(t, errors.As(got, &ve))
	assert.Contains(t, got.Error(), "db.find:")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/dberr/ -run TestClassifyOr -v`
Expected: FAIL — `undefined: dberr.ClassifyOr`

- [ ] **Step 3: Add the exported function**

In `internal/dberr/dberr.go`, add `"fmt"` to the import block and append:

```go
// ClassifyOr maps a SQL driver error to a typed api error when its cause
// is caller-facing, and otherwise wraps it with the caller's context
// string exactly as an unclassified error would have been.
//
// resource names the table or entity involved; it reaches clients via
// ConflictError.Resource, so it must not carry schema detail. op is the
// caller's error prefix, e.g. "db.create" or "auth.create_session".
func ClassifyOr(err error, resource, op string) error {
	if typed := Classify(err, resource); typed != nil {
		return typed
	}
	return fmt.Errorf("%s: %w", op, err)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/dberr/ -run TestClassifyOr -v`
Expected: PASS (3 tests)

- [ ] **Step 5: Delete the old helper and repoint `plugins/db`**

```bash
rm plugins/db/errors.go plugins/db/errors_test.go
```

In each of the nine files, replace `classifyOr(` with `dberr.ClassifyOr(` and add the import `"github.com/chimpanze/noda/internal/dberr"`:

```bash
sed -i '' 's/classifyOr(/dberr.ClassifyOr(/g' \
  plugins/db/create.go plugins/db/update.go plugins/db/delete.go \
  plugins/db/find.go plugins/db/find_one.go plugins/db/count.go \
  plugins/db/upsert.go plugins/db/query.go plugins/db/exec.go
goimports -w plugins/db/
```

If `goimports` is unavailable, add the import line by hand to each file — the nine files are the only ones affected.

- [ ] **Step 6: Verify nothing in `plugins/db` changed behaviorally**

Run: `go build ./... && go test ./plugins/db/ ./internal/dberr/`
Expected: PASS. `grep -rn 'classifyOr' plugins/` must return **no matches** for the unexported name.

- [ ] **Step 7: Commit**

```bash
gofmt -l . && git add -A && git commit -m "refactor(dberr): export ClassifyOr so plugins/auth can share it (#418)"
```

---

### Task 2: Classify the five straightforward auth node sites

**Files:**
- Create: `plugins/auth/classify_test.go`
- Modify: `plugins/auth/create_session.go:93-95`, `revoke_session.go:94-96`, `get_user.go:75-80`, `verify_credentials.go:79-81`, `set_password.go:131-133`

**Interfaces:**
- Consumes: `dberr.ClassifyOr` from Task 1.
- Produces: test helpers `injectDriverErr(t *testing.T, db *gorm.DB, kind, sqlstate string)` and `seedUser(t *testing.T, db *gorm.DB) string`, both used again by Task 3.

**Why injection rather than a real SQLite error:** SQLite cannot produce `40001` or `57014` at all, and the auth test fixture runs with **foreign keys OFF** — `newTestDB` (`plugins/auth/schema_test.go:46`) uses the DSN `_pragma=foreign_keys(1)`, which is *modernc* syntax that the *mattn* driver silently ignores (verified: `PRAGMA foreign_keys` returns `0`, and an insert with a dangling `user_id` succeeds). A gorm callback is the only deterministic way to drive these sites through `Classify`. This technique was verified working against all three gorm operation kinds before this plan was written.

- [ ] **Step 1: Write the failing test**

Create `plugins/auth/classify_test.go`:

```go
package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// injectDriverErr makes every gorm operation of the given kind fail with a
// chosen SQLSTATE.
//
// SQLite cannot produce codes like 40001 or 57014, and this package's test
// fixture runs with foreign keys OFF (newTestDB's DSN uses modernc's
// _pragma= syntax, which the mattn driver ignores), so a real classifiable
// error cannot be provoked at most of these call sites. Injection drives
// the site through Classify deterministically instead.
//
// Register the injection AFTER seeding, or the seed will fail too.
func injectDriverErr(t *testing.T, db *gorm.DB, kind, sqlstate string) {
	t.Helper()
	fn := func(tx *gorm.DB) {
		_ = tx.AddError(&pgconn.PgError{Code: sqlstate, Message: "injected"})
	}
	var err error
	switch kind {
	case "query":
		err = db.Callback().Query().Before("gorm:query").Register("test:inject", fn)
	case "create":
		err = db.Callback().Create().Before("gorm:create").Register("test:inject", fn)
	case "update":
		err = db.Callback().Update().Before("gorm:update").Register("test:inject", fn)
	default:
		t.Fatalf("unknown callback kind %q", kind)
	}
	require.NoError(t, err)
}

// seedUser creates one user and returns its id.
func seedUser(t *testing.T, db *gorm.DB) string {
	t.Helper()
	out, data, err := newCreateUserExecutor(nil).Execute(context.Background(), fakeCtx{},
		map[string]any{"email": "seed@example.com", "password": "password123"},
		testServices(db))
	require.NoError(t, err)
	require.Equal(t, api.OutputSuccess, out)
	uid, _ := data.(map[string]any)["id"].(string)
	require.NotEmpty(t, uid)
	return uid
}

// A 40001 reaching any auth node must produce ServiceUnavailableError (503),
// matching what plugins/db has returned since #403 — not a bare 500.
func TestAuthNodesClassifySerializationFailure(t *testing.T) {
	cases := []struct {
		name string
		kind string
		run  func(db *gorm.DB, uid string) error
	}{
		{"get_user", "query", func(db *gorm.DB, uid string) error {
			_, _, err := newGetUserExecutor(nil).Execute(context.Background(), fakeCtx{},
				map[string]any{"user_id": uid}, testServices(db))
			return err
		}},
		{"verify_credentials", "query", func(db *gorm.DB, uid string) error {
			_, _, err := newVerifyCredentialsExecutor(nil).Execute(context.Background(), fakeCtx{},
				map[string]any{"email": "seed@example.com", "password": "password123"},
				testServices(db))
			return err
		}},
		{"create_session", "create", func(db *gorm.DB, uid string) error {
			_, _, err := newCreateSessionExecutor(nil).Execute(context.Background(), fakeCtx{},
				map[string]any{"user_id": uid}, testServices(db))
			return err
		}},
		{"revoke_session", "update", func(db *gorm.DB, uid string) error {
			_, _, err := newRevokeSessionExecutor(nil).Execute(context.Background(), fakeCtx{},
				map[string]any{"user_id": uid}, testServices(db))
			return err
		}},
		{"set_password", "update", func(db *gorm.DB, uid string) error {
			_, _, err := newSetPasswordExecutor(nil).Execute(context.Background(), fakeCtx{},
				map[string]any{"user_id": uid, "password": "newpassword123"}, testServices(db))
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			uid := seedUser(t, db)
			injectDriverErr(t, db, tc.kind, "40001")

			err := tc.run(db, uid)
			require.Error(t, err)

			var su *api.ServiceUnavailableError
			require.True(t, errors.As(err, &su), "want ServiceUnavailableError, got %v", err)
			require.Equal(t, "database", su.Service)
			require.True(t, errors.As(err, new(*pgconn.PgError)), "cause must stay recoverable")
		})
	}
}

// An unmapped driver code keeps today's behavior exactly: wrapped with the
// node's own context prefix, so existing messages and %w chains survive.
func TestAuthNodesFallThroughUnmapped(t *testing.T) {
	db := newTestDB(t)
	uid := seedUser(t, db)
	injectDriverErr(t, db, "query", "42703") // undefined column: author bug, stays 500

	_, _, err := newGetUserExecutor(nil).Execute(context.Background(), fakeCtx{},
		map[string]any{"user_id": uid}, testServices(db))
	require.Error(t, err)

	var su *api.ServiceUnavailableError
	var ve *api.ValidationError
	require.False(t, errors.As(err, &su))
	require.False(t, errors.As(err, &ve))
	require.Contains(t, err.Error(), "auth.get_user:")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/auth/ -run 'TestAuthNodesClassify|TestAuthNodesFallThrough' -v`
Expected: the five `TestAuthNodesClassifySerializationFailure` subtests FAIL with `want ServiceUnavailableError, got auth.<node>: : injected (SQLSTATE 40001)`. `TestAuthNodesFallThroughUnmapped` already PASSES — it pins behavior that must not change.

- [ ] **Step 3: Apply `ClassifyOr` at the five sites**

Add `"github.com/chimpanze/noda/internal/dberr"` to each file's imports, then:

`plugins/auth/create_session.go:93-95` —
```go
	if err := db.WithContext(ctx).Table("auth_sessions").Create(row).Error; err != nil {
		return "", nil, dberr.ClassifyOr(err, "session", "auth.create_session")
	}
```

`plugins/auth/revoke_session.go:94-96` —
```go
	if res.Error != nil {
		return "", nil, dberr.ClassifyOr(res.Error, "session", "auth.revoke_session")
	}
```

`plugins/auth/get_user.go:75-80` — replace only the wrap **below** the `ErrRecordNotFound` branch; the branch itself is untouched:
```go
	if err := q.Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "not_found", map[string]any{}, nil
		}
		return "", nil, dberr.ClassifyOr(err, "user", "auth.get_user")
	}
```

`plugins/auth/verify_credentials.go:79-81` — again only the wrap below the `ErrRecordNotFound` branch:
```go
	if err != nil {
		return "", nil, dberr.ClassifyOr(err, "user", "auth.verify_credentials")
	}
```

`plugins/auth/set_password.go:131-133` — the outer transaction boundary. `Classify` uses `errors.AsType`, which unwraps, so a driver error wrapped inside the transaction (e.g. by `fmt.Errorf("revoke sessions: %w", …)` at line 125) still classifies here:
```go
	if err != nil {
		return "", nil, dberr.ClassifyOr(err, "user", "auth.set_password")
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./plugins/auth/ -run 'TestAuthNodesClassify|TestAuthNodesFallThrough' -v`
Expected: PASS (5 subtests + 1)

- [ ] **Step 5: Verify no existing auth behavior regressed**

Run: `go test ./plugins/auth/`
Expected: PASS — in particular `TestCreateUser`'s duplicate-→-`"exists"` assertion and every `verify_credentials` `"invalid"` test.

- [ ] **Step 6: Commit**

```bash
gofmt -l . && git add -A && git commit -m "fix(auth): classify SQL driver errors at five node call sites (#418)"
```

---

### Task 3: Classify `one_time_tokens` and `create_user`, and pin the invariants

**Files:**
- Modify: `plugins/auth/one_time_tokens.go:89-91`, `:101-103`, `:214-216`
- Modify: `plugins/auth/create_user.go:117-122`
- Modify: `plugins/auth/classify_test.go` (append)

**Interfaces:**
- Consumes: `injectDriverErr`, `seedUser` from Task 2; `dberr.ClassifyOr` from Task 1.
- Produces: nothing new — this is the last of the node-level work.

- [ ] **Step 1: Write the failing test**

Append to `plugins/auth/classify_test.go`:

```go
// The token nodes classify like every other node.
func TestTokenNodesClassifySerializationFailure(t *testing.T) {
	t.Run("create_token", func(t *testing.T) {
		db := newTestDB(t)
		uid := seedUser(t, db)
		injectDriverErr(t, db, "update", "40001") // hits the invalidate-prior UPDATE

		_, _, err := newCreateTokenExecutor(nil).Execute(context.Background(), fakeCtx{},
			map[string]any{"user_id": uid, "purpose": PurposeResetPassword}, testServices(db))
		require.Error(t, err)

		var su *api.ServiceUnavailableError
		require.True(t, errors.As(err, &su), "want ServiceUnavailableError, got %v", err)
	})

	t.Run("consume_token", func(t *testing.T) {
		db := newTestDB(t)
		uid := seedUser(t, db)

		out, data, err := newCreateTokenExecutor(nil).Execute(context.Background(), fakeCtx{},
			map[string]any{"user_id": uid, "purpose": PurposeResetPassword}, testServices(db))
		require.NoError(t, err)
		require.Equal(t, api.OutputSuccess, out)
		raw, _ := data.(map[string]any)["token"].(string)
		require.NotEmpty(t, raw)

		injectDriverErr(t, db, "update", "40001")

		_, _, err = newConsumeTokenExecutor(nil).Execute(context.Background(), fakeCtx{},
			map[string]any{"token": raw, "purpose": PurposeResetPassword}, testServices(db))
		require.Error(t, err)

		var su *api.ServiceUnavailableError
		require.True(t, errors.As(err, &su), "want ServiceUnavailableError, got %v", err)
	})
}

// INVARIANT 1 (spec): the "exists" edge stays bound to IsUniqueViolation.
//
// A 23505 must still return "exists" with a nil error — that edge is the
// anti-enumeration register flow. A 23503 (foreign key) must NOT: routing it
// to "exists" would report a false "email already registered" for an FK bug.
func TestCreateUserExistsEdgeStaysOnUniqueViolation(t *testing.T) {
	t.Run("23505 still yields exists", func(t *testing.T) {
		db := newTestDB(t)
		injectDriverErr(t, db, "create", "23505")

		out, _, err := newCreateUserExecutor(nil).Execute(context.Background(), fakeCtx{},
			map[string]any{"email": "dup@example.com", "password": "password123"},
			testServices(db))
		require.NoError(t, err, "unique violation must not surface as an error")
		require.Equal(t, "exists", out)
	})

	t.Run("23503 does not yield exists", func(t *testing.T) {
		db := newTestDB(t)
		injectDriverErr(t, db, "create", "23503")

		out, _, err := newCreateUserExecutor(nil).Execute(context.Background(), fakeCtx{},
			map[string]any{"email": "fk@example.com", "password": "password123"},
			testServices(db))
		require.Error(t, err)
		require.NotEqual(t, "exists", out, "an FK violation must never read as 'email taken'")

		var ce *api.ConflictError
		require.True(t, errors.As(err, &ce), "want ConflictError, got %v", err)
		require.Equal(t, "user", ce.Resource)
	})
}

// INVARIANT 2 (spec): verify_credentials' anti-enumeration edges are unchanged.
// An unknown email must still return "invalid" with a nil error — never a
// typed error and never a distinguishable status.
func TestVerifyCredentialsUnknownEmailStillInvalid(t *testing.T) {
	db := newTestDB(t)
	seedUser(t, db)

	out, _, err := newVerifyCredentialsExecutor(nil).Execute(context.Background(), fakeCtx{},
		map[string]any{"email": "nobody@example.com", "password": "password123"},
		testServices(db))
	require.NoError(t, err)
	require.Equal(t, "invalid", out)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/auth/ -run 'TestTokenNodes|TestCreateUserExists|TestVerifyCredentialsUnknown' -v`
Expected: both `TestTokenNodesClassifySerializationFailure` subtests FAIL with `want ServiceUnavailableError, got auth.create_token: …` / `auth.consume_token: …`. The `23503` subtest FAILS on `want ConflictError`. The `23505` and unknown-email subtests already PASS — they pin invariants that must not change.

- [ ] **Step 3: Apply `ClassifyOr` at the four remaining sites**

Add `"github.com/chimpanze/noda/internal/dberr"` to both files' imports.

`plugins/auth/one_time_tokens.go:89-91` —
```go
		Update("consumed_at", now).Error; err != nil {
		return "", nil, dberr.ClassifyOr(err, "token", "auth.create_token")
	}
```

`plugins/auth/one_time_tokens.go:101-103` —
```go
	}).Error; err != nil {
		return "", nil, dberr.ClassifyOr(err, "token", "auth.create_token")
	}
```

`plugins/auth/one_time_tokens.go:214-216` — the outer transaction boundary:
```go
	if err != nil {
		return "", nil, dberr.ClassifyOr(err, "token", "auth.consume_token")
	}
```

`plugins/auth/create_user.go:117-122` — the `IsUniqueViolation` branch above is **unchanged**; only the fallthrough wrap becomes classified:
```go
	if err := db.WithContext(ctx).Table("auth_users").Create(row).Error; err != nil {
		if dberr.IsUniqueViolation(err) {
			return "exists", map[string]any{}, nil
		}
		return "", nil, dberr.ClassifyOr(err, "user", "auth.create_user")
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./plugins/auth/ -run 'TestTokenNodes|TestCreateUserExists|TestVerifyCredentialsUnknown' -v`
Expected: PASS (all subtests)

- [ ] **Step 5: Verify the whole auth package**

Run: `go test ./plugins/auth/`
Expected: PASS

- [ ] **Step 6: Confirm no raw driver error escapes any node site**

Run: `grep -n 'fmt.Errorf("auth\.' plugins/auth/*.go`
Expected: matches only on config/validation/service-lookup errors — every remaining match must be a line whose wrapped value is **not** a gorm `.Error`. Cross-check against the nine rows in the spec's call-site table.

- [ ] **Step 7: Commit**

```bash
gofmt -l . && git add -A && git commit -m "fix(auth): classify token and create_user driver errors (#418)"
```

---

### Task 4: Session middleware honors 503 and 504

**Files:**
- Modify: `plugins/auth/session_auth.go:53-55`
- Modify: `internal/server/session_middleware.go:49-52`
- Modify: `internal/server/session_middleware_test.go` (extend `fakeSessionAuth`, add a test)

**Interfaces:**
- Consumes: `dberr.ClassifyOr` from Task 1.
- Produces: nothing consumed by later tasks.

**Why both halves are required:** `AuthenticateSession` returning a typed error changes nothing on its own — the middleware maps *any* error to 500 today, so classifying inside `session_auth.go` alone would be inert.

- [ ] **Step 1: Write the failing test**

In `internal/server/session_middleware_test.go`, add an `err` field to the existing fake and return it:

```go
// fakeSessionAuth implements api.SessionAuthenticator without a real DB.
type fakeSessionAuth struct {
	validToken string
	err        error // when set, AuthenticateSession fails with this
}

func (f *fakeSessionAuth) AuthenticateSession(_ context.Context, _ any, tok string) (*api.AuthData, error) {
	if f.err != nil {
		return nil, f.err
	}
	if tok == f.validToken {
		return &api.AuthData{
			UserID: "user-1",
			Roles:  []string{"user"},
			Claims: map[string]any{"sub": "user-1", "email": "a@b.c", "session_id": "sess-1", "roles": []string{"user"}},
		}, nil
	}
	return nil, nil
}
```

Then append this test:

```go
// A database outage during session validation is infrastructure, not a
// server bug: it must surface as 503/504 rather than a blanket 500. 409 and
// 422 stay deliberately unmapped — a session lookup is a SELECT on a hashed
// token, so neither is reachable by a caller.
func TestSessionMiddlewareHonorsTypedErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"unavailable", &api.ServiceUnavailableError{Service: "database"}, 503},
		{"timeout", &api.TimeoutError{Operation: "database query"}, 504},
		{"conflict stays 500", &api.ConflictError{Resource: "session"}, 500},
		{"validation stays 500", &api.ValidationError{Message: "nope"}, 500},
		{"unmapped stays 500", errors.New("boom"), 500},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServerWithServices(t, map[string]any{
				"auth": &fakeSessionAuth{validToken: "tok123", err: tc.err},
				"db":   struct{}{},
			})
			h, err := s.buildMiddleware("auth.session")
			require.NoError(t, err)

			app := fiber.New()
			app.Use(h)
			app.Get("/x", func(c fiber.Ctx) error { return c.SendString("ok") })

			req := httptest.NewRequest("GET", "/x", nil)
			req.Header.Set("Authorization", "Bearer tok123")
			resp, err := app.Test(req)
			require.NoError(t, err)
			require.Equal(t, tc.want, resp.StatusCode)

			body := make([]byte, 512)
			n, _ := resp.Body.Read(body)
			require.NotContains(t, string(body[:n]), "database query",
				"middleware must not render Cause detail on this ungated path")
		})
	}
}
```

Add `"errors"` to that file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run TestSessionMiddlewareHonorsTypedErrors -v`
Expected: the `unavailable` and `timeout` subtests FAIL with `expected: 503 / actual: 500` and `expected: 504 / actual: 500`. The three `stays 500` subtests PASS already.

- [ ] **Step 3: Classify inside `AuthenticateSession`**

`plugins/auth/session_auth.go:53-55`, adding the `dberr` import:
```go
	if err != nil {
		return nil, dberr.ClassifyOr(err, "session", "auth: AuthenticateSession")
	}
```

Leave the best-effort `last_used_at` touch at line 59 exactly as it is — invariant 3.

- [ ] **Step 4: Map the typed errors in the middleware**

`internal/server/session_middleware.go:49-52`, adding `"errors"` and `"github.com/chimpanze/noda/pkg/api"` to imports if absent:

```go
		ad, err := authn.AuthenticateSession(c.Context(), db, token)
		if err != nil {
			slog.Error("auth.session: validation error", "error", err)
			// Static bodies only: unlike MapErrorToHTTP this path has no
			// dev-mode gate, so it must never render the error's Cause.
			var suErr *api.ServiceUnavailableError
			var toErr *api.TimeoutError
			switch {
			case errors.As(err, &suErr):
				return fiber.NewError(fiber.StatusServiceUnavailable, "service unavailable")
			case errors.As(err, &toErr):
				return fiber.NewError(fiber.StatusGatewayTimeout, "timeout")
			default:
				return fiber.NewError(fiber.StatusInternalServerError, "internal error")
			}
		}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/server/ -run TestSessionMiddleware -v`
Expected: PASS — the new test's five subtests plus the pre-existing `TestSessionMiddleware` and `TestSessionMiddlewareOrdering`.

- [ ] **Step 6: Commit**

```bash
gofmt -l . && git add -A && git commit -m "fix(server): session middleware honors 503/504 from session auth (#418)"
```

---

### Task 5: Cross-driver integration coverage, CHANGELOG, follow-up

**Files:**
- Create: `plugins/auth/classify_integration_test.go`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: everything from Tasks 1–4.

**Why a real-Postgres test earns its keep:** every unit test above injects a `pgconn.PgError` that never touched a database. This task proves the wiring against a driver that really emits the code — and against real foreign keys, which the SQLite fixture does not enforce.

- [ ] **Step 1: Write the failing test**

Create `plugins/auth/classify_integration_test.go`:

```go
//go:build integration

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// pgAuthDB builds the auth schema on a real Postgres, where foreign keys are
// actually enforced — unlike the SQLite unit fixture.
func pgAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(containers.StartPostgres(t)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec(testSchemaPostgres).Error)
	return db
}

// A real FK violation from a real driver must classify as ConflictError,
// and must NOT be mistaken for the "exists" edge.
func TestCreateSessionRealForeignKeyViolation(t *testing.T) {
	db := pgAuthDB(t)

	_, _, err := newCreateSessionExecutor(nil).Execute(context.Background(), fakeCtx{},
		map[string]any{"user_id": "00000000-0000-0000-0000-000000000000"}, testServices(db))
	require.Error(t, err)

	var ce *api.ConflictError
	require.True(t, errors.As(err, &ce), "want ConflictError, got %v", err)
	require.Equal(t, "session", ce.Resource)
}

// A real unique violation still routes to the "exists" edge, not a 409.
func TestCreateUserRealUniqueViolationStillExists(t *testing.T) {
	db := pgAuthDB(t)
	cfg := map[string]any{"email": "dup@example.com", "password": "password123"}

	out, _, err := newCreateUserExecutor(nil).Execute(context.Background(), fakeCtx{}, cfg, testServices(db))
	require.NoError(t, err)
	require.Equal(t, api.OutputSuccess, out)

	out, _, err = newCreateUserExecutor(nil).Execute(context.Background(), fakeCtx{}, cfg, testServices(db))
	require.NoError(t, err, "unique violation must stay on the exists edge")
	require.Equal(t, "exists", out)
}
```

Define `testSchemaPostgres` in the same file as the `testSchema` from `schema_test.go` with `TIMESTAMP` kept and `TEXT PRIMARY KEY` unchanged — both are valid Postgres. If `schema_test.go`'s `testSchema` applies cleanly to Postgres as written, reuse it directly and delete the duplicate rather than maintaining two copies.

- [ ] **Step 2: Run test to verify it fails, then passes**

Run: `go test -tags integration ./plugins/auth/ -run 'TestCreateSessionRealForeignKey|TestCreateUserRealUnique' -v`
Expected: PASS once Tasks 2–3 are in. If `TestCreateSessionRealForeignKeyViolation` fails with a plain wrapped error, `create_session`'s `ClassifyOr` call is missing or mis-scoped.

Requires Docker. If unavailable locally, note it and let CI's `Integration e2e (external services)` job run it.

- [ ] **Step 3: Add the CHANGELOG entries**

Under `## [Unreleased]` → `### Changed`, add:

```markdown
- `plugins/auth`'s own SQL failures are now classified like `plugins/db`'s (#418). Serialization failures and deadlocks become 503, statement timeouts 504, and data exceptions 422, where every auth node previously returned a blanket 500. `internal/dberr.ClassifyOr` is now exported and shared by both plugins. **Behavior change:** a caller treating any 5xx from an auth route as retryable will see different statuses. Node output edges are unchanged — `create_user` still returns `"exists"` for a unique violation (it branches on `IsUniqueViolation`, not `Classify`, so a foreign-key violation cannot be misreported as "email already registered"), and `verify_credentials` still returns `"invalid"` for an unknown email with unchanged timing.
- Session-authenticated routes now return **503** (or **504** on a statement timeout) when the database is unavailable during session validation, instead of 500 (#418). `internal/server/session_middleware.go` previously mapped every `AuthenticateSession` failure to a blanket 500. Conflict and validation errors deliberately stay 500 on this path: a session lookup is a `SELECT` on a hashed token, so neither is reachable by a caller. Response bodies remain static and never render driver detail — this path has no dev-mode gate.
```

- [ ] **Step 4: Full verification**

```bash
gofmt -l .
go build ./...
go vet ./...
go test ./...
```
Expected: `gofmt -l .` prints nothing; build and vet clean; **0 test failures**.

- [ ] **Step 5: Commit**

```bash
git add -A && git commit -m "test(auth): cross-driver coverage for auth error classification (#418)"
```

- [ ] **Step 6: File the fixture follow-up**

The SQLite unit fixture does not enforce foreign keys: `newTestDB` (`plugins/auth/schema_test.go:46`) passes `_pragma=foreign_keys(1)`, which is *modernc* DSN syntax that the *mattn* driver ignores. Verified: `PRAGMA foreign_keys` returns `0` and an insert with a dangling `user_id` succeeds silently. This is pre-existing, unrelated to #418, and out of scope here — any existing test that believes it covers FK cascade or rejection behavior is not actually testing it.

```bash
gh issue create --title "plugins/auth test fixture does not enforce foreign keys" \
  --body "newTestDB (plugins/auth/schema_test.go:46) uses the DSN parameter \`_pragma=foreign_keys(1)\`. That is modernc/glebarez syntax; this project's \`gorm.io/driver/sqlite\` wraps mattn/go-sqlite3, which ignores it and expects \`_foreign_keys=1\`.

Verified: \`PRAGMA foreign_keys\` returns 0 in the fixture, and inserting an auth_sessions row with a dangling user_id succeeds with no error.

Consequence: FK constraints in \`testSchema\` are inert, so any unit test relying on FK rejection or ON DELETE CASCADE is passing vacuously. Found while adding auth error classification (#418); pre-existing and unrelated to that change.

Fixing it means switching the DSN parameter and then checking which existing tests were quietly depending on FKs being off."
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| §1 Shared `ClassifyOr` (export, delete private copy, move tests) | 1 |
| §2 Call sites — 5 straightforward | 2 |
| §2 Call sites — tokens ×3, `create_user` | 3 |
| §2 `resource` naming (logical, not table names) | Global Constraints; asserted in 2, 3, 5 |
| §3 `session_auth.go` + middleware 503/504, static bodies, 409/422 unmapped | 4 |
| Invariant 1 (`exists` on `IsUniqueViolation`) | 3 Step 1, 5 Step 1 |
| Invariant 2 (`verify_credentials` `"invalid"` edges) | 3 Step 1; regression check 2 Step 5 |
| Invariant 3 (best-effort writes stay ignored) | 4 Step 3; no task touches line 59 or 106 |
| Out of scope (`"user not found"` → `NotFoundError`) | Not implemented, by design |
| Behavior changes → CHANGELOG | 5 Step 3 |
| Verification (unit, invariants, middleware, cross-driver, `plugins/db` unchanged) | 1 Step 6, 2, 3, 4, 5 |

**Type consistency:** `ClassifyOr(err error, resource, op string) error` is defined once in Task 1 and called with that exact signature in Tasks 2, 3, 4. Executor constructors (`newCreateUserExecutor`, `newGetUserExecutor`, `newCreateSessionExecutor`, `newRevokeSessionExecutor`, `newSetPasswordExecutor`, `newVerifyCredentialsExecutor`, `newCreateTokenExecutor`, `newConsumeTokenExecutor`) all take `map[string]any` and return `api.NodeExecutor`. Helpers `injectDriverErr` / `seedUser` are defined in Task 2 and reused in Task 3 with identical signatures.

All eight executor constructors named in this plan were verified against source: `newCreateUserExecutor` (`create_user.go:50`), `newGetUserExecutor` (`get_user.go:47`), `newCreateSessionExecutor` (`create_session.go:45`), `newRevokeSessionExecutor` (`revoke_session.go:48`), `newSetPasswordExecutor` (`set_password.go:49`), `newVerifyCredentialsExecutor` (`verify_credentials.go:46`), `newCreateTokenExecutor` (`one_time_tokens.go:50`), `newConsumeTokenExecutor` (`one_time_tokens.go:163`). The `PurposeVerifyEmail` / `PurposeResetPassword` constants used in Task 3 are exported from `one_time_tokens.go`.

**Placeholder scan:** clean — no TBD/TODO, and every code step carries complete code.
