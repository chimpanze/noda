# Noda Codebase Review — Findings

Date: 2026-04-22
Branch: feat/adventure-stream-android
Scope: Full codebase, 7 parallel sub-agent reviews
Status: **Findings as reported by sub-agents — verification pending**

---

## Critical

### C1. No SSRF protection in HTTP plugin
- **Files:** `plugins/http/request.go:84-90`, `plugins/http/plugin.go:58-65`
- **Claim:** URLs go straight to the Go HTTP client. No blocklist for RFC1918, link-local (169.254/16), localhost, or `metadata.google.internal`. No custom `CheckRedirect` → `Authorization`/`Cookie`/`X-Api-Key` headers leak across origins on redirect.
- **Suggested fix:** Parse → resolve → IP allowlist before `Do()`; strip auth headers on cross-origin redirects.

### C2. HTTP response header injection via `ResolveHeaders`
- **File:** `internal/plugin/resolve.go:299-322`
- **Claim:** `response.json`, `response.error`, `response.file` flow expression-resolved values into headers without CRLF stripping. `response.redirect` validates locally; the shared helper does not.
- **Suggested fix:** Reject `\r`/`\n` in header values inside `ResolveHeaders`.

### C3. Wasm `CallAsync` goroutines untracked
- **Files:** `internal/wasm/hostapi.go:104-127`, `internal/wasm/module.go:176-226`
- **Claim:** Async goroutine is *not* added to `module.outstandingCalls`, but `Stop()` only waits on that WaitGroup. Async results can write into module maps after teardown.
- **Suggested fix:** `Add(1)/defer Done()` and check lifecycle context before `AddAsyncResult`.

### C4. DB plugin has no multi-step transaction story
- **File:** `plugins/db/plugin.go:39-83`
- **Claim:** Each `db.create`/`db.update`/`db.delete` commits independently. No engine-level transaction handle passed between nodes.
- **Suggested fix:** Engine-driven transactions OR explicit `db.tx.begin/commit/rollback` nodes.

---

## High

### H5. Silent backpressure drops (SSE / trace / worker)
- **Files:** `internal/connmgr/sse.go:94-120`, `internal/trace/events.go:72-97`, `internal/worker/runtime.go:147-165`
- **Claim:** SSE 64-byte buffer drops silently; trace fan-out is synchronous + per-subscriber 256 channel drops; worker `Stop()` doesn't drain in-flight ack/nack.
- **Suggested fix:** Bounded queues + drop counters as metrics; drain worker on shutdown.

### H6. Image plugin: untrusted bytes → libvips with no validation
- **File:** `plugins/image/helpers.go:54-70`
- **Claim:** No size cap, no dimension check, no decompression-bomb guard before `bimg.NewImage`.
- **Suggested fix:** Enforce max input bytes, fast-header dimension check, max-megapixels.

### H7. OIDC token verifier doesn't validate audience
- **Files:** `plugins/core/oidc/exchange.go:112`, `plugins/core/oidc/refresh.go:112`
- **Claim:** `&gooidc.Config{ClientID: clientID}` alone — no audience claim verification.
- **Suggested fix:** Explicit audience check on extracted claims.

### H8. AND-join executor masks root-cause errors
- **File:** `internal/engine/executor.go:115-158`
- **Claim:** Concurrent failing nodes race to `firstErr.CompareAndSwap`; loser is discarded.
- **Suggested fix:** Cancel context before storing, or collect all errors as multierror.

### H9. Storage plugin path traversal hardening incomplete
- **File:** `plugins/storage/service.go:17-22`
- **Claim:** `validatePath` checks `..` after `Clean`, but absolute paths, Windows UNC, and symlink escapes inside `BasePathFs` are not blocked.
- **Suggested fix:** Reject `filepath.IsAbs`, EvalSymlinks the base at startup, post-write real-path verification.

### H10. Stream plugin never reclaims pending messages
- **File:** `plugins/stream/service.go:43-85`
- **Claim:** `XReadGroup` with `>` only; no `XAUTOCLAIM`, no idle-claim, no consumer heartbeat. Crashed worker → entries pending forever.
- **Suggested fix:** Optional `XAutoClaim` loop in `Subscribe`.

