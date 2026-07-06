# Auth Anti-Enumeration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close account-enumeration in the scaffolded auth templates: make register indistinguishable (verification-first) and flatten reset/resend response timing to a fixed deadline.

**Architecture:** All fixes are in `cmd/noda/auth_templates/` (scaffolded workflow/route/test config), enabled by one small runtime change: `util.delay` resolving its `timeout` per request so a computed deadline can be expressed in config.

**Tech Stack:** Go, `text/template` (`[[ ]]` delims) scaffolding, Noda workflow engine + expression compiler, `internal/testing` workflow-test runner.

## Global Constraints

- No new service dependency: the scaffold stays **DB + email only** (no Redis/worker). The async-email worker is documented as the high-security upgrade, not implemented.
- Deadline constant **T = 500** (milliseconds), inlined in the pad expressions.
- **No multi-inbound (join) nodes** in the rewritten workflows. The engine's `computeJoinTypes` can misclassify a merge whose inbound includes the conditional node itself as an AND-join, which deadlocks. Keep every node single-inbound: separate, byte-identical `respond` nodes per branch, each preceded by its own `now_ts → pad`. This is a deliberate refinement of the spec's "converge to one respond" for engine-safety; the security outcome (indistinguishable responses + equal deadline) is unchanged.
- Register: both branches send exactly one email, return identical `200 {"message":"Check your email to continue"}`, and set **no** session cookie (verification-first; no auto-login).
- Scaffolded `.tmpl` files use `{{ }}` for Noda runtime expressions and `[[ ]]` for scaffold substitution (`[[.DBService]]`, `[[.EmailService]]`).

---

### Task 1: `util.delay` resolves its `timeout` per request

**Files:**
- Modify: `plugins/core/util/delay.go`
- Test: `plugins/core/util/delay_test.go`

**Interfaces:**
- Consumes: `plugin.ResolveString(nCtx api.ExecutionContext, config map[string]any, key string) (string, error)` from `github.com/chimpanze/noda/internal/plugin`.
- Produces: `util.delay` now accepts a `timeout` that is a Noda expression resolving to a duration string (e.g. `"{{ 10 + 5 }}ms"` → `"15ms"`), in addition to a static `"500ms"`.

- [ ] **Step 1: Write the failing test**

Add to `plugins/core/util/delay_test.go`:

```go
func TestDelay_ResolvesTemplatedTimeout(t *testing.T) {
	// A computed expression must resolve per-request to a duration string.
	config := map[string]any{"timeout": "{{ 10 + 5 }}ms"}
	executor := newDelayExecutor(config)
	execCtx := engine.NewExecutionContext()

	start := time.Now()
	output, data, err := executor.Execute(context.Background(), execCtx, config, nil)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Nil(t, data)
	assert.GreaterOrEqual(t, elapsed, 12*time.Millisecond)
	assert.Less(t, elapsed, 200*time.Millisecond)
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./plugins/core/util/ -run TestDelay_ResolvesTemplatedTimeout -v`
Expected: FAIL — the current build-time parse of `"{{ 10 + 5 }}ms"` fails, so `Execute` returns an "invalid duration" error.

- [ ] **Step 3: Rewrite `delay.go` to resolve at execute time**

Replace the executor and its constructor/`Execute` in `plugins/core/util/delay.go` (keep the descriptor, `ConfigSchema`, `OutputDescriptions`, and the `plugin.RegisterEntry` in `plugin.go` unchanged). Add the import.

```go
package util

import (
	"context"
	"fmt"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

// ... descriptor unchanged ...

type delayExecutor struct{}

func newDelayExecutor(_ map[string]any) api.NodeExecutor { return &delayExecutor{} }

func (e *delayExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *delayExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	timeoutStr, err := plugin.ResolveString(nCtx, config, "timeout")
	if err != nil {
		return "", nil, fmt.Errorf("util.delay: %w", err)
	}
	d, err := time.ParseDuration(timeoutStr)
	if err != nil {
		return "", nil, fmt.Errorf("util.delay: invalid duration %q", timeoutStr)
	}

	select {
	case <-time.After(d):
		return api.OutputSuccess, nil, nil
	case <-ctx.Done():
		return "", nil, &api.TimeoutError{Duration: d, Operation: "util.delay"}
	}
}
```

