# `plugins/auth` SQL Error Classification — Design

**Issue:** #418 (item 3 — part 2 of 2)
**Follows:** #421 (`a9c23d4`, items 1–2: the three unclassified SQLite result codes)
**Depends on:** `internal/dberr` as established by #403 (`360b4e6`)

## Problem

#403 wired `dberr` into all 9 `plugins/db` node call sites and into `auth.create_user`'s
unique check. Every other gorm call in `plugins/auth` still returns a raw wrapped error.

Consequence: an identical `40001` serialization failure returns **503 from `db.find`** but
**500 from `auth.verify_credentials`**. The codebase is half-converted — the same driver
condition means different things depending on which node observed it.

A second, larger gap sits behind the middleware. `Service.AuthenticateSession`
(`plugins/auth/session_auth.go:53`) returns a raw error, and
`internal/server/session_middleware.go:49-52` maps **any** error from it to a hardcoded
`500 "internal error"`. A database outage during session validation is therefore reported
as a server bug on every authenticated route, and classifying inside `session_auth.go`
alone would have no observable effect.

## Design

### 1. Shared `ClassifyOr`

`plugins/db/errors.go` holds an unexported `classifyOr` that `plugins/auth` cannot reach.
Export it from `internal/dberr` instead and delete the private copy:

```go
// ClassifyOr maps a SQL driver error to a typed api error when its cause is
// caller-facing, and otherwise wraps it with the caller's context string.
//
// resource names the entity involved; it reaches clients via
// ConflictError.Resource, so it must not carry schema detail.
// op is the caller's error prefix, e.g. "db.create" or "auth.create_session".
func ClassifyOr(err error, resource, op string) error {
	if typed := Classify(err, resource); typed != nil {
		return typed
	}
	return fmt.Errorf("%s: %w", op, err)
}
```

`plugins/db`'s 9 call sites become `dberr.ClassifyOr(...)`; `plugins/db/errors.go` is
deleted and `plugins/db/errors_test.go` moves to `internal/dberr`. **No behavior change in
`plugins/db`** — a pure move, and the moved tests are the proof.

`Classify` resolves driver errors with `errors.AsType` (`internal/dberr/postgres.go:49`,
`sqlite.go:58`), which unwraps. A driver error wrapped by an inner
`fmt.Errorf("revoke sessions: %w", …)` therefore still classifies at the outer boundary.
This is what lets the two transaction sites be handled with one call each instead of
threading classification through every inner return.

### 2. Call sites (9 sites across 8 files)

| File:line | Error source | `resource` | `op` |
|---|---|---|---|
| `create_session.go:93` | `Create().Error` | `session` | `auth.create_session` |
| `revoke_session.go:94` | `res.Error` | `session` | `auth.revoke_session` |
| `get_user.go:75` | `Take().Error` | `user` | `auth.get_user` |
| `verify_credentials.go:79` | `Take().Error` | `user` | `auth.verify_credentials` |
| `set_password.go:131` | transaction result | `user` | `auth.set_password` |
| `one_time_tokens.go:89` | invalidate-prior `Update().Error` | `token` | `auth.create_token` |
| `one_time_tokens.go:101` | `Create().Error` | `token` | `auth.create_token` |
| `one_time_tokens.go:214` | transaction result | `token` | `auth.consume_token` |
| `create_user.go:120` | fallthrough wrap below the `IsUniqueViolation` branch | `user` | `auth.create_user` |

At `get_user.go:75` and `verify_credentials.go:79` the `ClassifyOr` call replaces only the
wrap **below** the existing `errors.Is(err, gorm.ErrRecordNotFound)` branch. Ordering is
preserved deliberately: `ErrRecordNotFound` is not a driver error and `Classify` returns
nil for it, so the branches are not interchangeable in meaning even though they would
behave identically.

`resource` uses logical names (`user`, `session`, `token`), not table names.
`MapErrorToHTTP` renders `ConflictError` to production clients as `conflict on %s`
(`internal/server/errors.go:68`), so a table name here would leak schema.

### 3. Session middleware honors 503 and 504

`session_auth.go:53` gains `dberr.ClassifyOr(err, "session", "auth: AuthenticateSession")`,
and `internal/server/session_middleware.go:49-52` becomes:

```go
if err != nil {
	slog.Error("auth.session: validation error", "error", err)
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

Two deliberate restrictions:

- **Static bodies, no `Cause` rendering.** This path has no dev-mode gate, unlike
  `MapErrorToHTTP`. Reusing that mapper wholesale would pull dev-mode message enrichment
  into the middleware and render driver detail on an unauthenticated-facing route.
- **409 and 422 stay unmapped.** Session lookup is a `SELECT` parameterized on a hashed
  token; a conflict or data-exception is not reachable by a caller. Mapping them would add
  a status an attacker could probe for with no legitimate trigger.

## Invariants this must not break

These are load-bearing security behaviors from the anti-enumeration tranche, not
incidental structure.

1. **`create_user`'s `"exists"` output stays on `IsUniqueViolation`, never `Classify`.**
   `Classify` also matches foreign-key violations, so switching would convert an FK bug
   into a false "email already registered" *and* turn the anti-enumeration `"exists"`
   branch into a 409.
2. **`verify_credentials`' `"invalid"` branches are untouched.** Unknown email, unusable
   `password_hash`, wrong password, and inactive status keep returning `"invalid"` with
   flat timing. Only the infra-error path at line 79 is classified, and that path does not
   depend on account existence, so it introduces no enumeration oracle.
3. **Best-effort writes stay ignored.** The `last_used_at` touch
   (`session_auth.go:59`) and the rehash upgrade (`verify_credentials.go:106`) discard
   their errors by design. Classifying them would turn a cosmetic write failure into a
   failed login.

Out of scope: `set_password.go:118`'s `fmt.Errorf("user not found")` is an application
condition, not a driver error. Promoting it to `api.NotFoundError` is a separate behavior
change and is not part of this work.

## Behavior changes

- **Auth node failures on caller-triggerable SQL conditions move off 500.** Serialization
  failures and deadlocks become 503, statement timeouts 504, data exceptions 422, matching
  what `plugins/db` has returned since #403. A caller treating any 5xx from an auth route
  as retryable will see different statuses.
- **Session-authenticated routes return 503/504 on a database outage** instead of 500.
  This affects every route behind the session middleware and is the widest-reaching part
  of this change.
- No node output changes. No `"success"`/`"invalid"`/`"exists"` edge behavior changes.

## Verification

- Unit tests per call site: a `40001` and a `57014` produce the typed error; an unmapped
  code produces the unchanged `op: %w` wrap.
- Regression tests pinning the three invariants — specifically that an FK violation on
  `create_user` does **not** produce `"exists"`, and that `verify_credentials` still
  returns `"invalid"` (not 422 or 500) for an unknown email.
- Middleware tests: `ServiceUnavailableError` → 503, `TimeoutError` → 504, unmapped → 500,
  and that no response body carries driver text.
- Cross-driver integration coverage against real Postgres and SQLite, following
  `plugins/db/classify_integration_test.go` as established in part 1.
- `plugins/db` behavior is unchanged: the relocated `errors_test.go` must pass untouched.