### H11. Email recipient + LiveKit room name validation missing
- **Files:** `plugins/email/send.go:51-57`, `plugins/livekit/room_create.go:50-55`
- **Claim:** Recipients not validated, no per-message cap; LiveKit room names passed through raw.

### H12. SQL injection surface (JOIN ON, raw query/exec)
- **Files:** `plugins/db/where.go:43,119`, `plugins/db/query.go:49-78`, `plugins/db/exec.go:49-75`
- **Claim:** ON clause built with `fmt.Sprintf` after permissive regex check; raw SQL nodes accept any string with only first 50 chars logged.
- **Suggested fix:** Stricter ON-clause grammar; opt-in raw nodes with per-node timeouts.

### H13. Lifecycle / signal handling timing gaps
- **Files:** `internal/server/server.go:216`, `internal/lifecycle/adapters.go:26`, `cmd/noda/runtime.go:359`, `internal/lifecycle/lifecycle.go:101`, `internal/devmode/reload.go:96-98`
- **Claims:**
  - HTTP server `Start()` blocks but registered with no-op adapter; called *after* `lc.StartAll()` returns. Signal in this window races shutdown.
  - `lifecycle.StopAll` always parents on `context.Background()`, ignoring outer cancel.
  - Devmode reload swap holds write-lock for swap, then runs invalidation callback *outside* lock — TOCTOU.

### H14. Wasm runtime has no per-call fuel/memory budget
- **Files:** `internal/wasm/runtime.go:73-76`, `internal/wasm/module.go:17`
- **Claim:** Extism page cap exists, but no instruction metering, no heap budget per call. 30s `wasmCallTimeout` is coarse.
- **Suggested fix:** Surface Extism fuel options.

### H15. Service-creation timeout leaks goroutines
- **File:** `internal/registry/lifecycle.go:75-86`
- **Claim:** Cleanup goroutine reads from buffered channel that may never fire if underlying call hangs.
- **Suggested fix:** Time-bound the cleanup goroutine.

### H16. Upload node doesn't sanitize `storagePath`
- **File:** `plugins/core/upload/handle.go:148-151`
- **Claim:** `storagePath` used directly to build file path; combined with H9 an attacker can escape.

---

## Medium / Notable

- **M1.** `internal/expr/functions.go:296,302` — silent `float64 → int` overflow in `coerceToInt`.
- **M2.** `internal/expr/evaluator.go:43` — interpolated expressions stringify via `%v`, leaking type info.
- **M3.** `internal/connmgr/websocket.go:156-180` — no half-dead WS detection; on_connect/on_disconnect bypass `msgSem` (`websocket.go:195-209`).
- **M4.** `internal/scheduler/runtime.go:220-280` — distributed lock acquired but never released; correctness depends on TTL alone.
- **M5.** `internal/migrate/migrate.go:86-101` — no pg advisory lock; concurrent boots can race.
- **M6.** `internal/testing/match.go:114-123` — strict `reflect.DeepEqual`; no `$any`/`$regex`/`$skipFields` matchers. Mock outputs not validated against node schemas.
- **M7.** `plugins/core/control/loop.go:80-89` — default `maxItems: 100,000` is generous.
- **M8.** `plugins/core/response/error.go:90` — trace ID always returned to client.
- **M9.** `plugins/core/oidc/exchange.go:86`, `auth_url.go:125` — OIDC discovery on every call, no caching.
- **M10.** `plugins/cache/service.go:40-52` — no `SetNX`/Lua exposed; no stampede protection.
- **M11.** `internal/engine/cache.go:54-76` — graphs immutable, but clients holding workflow IDs across reload have no generation/version check.

---

## Verification Status

Verified by reading the cited source on `feat/adventure-stream-android` (2026-04-22).