- [ ] **Step 4: Fix the one pre-existing test whose error message changed**

`TestDelay_MissingTimeout` asserts the error contains `"invalid duration"`. With per-request resolution, a missing `timeout` now fails in `ResolveString` first. Update that assertion:

```go
func TestDelay_MissingTimeout(t *testing.T) {
	config := map[string]any{}
	executor := newDelayExecutor(config)
	execCtx := engine.NewExecutionContext()

	_, _, err := executor.Execute(context.Background(), execCtx, config, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}
```

Leave `TestDelay_WaitsCorrectDuration`, `TestDelay_ContextCancellation`, `TestDelay_DurationParsing`, `TestDelay_InvalidDuration`, `TestDelay_ZeroDuration`, `TestDelay_Descriptor` as-is — a static `"100ms"` resolves to itself and still passes.

- [ ] **Step 5: Run the delay suite**

Run: `go test -race ./plugins/core/util/ -run TestDelay -v`
Expected: PASS (all delay tests, including the new templated one).

- [ ] **Step 6: Commit**

```bash
git add plugins/core/util/delay.go plugins/core/util/delay_test.go
git commit -m "feat(util): resolve util.delay timeout per request (enables computed delays)"
```

---

### Task 2: Register — verification-first, indistinguishable (auth-1)

**Files:**
- Modify: `cmd/noda/auth_templates/workflows/auth.register.json.tmpl`
- Modify: `cmd/noda/auth_templates/tests/test-auth-register.json`
- Modify: `cmd/noda/auth_init_test.go` (add a runner-based behavior test — reused by Tasks 3–4)
- Check: `cmd/noda/auth_templates/routes/auth.register.json` (must not assert 201/cookie)

**Interfaces:**
- Consumes (from Task 1): `util.delay` per-request timeout (not used in register, but the shared runner harness added here is reused by Tasks 3–4).
- Produces: `runScaffoldedAuthSuite(t, dir, suiteFile)` helper other tasks call.

- [ ] **Step 1: Update the scaffolded register test to the new behavior (failing)**

Replace `cmd/noda/auth_templates/tests/test-auth-register.json` with:

```json
{
  "id": "test-auth-register",
  "workflow": "auth-register",
  "tests": [
    {
      "name": "new email: verify email sent, generic 200, no cookie",
      "input": { "email": "alice@example.com", "password": "password123" },
      "mocks": {
        "create_user": { "output": { "id": "user-1", "email": "alice@example.com", "roles": ["user"] } },
        "verify_token": { "output": { "token": "verify-tok", "expires_at": "2030-01-01T00:00:00Z" } },
        "send_verify_email": { "output": { "sent": true } },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    },
    {
      "name": "existing email: identical generic 200 via exists branch",
      "input": { "email": "alice@example.com", "password": "password123" },
      "mocks": {
        "create_user": { "output_name": "exists", "output": {} },
        "send_exists_email": { "output": { "sent": true } },
        "respond_exists": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_exists.status": 200 } }
    }
  ]
}
```

- [ ] **Step 2: Add the runner harness + register behavior test (failing)**

Add to `cmd/noda/auth_init_test.go`. `buildCoreNodeRegistry()` (same `main` package, main.go:788), `nodatesting.LoadTests`, and `nodatesting.RunTestSuite` are the pieces; `loadResolvedConfigForTest(dir)` (already in this file) renders+validates+compiles. Import `nodatesting "github.com/chimpanze/noda/internal/testing"` if not already imported.

```go
// runScaffoldedAuthSuite scaffolds auth into a temp project, then runs one
// scaffolded test suite through the real workflow-test runner and asserts every
// case passes. This exercises the rendered templates end-to-end.
func runScaffoldedAuthSuite(t *testing.T, suiteID string) {
	t.Helper()
	dir := scaffoldAuthProject(t) // existing helper used by TestAuthInitScaffold; if absent, mirror its setup
	rc, err := loadResolvedConfigForTest(dir)
	require.NoError(t, err)

	suites, err := nodatesting.LoadTests(rc)
	require.NoError(t, err)

	reg, err := buildCoreNodeRegistry()
	require.NoError(t, err)

	var ran bool
	for _, suite := range suites {
		if suite.ID != suiteID {
			continue
		}
		ran = true
		for _, res := range nodatesting.RunTestSuite(suite, rc, reg) {
			assert.Truef(t, res.Passed, "case %q failed: %s", res.CaseName, res.Error)
		}
	}
	require.Truef(t, ran, "suite %q not found among scaffolded tests", suiteID)
}

func TestAuthScaffold_RegisterIsAntiEnumerating(t *testing.T) {
	runScaffoldedAuthSuite(t, "test-auth-register")
}
```

