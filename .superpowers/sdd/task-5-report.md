# Task 5 Report — `email.send` E2E against Mailpit

## What was implemented

Created `plugins/email/engine_e2e_integration_test.go` (build tag `integration`, package `email`) with two tests:

- **`TestEmailSend_Engine`**: starts a real Mailpit container via `containers.StartMailpit(t)`, creates the email plugin service with `{host, port, from}` (no explicit TLS), registers it, compiles and executes a one-node `email.send` workflow, then polls Mailpit's HTTP API (`GET /api/v1/messages`) for up to 5 seconds and asserts `total == 1`, `Subject == "Noda E2E"`, and `To[0].Address == "recipient@example.com"`.
- **`TestEmailSend_UnreachableHost_Engine`**: creates a service pointing at port 1 (nothing listening), runs the workflow, and asserts a non-nil error with no panic.

## TLS bug — present and fixed

**Was the bug present?** Yes.

`plugins/email/plugin.go`'s `CreateService` previously defaulted `useTLS = true` unconditionally. Any service created without an explicit `"tls": false` key would use `tls.Dialer.DialContext` for the initial TCP connection — an implicit TLS handshake — against whatever port was configured. Mailpit's SMTP listener (port 1025) speaks plaintext and rejects TLS immediately, causing `email.send` to fail with a TLS handshake error.

**Dial logic before:**
```go
useTLS := true
if v, ok := config["tls"].(bool); ok {
    useTLS = v
}
```

**Dial logic after:**
```go
// Default useTLS: true only for port 465 (implicit TLS / SMTPS).
// For all other ports (25, 587, custom), default to false so that
// plaintext SMTP servers (e.g. Mailpit on 1025) are reachable.
// An explicit "tls" config key always takes precedence.
useTLS := port == 465
if v, ok := config["tls"].(bool); ok {
    useTLS = v
}
```

This is the minimal security-preserving fix:
- Port 465 (SMTPS) still defaults to implicit TLS.
- Ports 25, 587, and any custom port (including 1025) default to plaintext, which allows STARTTLS to be negotiated by the SMTP library if the server advertises it.
- An explicit `"tls": true` or `"tls": false` in config overrides the default, so operators can still use TLS on non-standard ports.

The unit test `TestCreateService_Default` was updated to assert `useTLS == false` for the default port 587 case (was incorrectly asserting `true`).

## Mailpit API field names

The Mailpit `/api/v1/messages` JSON response uses:
- `total` (int) — number of messages
- `messages` (array) — each message has:
  - `Subject` (string, capital S)
  - `To` (array of `{Address string, Name string}`)

These matched the brief's assumptions exactly; no struct tag adjustments were needed.

## STARTTLS security fix (opportunistic upgrade on plaintext path)

**Problem identified post-task-5:** The prior fix (`useTLS = port == 465`) corrected implicit-TLS on all ports but left the plaintext dial path with no encryption whatsoever — a silent security downgrade for STARTTLS submission servers (ports 587/25).

**Fix:** `plugins/email/service.go` `dialCtx` plaintext branch updated to attempt STARTTLS after `smtp.NewClient`:

```go
// Before (plaintext branch):
conn, err := dialer.DialContext(ctx, "tcp", addr)
if err != nil {
    return nil, err
}
return smtp.NewClient(conn, s.host)

// After (plaintext branch):
conn, err := dialer.DialContext(ctx, "tcp", addr)
if err != nil {
    return nil, err
}
client, err := smtp.NewClient(conn, s.host)
if err != nil {
    return nil, err
}
if ok, _ := client.Extension("STARTTLS"); ok {
    if err := client.StartTLS(&tls.Config{ServerName: s.host}); err != nil {
        _ = client.Close()
        return nil, err
    }
}
return client, nil
```

The `dialFn` test-seam early-return and the implicit-TLS branch (port 465) are unchanged. `crypto/tls` was already imported.

Net behaviour:
- port 465 → implicit TLS (unchanged)
- port 587/25 with STARTTLS-advertising server → plaintext connect then encrypted upgrade
- Mailpit (no STARTTLS) → stays plaintext, test still passes

## Test results

**Unit tests (`go test ./plugins/email/`):**
```
ok  github.com/chimpanze/noda/plugins/email  0.443s
```

**Integration tests (`go test -tags=integration ./plugins/email/ -run Engine -v`):**
```
--- PASS: TestEmailSend_Engine (2.13s)
--- PASS: TestEmailSend_UnreachableHost_Engine (0.00s)
PASS
ok  github.com/chimpanze/noda/plugins/email  2.553s
```

**Default build (`go build ./...`):** Clean, no output.

## Files changed

- `plugins/email/engine_e2e_integration_test.go` — created (new integration test)
- `plugins/email/plugin.go` — fixed TLS default (`useTLS := port == 465`)
- `plugins/email/email_test.go` — updated `TestCreateService_Default` assertion for new default
- `plugins/email/service.go` — added opportunistic STARTTLS on plaintext dial path
- `docs/superpowers/specs/2026-06-25-external-node-e2e-findings.md` — Bug 2 entry corrected to reflect both fixes

## Self-review

- Happy path covered: delivery asserted via Mailpit API (subject + recipient address, not just `total > 0`).
- Error path covered: unreachable host returns a clean error, no panic.
- All test names contain "Engine": `TestEmailSend_Engine`, `TestEmailSend_UnreachableHost_Engine`.
- TLS fix is minimal (one line changed in plugin.go), security-preserving (port 465 still TLS by default, explicit config still respected).
- STARTTLS opportunistic upgrade restores encryption for 587/25 servers that advertise it.
- Findings documented accurately in the findings doc.
- Existing email unit tests pass.
- Only task-related files staged.

## Concerns

None.