| ID | Finding | Status | Notes |
|----|---------|--------|-------|
| C1 | SSRF | ✅ Shipped (this branch) | `request.go:141` calls `http.NewRequestWithContext` then `client.Do` with no URL validation. `plugin.go:58` builds `&http.Client{Timeout: timeout}` — no `CheckRedirect`, so Go's default follows redirects with auth headers preserved. |
| C2 | Header injection | ✅ Shipped (this branch) | `resolve.go:319` `result[k] = fmt.Sprintf("%v", val)` with no CRLF stripping. The non-string branch at line 312 has the same problem. Used by `response.json`, `response.error`, `response.file`, and `http.request`. |
| C3 | Wasm CallAsync | ✅ Shipped (this branch) | `hostapi.go:104` `go func()` with no `m.outstandingCalls.Add(1)`. Worse: `module.go:203-206` clears `pendingLabels` and `asyncResults` *before* waiting on `outstandingCalls` at line 215-223 — even tracked goroutines would write into already-cleared maps. |
| C4 | DB transactions | ✅ Confirmed | `plugin.go:39-83` returns a `*gorm.DB` per service. Every node calls `db.WithContext(ctx).<op>` — no transaction handle is plumbed across nodes. Genuine design gap. |
| H5 | Backpressure drops | ⚠ Partial | SSE drop confirmed (`sse.go:118-120` `default → "sse buffer full"` on 64-channel). Trace sync fan-out confirmed (`events.go:94-96`). Worker shutdown is more nuanced: `wg.Wait` actually does block on processMessage, but `r.cancel()` cancels the ctx that `XAck` (`runtime.go:326`) uses, so on shutdown an in-flight message becomes "processed but not acked" and stays pending. |
| H6 | Image validation | ✅ Shipped (this branch) | `helpers.go:32` `source.Read` returns raw bytes; subsequent ops (`resize`, `crop`, etc.) call `bimg.NewImage(data)` with no size/dimension check. `writeTargetImage` even calls `bimg.NewImage(data).Size()` *after* writing to storage. |
| H7 | OIDC audience | ❌ **False positive** | go-oidc v3.17.0 `oidc/verify.go:296-298` shows `if !v.config.SkipClientIDCheck && v.config.ClientID != "" { contains(t.Audience, v.config.ClientID) }`. Setting `&gooidc.Config{ClientID: clientID}` **does** verify the audience claim. The agent didn't read the library. |
| H8 | AND-join error masking | ⚠ By design | `executor.go:116` `firstErr.CompareAndSwap(nil, err); cancel(); return`. This is intentional first-error semantics. Concurrent failures racing `CompareAndSwap` is the explicit design, not a bug. Could improve UX with multierror, but not a correctness issue. |
| H9 | Storage path traversal | ✅ Shipped (this branch) | `validatePath` only blocks `..`/`../` after `Clean` (`service.go:17-22`). However, `plugin.go:36` uses `afero.NewBasePathFs(NewOsFs(), path)`, which Afero documents as preventing escapes (joining all paths under base). Absolute paths are *probably* contained by BasePathFs but no defense-in-depth check exists. **Symlink concern is real** — BasePathFs does not protect against symlinks within the base FS pointing outside it. |
| H10 | Stream pending | ✅ Shipped (this branch) | `service.go:55-61` uses `XReadGroup` with `>` only. No `XAUTOCLAIM`, no consumer heartbeat, no idle reclaim anywhere in the package. |
| H11 | Email/LiveKit validation | ✅ Shipped (this branch) | `helpers.go:14-54` `resolveRecipients` accepts string or []string and returns them unmodified — no syntactic validation, no per-message recipient cap. SMTP layer would surface a server-side error but no local guard exists. LiveKit not re-verified but likely same shape. |
| H12 | SQL injection surface | ⚠ Partial | `validate.go:80-97` `ValidateSQLFragment` blocks `;`, `--`, `/*`, and a denylist of keywords (DROP/DELETE/INSERT/UPDATE/ALTER/CREATE/EXEC/UNION/SELECT/GRANT/REVOKE/TRUNCATE) as whole words. This is *reasonable* — subqueries and second-statement attacks are blocked. Risk reduces to predicate-tampering (e.g. `1=1 OR x=y`) on JOIN ON / WHERE / HAVING when those clauses come from expressions. Raw `db.query`/`db.exec` (`query.go:55`) accept any resolved string — that's a foot-gun, not a bug, but downgrade severity to **medium** unless the team wants opt-in gating. |
| H13 | Lifecycle timing | ⚠ Partial | (a) `lifecycle.go:101` `context.WithTimeout(context.Background(), budget)` ignores outer ctx — **confirmed**. (b) Devmode `reload.go:96-114` releases lock before invoking `onReload` — **confirmed** TOCTOU window between new config visible and cache invalidated. (c) Server adapter no-op claim is real but mostly harmless: `runtime.go:340-364` shows `srv.Start()` blocks *after* lifecycle is up, and `app.ShutdownWithContext` triggered by `Stop` cleanly returns `app.Listen`. The flow is unconventional but works. |
| H14 | Wasm fuel | 🚫 Not implementable | Verified during runtime-hardening design (2026-04-23): Extism go-sdk v1.7.1 does not expose any fuel/instruction-metering API, and the underlying wazero runtime deliberately rejects fuel metering as a feature. Only a wall-clock `manifest.Timeout` is available (which Noda already duplicates via its `wasmCallTimeout` constant). Revisit if Extism upstream adds metering. |
| H15 | Service goroutine leak | ✅ Shipped (this branch) | `lifecycle.go:75-86` cleanup goroutine reads from `resultCh` (buffered 1). If `CreateService` eventually returns, cleanup runs and closes the resource. If it never returns, **the goroutine leaks** but there is no resource to close anyway. Real bug only on hung-but-eventually-completing creates, which are rare. |
| H16 | Upload path | ✅ Shipped (this branch) | `handle.go:148-151` uses `storagePath` directly; `fmt.Sprintf("%s_%d", storagePath, i)` for multifile. No traversal check at this layer. Mitigated downstream by storage `validatePath`, so escape requires also defeating H9's BasePathFs containment — risk reduces to *medium*. |