Note: if `TestSuite` exposes the id under a different field than `.ID` (check `internal/testing/types.go` / `loader.go`), match on that. If no `scaffoldAuthProject` helper exists, extract the temp-dir scaffold setup from `TestAuthInitScaffold` into one.

- [ ] **Step 3: Run it and watch it fail**

Run: `go test ./cmd/noda/ -run TestAuthScaffold_RegisterIsAntiEnumerating -v`
Expected: FAIL — the current template still has `respond` at 201 with a cookie and a `respond_exists` at 400, and no `send_exists_email`, so the mocks/expectations don't line up.

- [ ] **Step 4: Rewrite the register template**

Replace `cmd/noda/auth_templates/workflows/auth.register.json.tmpl` with:

```json
{
  "id": "auth-register",
  "name": "Auth: Register",
  "nodes": {
    "create_user": {
      "type": "auth.create_user",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "email": "{{ input.email }}", "password": "{{ input.password }}" }
    },
    "verify_token": {
      "type": "auth.create_token",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "user_id": "{{ nodes.create_user.id }}", "purpose": "verify_email" }
    },
    "send_verify_email": {
      "type": "email.send",
      "services": { "mailer": "[[.EmailService]]" },
      "config": {
        "to": "{{ nodes.create_user.email }}",
        "subject": "Verify your email",
        "body": "<p>Welcome! Verify your email with this token: <strong>{{ nodes.verify_token.token }}</strong></p>"
      }
    },
    "send_exists_email": {
      "type": "email.send",
      "services": { "mailer": "[[.EmailService]]" },
      "config": {
        "to": "{{ input.email }}",
        "subject": "Account already registered",
        "body": "<p>Someone tried to register an account with this email, but one already exists. If this was you, try logging in or resetting your password.</p>"
      }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 200, "body": { "message": "Check your email to continue" } }
    },
    "respond_exists": {
      "type": "response.json",
      "config": { "status": 200, "body": { "message": "Check your email to continue" } }
    }
  },
  "edges": [
    { "from": "create_user", "to": "verify_token" },
    { "from": "create_user", "output": "exists", "to": "send_exists_email" },
    { "from": "verify_token", "to": "send_verify_email" },
    { "from": "send_verify_email", "to": "respond" },
    { "from": "send_exists_email", "to": "respond_exists" }
  ]
}
```

Both `respond` and `respond_exists` are byte-identical (`200`, same body, no `cookies`) — indistinguishable to a client. No session node, no cookie.

- [ ] **Step 5: Verify the route doesn't leak the old shape**

