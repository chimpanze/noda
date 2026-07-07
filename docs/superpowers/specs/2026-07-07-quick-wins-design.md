# Quick-wins tranche — design (2026-07-07)

Six small, independent fixes from the open-issue triage after the 2026-07-05
review followup closed out (tranche G / PR #290). Scope approved by the user,
including the #271 precedence decision. Issues: #264, #266, #269, #271, #280,
#281.

All six are grounded against current `main` (`affbc9c`). Each section states
the observed defect, the decided fix, and the test that pins it.

---

## 1. #264 — wasm gateway: `parseReconnectConfig` breaks under msgpack

**Defect** (`internal/wasm/gateway.go:337-352`): `max_attempts` and
`initial_delay` are read with raw `.(float64)` assertions. msgpack decodes
small integers as `int8`/`int64`, so for a module configured
`encoding: "msgpack"` both coerce to zero — reconnection is silently
disabled (`reconnectLoop` exits immediately on `MaxAttempts`… actually
`Enabled` survives (`bool` is codec-stable), but attempts/delay are 0, so
behavior degrades to whatever zero semantics the loop has — either way, not
what was configured).

**Fix**: use the existing `toInt64`/`toFloat` helpers
(`internal/wasm/coerce.go`) for the two numeric fields, exactly mirroring the
`CloseConn`/`Configure` fixes from tranche A. `enabled` (bool) and `backoff`
(string) are codec-stable and stay as-is.

**Test**: unit test calling `parseReconnectConfig` with msgpack-shaped values
(`int8(5)` for `max_attempts`, `int64(250)` for `initial_delay`) asserting
`MaxAttempts == 5` and `InitialDelay == 250ms`; keep a float64 (JSON-shaped)
case as control. Must fail against the raw-assertion code.

## 2. #266 — wasm module: `SendCommand` `outstandingCalls.Add` can race `Stop`'s `Wait`

**Defect** (`internal/wasm/module.go:296-339`): `SendCommand` is called from
workflow goroutines (`plugins/core/wasm/send.go` via
`WasmService.SendCommand`) at arbitrary times. Its only guard is the
lock-free `m.failed` check; it then takes `m.mu` and calls
`m.outstandingCalls.Add(1)`. `Stop` sets `m.running = false` under `m.mu`,
releases it, and later `Wait()`s on `outstandingCalls`. A `SendCommand`
acquiring `m.mu` after `Stop` released it can `Add(1)` concurrently with the
`Wait` at counter zero — the WaitGroup misuse pattern Go documents as
panic-prone ("Add with positive delta concurrent with Wait").

**Fix**: inside the `m.mu`-held section of `SendCommand`, before any work,
check `m.running` and drop the command with a warn log if false. `running` is
mutated only under `m.mu` (Start/Stop), so a passing check strictly
happens-before Stop's `Wait` — the race is closed by lock ordering, not by
probabilistic flags. (`m.stopping` is NOT sufficient: Stop sets it *after*
releasing `m.mu`, leaving a window where `running` is already false but
`stopping` isn't yet set.)

**Non-fix, documented**: the other two `outstandingCalls.Add` sites
(`internal/wasm/hostapi.go:117,185`) run only inside host functions, i.e.
during a guest export invocation. All guest invocations are serialized
through the tick loop / `Stop`'s own `callWithTimeout("shutdown")`, both of
which complete before `Stop` reaches `Wait()` — those `Add`s are
happens-before the `Wait` by construction. Add a comment at the `Wait` site
capturing this invariant so a future caller doesn't silently break it.

**Test**: after `Stop` returns, `SendCommand` must be a no-op (no goroutine
launched, no panic, command not buffered). Plus a stress test under `-race`:
N goroutines hammering `SendCommand` while `Stop` runs concurrently; the
test's assertion is "no panic / no race report" (polarity: against unfixed
code, `-race` flags the WaitGroup misuse or the panic fires
probabilistically — mark the stress test as best-effort detection, the
deterministic drop-after-stop test is the pinned regression).

## 3. #269 — examples: wasm-counter & discord-bot fail tinygo build

**Defect**: `examples/wasm-counter/wasm/counter/main.go` passes `cmd.Data`
(PDK type `any`) and `examples/discord-bot/wasm/bot/main.go` passes
`msg.Data` (`any`) directly to `json.Unmarshal`, which takes `[]byte`.
Doesn't compile. Root cause of the confusion: the host delivers `.Data`
already **codec-decoded** (`gateway.go readLoop` unmarshals into `any`;
commands likewise), so there are no raw bytes to unmarshal — the examples
were written against an imagined raw-bytes ABI.

**Fix**: in each example module, add a small local helper

```go
func decodeInto(v any, dst any) error {
    b, err := json.Marshal(v)
    if err != nil {
        return err
    }
    return json.Unmarshal(b, dst)
}
```

and use it where `.Data` is consumed (counter: `cmd.Data` → op struct;
discord-bot: `msg.Data` → `gatewayPayload`). The double round-trip is fine at
example scale and keeps the typed-struct style. Discord's nested
`json.RawMessage` fields work because `decodeInto` re-marshals to real JSON
first. No PDK API change (adding `noda.DecodeInto` is out of scope — would be
a PDK surface decision, note it in the PR as a possible follow-up).

**Verification** (no unit tests — guest modules aren't CI-built): local
`tinygo build -o /dev/null -target wasip1 -buildmode=c-shared` (match each
example's documented build command) for **wasm-counter**, **discord-bot**,
and **wasm-helpers** as the known-good control.

## 4. #271 — server: workflow error/timeout paths bypass the `responseCh` drain

**Defect** (`internal/server/routes.go:429-462`): the `wfErr != nil` branch
returns `MapErrorToHTTP(...)` without draining `responseCh`. Since tranche B
made timeouts/starved-joins fail loudly, this path is newly reachable with a
response already produced: if the response node fires essentially
simultaneously with workflow completion, `select` may pick `workflowDone`
and discard the produced response — a nondeterministic 504/500 for identical
runs. The `timer.C` (response-timeout) branch has the same shape: if
`responseCh` and `timer.C` are both ready, the select can randomly return
504 despite a response sitting in the channel.

**Decision (user-approved)**: a produced response wins, deterministically, on
both branches. Rationale: had the scheduler interleaved slightly
differently, the client would have received that response anyway (line 430);
the loud-failure goal of tranche B is about *silent truncation*, and the
workflow error remains visible in the trace/logs.

**Fix**: in both the `wfErr != nil` branch and the `timer.C` branch, first
non-blocking-drain `responseCh`; if a response is present, write it via
`validateAndWriteResponse` (same as the success path) and log the suppressed
workflow error (error branch) at warn level with the trace ID. Otherwise
proceed with the current error/504 behavior.

**Test** (`internal/server`): simulate the race deterministically — invoke
the handler against a workflow whose response interceptor fires and whose
run function then returns an error, with the responseCh guaranteed populated
before `workflowDone` is read (e.g. a test workflow: `response.json` node →
node that fails; or drive the handler with a stubbed runner). Assert the
client gets the produced response, not the 5xx. A second test for the
timeout branch: response produced exactly as/after the deadline passes →
response wins over 504. Use the existing routes test harness/fixtures
(`testdata/` config style, per project conventions).

## 5. #280 — trace: `redactValue` returns values unredacted past the depth cap

**Defect** (`internal/trace/redact.go:54-57`): `redactValueDepth` returns
`v` **unredacted** once `depth > maxRedactDepth` (32). Fail-open in the one
direction a security redactor must not fail.

**Fix**: return the sentinel string `"[REDACTED: max depth]"` instead of `v`
past the cap. Scalars past the cap get scrubbed too — acceptable: 32-level
nesting in trace payloads is degenerate input, and over-redaction is the
safe direction.

**Test**: build a map nested past the cap with a non-sensitive leaf value;
assert the raw leaf does not appear anywhere in the redacted output and the
sentinel does. Control: a value at depth < cap passes through unredacted.

## 6. #281 — trace: dev `/ws/trace` origin compare is case-sensitive

**Defect** (`internal/trace/websocket.go:79-89`): `originAllowed` compares
`u.Hostname()` to `c.Hostname()` with `==`. Hostnames are case-insensitive
(RFC 4343); `Origin: http://Example.com` against `Host: example.com` is
wrongly rejected. Fails closed, so a nuisance, not a vulnerability.

**Fix**: `strings.EqualFold` for the origin-vs-host compare and the three
localhost literals.

**Test**: table-test `originAllowed` with mixed-case origin vs lowercase
host (allowed), mixed-case `LocalHost` (allowed), and a genuinely foreign
origin (rejected — polarity control).

---

## Cross-cutting

- **Branch/worktree**: `.worktrees/quick-wins`, branch `quick-wins`, off
  `main`. Spec+plan `git add -f`'d onto the branch per convention.
- **CHANGELOG**: entries under `[Unreleased]` — `### Fixed` for
  #264/#266/#269/#271/#281, `### Security` for #280 (redaction fail-safe).
- **Issue closure**: PR body `Fixes #264 … Fixes #281` (all six).
- **Gates**: `gofmt -l`, `go vet ./...`, `golangci-lint run`,
  `go test -race` on `internal/wasm`, `internal/trace`, `internal/server`;
  tinygo builds for the three example modules.
- **Not in scope**: #291 fixture regen, #283 scheduler lock keying, PDK
  `DecodeInto` helper, CI-building example wasm modules.