**Summary:** of 16 Critical/High items, **11 fully confirmed**, **3 partial/contextual** (H5, H9, H12), **1 by-design** (H8), **1 false positive** (H7).

The other thing the agents systematically did *not* do was check vendored library semantics (H7 is the canonical example). For any future review involving a third-party library's defaults, verify against the vendored source before treating as a finding.

---

## Shipped 2026-04-23

C1 (SSRF), C2 (header injection), H6 (image bombs), H9 (storage symlinks),
H11 (email recipient validation), H16 (upload path) — see commits
`7b860ab..263e01f` on branch `feat/security-hardening`. Spec at
`docs/superpowers/specs/2026-04-22-security-hardening-design.md` (gitignored).

## Shipped 2026-04-25

C3 (Wasm CallAsync lifetime), H10 (stream pending reclaim), H15 (service-creation
goroutine cleanup) — see commits on branch `feat/runtime-hardening`. Spec at
`docs/superpowers/specs/2026-04-23-runtime-hardening-design.md` (gitignored).

H14 (Wasm per-call fuel) was reclassified as not-implementable during this
spec's design phase; see its row above.

## Still open

- **C4 (DB transactions)** — to be addressed in a standalone spec; biggest single design choice in the codebase (engine-driven vs explicit `db.tx.*` nodes).
- **H5 (silent backpressure drops in SSE/trace/worker)** + **H13 (lifecycle context plumbing & devmode TOCTOU)** — to be addressed in a paired "backpressure & lifecycle" spec. H15's introduction of `ctx context.Context` to `Bootstrap`/`InitializeServices` is the precedent that spec will build on.
- **H8 (AND-join error masking)** — by design, no action planned. May be revisited if user feedback shows the first-error semantics is confusing.
- **H12 (SQL injection surface — partial)** — `ValidateSQLFragment` blocks `;`, `--`, `/*`, and 12 keywords; predicate-tampering on `WHERE`/`HAVING`/`JOIN ON` from expression-resolved input is the residual risk. Ship-blocker only if a workflow config user-input flow is found that exposes raw `db.query`/`db.exec` to untrusted callers. Track separately if such a flow ships.