Read `cmd/noda/auth_templates/routes/auth.register.json`; if it hardcodes a `201` status or a cookie expectation, align it to the workflow (the response is produced by the workflow's response node, so the route typically needs no change). Leave it otherwise untouched.

- [ ] **Step 6: Run the register behavior test + scaffold validation**

Run: `go test ./cmd/noda/ -run 'TestAuthScaffold_RegisterIsAntiEnumerating|TestAuthInitScaffold|TestAuthInitOutputValidates' -v`
Expected: PASS — the rendered register workflow compiles/validates and both branches reach an identical 200 response.

- [ ] **Step 7: Commit**

```bash
git add cmd/noda/auth_templates/workflows/auth.register.json.tmpl cmd/noda/auth_templates/tests/test-auth-register.json cmd/noda/auth_init_test.go
git commit -m "fix(auth-scaffold): verification-first register removes account enumeration (auth-1)"
```

---

### Task 3: Request-password-reset — constant-time deadline (auth-2)

**Files:**
- Modify: `cmd/noda/auth_templates/workflows/auth.request-password-reset.json.tmpl`
- Modify: `cmd/noda/auth_templates/tests/test-auth-request-password-reset.json`
- Modify: `cmd/noda/auth_init_test.go` (add one behavior test using the Task 2 harness)

**Interfaces:**
- Consumes: `runScaffoldedAuthSuite` (Task 2); `util.delay` per-request timeout (Task 1); `util.timestamp` `unix_ms`.

- [ ] **Step 1: Update the scaffolded test (failing)**

Replace `cmd/noda/auth_templates/tests/test-auth-request-password-reset.json`. Mock the timing nodes so the suite is fast and deterministic (no real delay), and assert each branch's own `respond_*` node at 200:

```json
{
  "id": "test-auth-request-password-reset",
  "workflow": "auth-request-password-reset",
  "tests": [
    {
      "name": "known email: reset email sent, generic 200",
      "input": { "email": "alice@example.com" },
      "mocks": {
        "start_ts": { "output": 1000 },
        "find_user": { "output": { "id": "user-1", "email": "alice@example.com" } },
        "reset_token": { "output": { "token": "reset-tok", "expires_at": "2030-01-01T00:00:00Z" } },
        "send_reset_email": { "output": { "sent": true } },
        "now_ts_sent": { "output": 1100 },
        "pad_sent": { "output": null },
        "respond_sent": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_sent.status": 200 } }
    },
    {
      "name": "unknown email: identical generic 200",
      "input": { "email": "nobody@example.com" },
      "mocks": {
        "start_ts": { "output": 1000 },
        "find_user": { "output_name": "not_found", "output": {} },
        "now_ts_unknown": { "output": 1001 },
        "pad_unknown": { "output": null },
        "respond_unknown": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_unknown.status": 200 } }
    }
  ]
}
```

Add the behavior test to `cmd/noda/auth_init_test.go`:

```go
func TestAuthScaffold_RequestPasswordResetIsConstantTime(t *testing.T) {
	runScaffoldedAuthSuite(t, "test-auth-request-password-reset")
}
```

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./cmd/noda/ -run TestAuthScaffold_RequestPasswordResetIsConstantTime -v`
Expected: FAIL — the current template has no `start_ts`/`now_ts_*`/`pad_*` nodes; mock ids don't resolve.

- [ ] **Step 3: Rewrite the reset template (no joins)**

Replace `cmd/noda/auth_templates/workflows/auth.request-password-reset.json.tmpl`:

```json
{
  "id": "auth-request-password-reset",
  "name": "Auth: Request Password Reset",
  "nodes": {
    "start_ts": { "type": "util.timestamp", "config": { "format": "unix_ms" } },
    "find_user": {
      "type": "auth.get_user",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "email": "{{ input.email }}" }
    },
    "reset_token": {
      "type": "auth.create_token",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "user_id": "{{ nodes.find_user.id }}", "purpose": "reset_password" }
    },
    "send_reset_email": {
      "type": "email.send",
      "services": { "mailer": "[[.EmailService]]" },
      "config": {
        "to": "{{ nodes.find_user.email }}",
        "subject": "Reset your password",
        "body": "<p>Reset your password with this token: <strong>{{ nodes.reset_token.token }}</strong></p><p>If you did not request this, ignore this email.</p>"
      }
    },
    "now_ts_sent": { "type": "util.timestamp", "config": { "format": "unix_ms" } },
    "pad_sent": { "type": "util.delay", "config": { "timeout": "{{ (nodes.start_ts + 500) > nodes.now_ts_sent ? (nodes.start_ts + 500 - nodes.now_ts_sent) : 0 }}ms" } },
    "respond_sent": { "type": "response.json", "config": { "status": 200, "body": { "message": "If that account exists, an email was sent" } } },
    "now_ts_unknown": { "type": "util.timestamp", "config": { "format": "unix_ms" } },
    "pad_unknown": { "type": "util.delay", "config": { "timeout": "{{ (nodes.start_ts + 500) > nodes.now_ts_unknown ? (nodes.start_ts + 500 - nodes.now_ts_unknown) : 0 }}ms" } },
    "respond_unknown": { "type": "response.json", "config": { "status": 200, "body": { "message": "If that account exists, an email was sent" } } }
  },
  "edges": [
    { "from": "start_ts", "to": "find_user" },
    { "from": "find_user", "to": "reset_token" },
    { "from": "find_user", "output": "not_found", "to": "now_ts_unknown" },
    { "from": "reset_token", "to": "send_reset_email" },
    { "from": "send_reset_email", "to": "now_ts_sent" },
    { "from": "now_ts_sent", "to": "pad_sent" },
    { "from": "pad_sent", "to": "respond_sent" },
    { "from": "now_ts_unknown", "to": "pad_unknown" },
    { "from": "pad_unknown", "to": "respond_unknown" }
  ]
}
```

**Before finishing, confirm the scalar reference form.** `util.timestamp` returns its `unix_ms` value as the node's output directly. Verify `nodes.start_ts` / `nodes.now_ts_sent` resolve to the integer (not a wrapped field) by running the suite unmocked once in a scratch check, or by inspecting how other templates read a scalar node output. If the engine exposes it under a field (e.g. `nodes.start_ts.value`), update every `pad_*` expression accordingly. The mocked suite passes regardless (pad is mocked), so this must be checked against a real run — do the unmocked scratch run in Step 4.

- [ ] **Step 4: Prove the pad expression actually resolves (unmocked scratch check)**

In the same test file, add a temporary/local assertion (or a `t.Log`-verified subtest) that runs the reset workflow's **unknown** branch without mocking `start_ts`/`now_ts_unknown`/`pad_unknown` and asserts it completes `status: "success"` reaching `respond_unknown` — this exercises the real timestamp + computed-delay + ParseDuration path. Keep it to the unknown branch (cheap: elapsed ≈ 0, so pad ≈ 500ms; if 500ms is too slow for CI taste, this scratch assertion may use a shorter deadline copy of the workflow, but the shipped template keeps T=500). If it errors on the expression, fix the scalar reference form (Step 3 note) until it passes, then keep whichever assertion form is fast and deterministic.

- [ ] **Step 5: Run reset behavior + validation**

Run: `go test ./cmd/noda/ -run 'TestAuthScaffold_RequestPasswordResetIsConstantTime|TestAuthInitOutputValidates' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/noda/auth_templates/workflows/auth.request-password-reset.json.tmpl cmd/noda/auth_templates/tests/test-auth-request-password-reset.json cmd/noda/auth_init_test.go
git commit -m "fix(auth-scaffold): constant-time password-reset response (auth-2)"
```

---

### Task 4: Resend-verification — constant-time deadline (auth-2)

**Files:**
- Modify: `cmd/noda/auth_templates/workflows/auth.resend-verification.json.tmpl`
- Modify: `cmd/noda/auth_templates/tests/test-auth-resend-verification.json`
- Modify: `cmd/noda/auth_init_test.go` (add one behavior test)

**Interfaces:**
- Consumes: `runScaffoldedAuthSuite` (Task 2); `util.delay`/`util.timestamp`; the confirmed scalar reference form from Task 3.

- [ ] **Step 1: Update the scaffolded test (failing)**

Replace `cmd/noda/auth_templates/tests/test-auth-resend-verification.json` with three cases (unknown, already-verified, unverified-sent), mocking the timing nodes:

```json
{
  "id": "test-auth-resend-verification",
  "workflow": "auth-resend-verification",
  "tests": [
    {
      "name": "unknown email: generic 200",
      "input": { "email": "nobody@example.com" },
      "mocks": {
        "start_ts": { "output": 1000 },
        "find_user": { "output_name": "not_found", "output": {} },
        "now_ts_unknown": { "output": 1001 },
        "pad_unknown": { "output": null },
        "respond_unknown": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_unknown.status": 200 } }
    },
    {
      "name": "already verified: identical generic 200",
      "input": { "email": "alice@example.com" },
      "mocks": {
        "start_ts": { "output": 1000 },
        "find_user": { "output": { "id": "user-1", "email": "alice@example.com", "email_verified_at": "2020-01-01T00:00:00Z" } },
        "check_unverified": { "output_name": "else", "output": {} },
        "now_ts_verified": { "output": 1002 },
        "pad_verified": { "output": null },
        "respond_verified": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_verified.status": 200 } }
    },
    {
      "name": "unverified: verify email sent, generic 200",
      "input": { "email": "bob@example.com" },
      "mocks": {
        "start_ts": { "output": 1000 },
        "find_user": { "output": { "id": "user-2", "email": "bob@example.com", "email_verified_at": null } },
        "check_unverified": { "output_name": "then", "output": {} },
        "verify_token": { "output": { "token": "verify-tok", "expires_at": "2030-01-01T00:00:00Z" } },
        "send_verify_email": { "output": { "sent": true } },
        "now_ts_sent": { "output": 1100 },
        "pad_sent": { "output": null },
        "respond_sent": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_sent.status": 200 } }
    }
  ]
}
```

Add: `func TestAuthScaffold_ResendVerificationIsConstantTime(t *testing.T) { runScaffoldedAuthSuite(t, "test-auth-resend-verification") }`

- [ ] **Step 2: Run it and watch it fail**

Run: `go test ./cmd/noda/ -run TestAuthScaffold_ResendVerificationIsConstantTime -v`
Expected: FAIL — new node ids absent from the current template.

- [ ] **Step 3: Rewrite the resend template (no joins, three padded branches)**

Replace `cmd/noda/auth_templates/workflows/auth.resend-verification.json.tmpl`:

```json
{
  "id": "auth-resend-verification",
  "name": "Auth: Resend Verification Email",
  "nodes": {
    "start_ts": { "type": "util.timestamp", "config": { "format": "unix_ms" } },
    "find_user": {
      "type": "auth.get_user",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "email": "{{ input.email }}" }
    },
    "check_unverified": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.find_user.email_verified_at == nil }}" }
    },
    "verify_token": {
      "type": "auth.create_token",
      "services": { "auth": "auth", "database": "[[.DBService]]" },
      "config": { "user_id": "{{ nodes.find_user.id }}", "purpose": "verify_email" }
    },
    "send_verify_email": {
      "type": "email.send",
      "services": { "mailer": "[[.EmailService]]" },
      "config": {
        "to": "{{ nodes.find_user.email }}",
        "subject": "Verify your email",
        "body": "<p>Verify your email with this token: <strong>{{ nodes.verify_token.token }}</strong></p><p>If you did not request this, ignore this email.</p>"
      }
    },
    "now_ts_sent": { "type": "util.timestamp", "config": { "format": "unix_ms" } },
    "pad_sent": { "type": "util.delay", "config": { "timeout": "{{ (nodes.start_ts + 500) > nodes.now_ts_sent ? (nodes.start_ts + 500 - nodes.now_ts_sent) : 0 }}ms" } },
    "respond_sent": { "type": "response.json", "config": { "status": 200, "body": { "message": "If that account needs verification, an email was sent" } } },
    "now_ts_unknown": { "type": "util.timestamp", "config": { "format": "unix_ms" } },
    "pad_unknown": { "type": "util.delay", "config": { "timeout": "{{ (nodes.start_ts + 500) > nodes.now_ts_unknown ? (nodes.start_ts + 500 - nodes.now_ts_unknown) : 0 }}ms" } },
    "respond_unknown": { "type": "response.json", "config": { "status": 200, "body": { "message": "If that account needs verification, an email was sent" } } },
    "now_ts_verified": { "type": "util.timestamp", "config": { "format": "unix_ms" } },
    "pad_verified": { "type": "util.delay", "config": { "timeout": "{{ (nodes.start_ts + 500) > nodes.now_ts_verified ? (nodes.start_ts + 500 - nodes.now_ts_verified) : 0 }}ms" } },
    "respond_verified": { "type": "response.json", "config": { "status": 200, "body": { "message": "If that account needs verification, an email was sent" } } }
  },
  "edges": [
    { "from": "start_ts", "to": "find_user" },
    { "from": "find_user", "to": "check_unverified" },
    { "from": "find_user", "output": "not_found", "to": "now_ts_unknown" },
    { "from": "check_unverified", "output": "then", "to": "verify_token" },
    { "from": "check_unverified", "output": "else", "to": "now_ts_verified" },
    { "from": "verify_token", "to": "send_verify_email" },
    { "from": "send_verify_email", "to": "now_ts_sent" },
    { "from": "now_ts_sent", "to": "pad_sent" },
    { "from": "pad_sent", "to": "respond_sent" },
    { "from": "now_ts_unknown", "to": "pad_unknown" },
    { "from": "pad_unknown", "to": "respond_unknown" },
    { "from": "now_ts_verified", "to": "pad_verified" },
    { "from": "pad_verified", "to": "respond_verified" }
  ]
}
```

- [ ] **Step 4: Run resend behavior + validation**

Run: `go test ./cmd/noda/ -run 'TestAuthScaffold_ResendVerificationIsConstantTime|TestAuthInitOutputValidates' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/noda/auth_templates/workflows/auth.resend-verification.json.tmpl cmd/noda/auth_templates/tests/test-auth-resend-verification.json cmd/noda/auth_init_test.go
git commit -m "fix(auth-scaffold): constant-time resend-verification response (auth-2)"
```

---

### Task 5: Docs, CHANGELOG, full gate

**Files:**
- Modify: `CHANGELOG.md`
- Modify/append: a doc note (the auth guide under `docs/` that describes the scaffolded flows, or the `noda auth init` "Next steps" output in `cmd/noda/auth_init.go` if that is where flow behavior is documented)

- [ ] **Step 1: CHANGELOG entry**

Under `## [Unreleased] → ### Security` add:

"Auth scaffold anti-enumeration: `noda auth init` now generates a **verification-first** register flow — both a new and an already-registered email return an identical `200` with no session cookie and send an email, so registration no longer discloses which addresses exist (it no longer auto-logs-in; users verify then log in). The password-reset and resend-verification flows now respond at a **fixed ~500 ms deadline** on every branch (via `util.timestamp` + `util.delay`), so the synchronous SMTP send on the known-account path no longer leaks account existence through response timing. For a hard timing guarantee, move the email send to an async worker. Also: `util.delay` now resolves its `timeout` per request, enabling computed delays."

- [ ] **Step 2: Doc note**

Add a short note where the scaffolded auth flows are documented: register is verification-first (no auto-login); reset/resend pad to a fixed ~500 ms deadline to resist timing enumeration; high-security deployments should send email from an async worker (emit an event, consume it in a worker) so the response never waits on SMTP. Keep it to a few sentences.

- [ ] **Step 3: Full gate**

Run:
```bash
gofmt -l . | grep -v '^examples/' ; go vet ./... ; golangci-lint run ; go test -race ./plugins/core/util/... ./cmd/noda/...
```
Expected: gofmt clean (ignore the pre-existing `examples/wasm-helpers/...` hit), vet clean, golangci-lint 0 issues, tests pass. Fix any lint introduced by this branch; note (don't fix) unrelated pre-existing issues.

- [ ] **Step 4: Commit**

```bash
git add CHANGELOG.md cmd/noda/auth_init.go docs
git commit -m "docs(auth): changelog + notes for anti-enumeration scaffold changes"
```

---

## Self-Review

- **Spec coverage:** Unit 0 → Task 1; auth-1 (Unit 1) → Task 2; auth-2 (Unit 2) reset → Task 3, resend → Task 4; tests woven into Tasks 2–4; Unit 3 docs/changelog/gate → Task 5. All covered.
- **Deliberate spec refinement:** the spec's "converge to one respond" is implemented as **separate, identical per-branch respond nodes with no join nodes**, because the engine's `computeJoinTypes` can misclassify a merge whose inbound includes the conditional node itself as an AND-join (deadlock). Same security outcome. Documented in Global Constraints.
- **Type/id consistency:** node ids used in each `tests/*.json` mock set exactly match the ids in the corresponding template (`start_ts`, `now_ts_sent`, `pad_sent`, `respond_sent`, `now_ts_unknown`, `pad_unknown`, `respond_unknown`, and for resend `now_ts_verified`, `pad_verified`, `respond_verified`). The pad expressions reference the matching `now_ts_*` in the same branch.
- **Open verification (flagged in Task 3, Step 3–4):** the exact scalar reference form for a `util.timestamp` output (`nodes.start_ts` vs a wrapped field) must be confirmed against a real run and applied uniformly to all `pad_*` expressions.
- **No new infra:** confirmed — only `util.delay`, `util.timestamp`, `email.send`, `auth.*`, `control.if`, `response.json` (all already core/available). Scaffold stays DB+email.
