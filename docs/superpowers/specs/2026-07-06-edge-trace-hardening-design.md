# Edge & Trace Hardening (Tranche D) — Design

Date: 2026-07-06
Source: `REVIEW-FINDINGS-2026-07-05.md` — the mechanical security cluster: data-1, server-2, realtime-2, realtime-3, realtime-4, nodes-1, nodes-2, edge-io-1 (8 findings; 7 Medium, 1 Low).
Branch/worktree (planned): `feat/edge-trace-hardening` in `.worktrees/edge-trace-hardening`, off `main`.

## Why

The 2026-07-05 review's "auth & edge" tranche split (user decision): the 8 mechanical, low-UX-risk security guards ship here; **auth-1/auth-2 (account-enumeration in the scaffolded register/reset/resend flows) are deferred to a separate auth-anti-enumeration tranche** — they require a flow/template redesign and a UX trade-off (register no longer auto-logs-in, async email) that deserves its own design. This tranche closes information-disclosure and injection/DoS surfaces that are pure defensive guards.

**Decisions (user-approved):** split as above; dev `/ws/trace` locked down via Origin check + loopback bind (no token); one PR.

## Findings in scope

| ID | Sev | Summary |
|---|---|---|
| data-1 | Med | `db.create`/`db.upsert` put the raw Postgres error into `ConflictError.Reason`; returned verbatim in 409 bodies in production → constraint/column/value disclosure + enumeration |
| server-2 | Low | Typed workflow errors (`ConflictError`, `ServiceUnavailableError`) leak DB/schema/driver detail regardless of dev mode |
| realtime-2 | Med | Trace redaction only handles `map[string]any`/`[]any`/`*api.HTTPResponse`; concretely-typed `[]map[string]any` (from `db.query`/`db.find`) bypasses `redactSecrets` → DB rows with secret columns leak to the trace stream |
| realtime-3 | Med | LiveKit ingress `stream_key` matches no sensitive pattern → leaks in traces |
| realtime-4 | Med | Dev-mode `/ws/trace` has no Origin check → cross-site/remote-browser trace exfiltration (inputs, DB rows, secrets) |
| nodes-1 | Med | `response.redirect` `/\evil.com` bypasses the protocol-relative open-redirect guard (browsers normalize `\`→`/`) |
| nodes-2 | Med | `ws.send`/`sse.send` channel resolved from an expression is passed to a wildcard matcher → cross-user/broadcast injection |
| edge-io-1 | Med | `image.resize` (and `image.crop`) enforce no output-dimension ceiling → arbitrary gigapixel output = allocation/DoS bomb |

## Verified facts

- `plugins/db/create.go:79-81` and `upsert.go:94-96` build `&api.ConflictError{Resource: table, Reason: errMsg}` where `errMsg` is the raw driver error. `ConflictError.Error()` = `"conflict on %s: %s"` (`pkg/api/errors.go:62`). `ServiceUnavailableError.Error()` includes `%v` of `Cause` (the driver error) (`errors.go:14`).
- `internal/server/errors.go MapErrorToHTTP`: the `ConflictError` (409) and `ServiceUnavailableError` (503) branches return `err.Error()` verbatim; only the default (500) branch gates on `devMode`.
- `internal/trace/redact.go`: `redactSecrets` handles `map[string]any` (recursing) and `[]any` (via `redactSlice`); `Emit` (`events.go:122-127`) dispatches only `map[string]any` and `*api.HTTPResponse`. A `[]map[string]any` value matches neither → unredacted. `sensitiveContains` lacks `stream_key`; `sensitiveExact` = `["key"]` (exact only).
- `internal/trace/websocket.go:14 RegisterTraceWebSocket` registers `/ws/trace` with `websocket.New(handler)` and no `Origins`/CheckOrigin (contrib default allow-all). `internal/server/middleware.go:182-188` has a reusable localhost-origin predicate.
- `plugins/core/response/redirect.go`: rejects `\r\n`, `//`-prefix, and non-`/`/non-`http(s)`; `/\evil.com` starts with `/` and passes.
- `plugins/core/ws/send.go` / `plugins/core/sse/send.go`: resolve `channel` via `plugin.ResolveString`, pass to `svc.Send`. `internal/connmgr/manager.go:190 matchConnections` treats `*` as wildcard (`matchWildcard`, `"*"` = all).
- `plugins/image/resize.go` + `crop.go`: `width`/`height` from `plugin.ResolveOptionalInt`, passed straight into `bimg.Options` with no ceiling.

## Design

### Unit 1 — Error-detail leak (data-1 + server-2)

Two layers:
- **db plugin** (`create.go`, `upsert.go`): set `ConflictError.Reason` to a **safe** constant string (`"unique constraint violation"`) instead of the raw `errMsg`. Keep `Resource: table` (table name is low-sensitivity and already surfaced in routes). No raw driver string, offending value, or column name leaves the plugin.
- **server** (`internal/server/errors.go MapErrorToHTTP`): for `ConflictError` and `ServiceUnavailableError`, build the client `Message` from **safe fields** in production and the full `.Error()` only in `devMode`:
  - `ConflictError` → prod `fmt.Sprintf("conflict on %s", cfErr.Resource)`; dev `cfErr.Error()`.
  - `ServiceUnavailableError` → prod `fmt.Sprintf("service unavailable: %s", suErr.Service)` (no `Cause`); dev `suErr.Error()`.
  - `ValidationError`/`NotFoundError` messages are unchanged (intended for clients; carry no driver detail).

This is defense-in-depth: even if a future call path builds a ConflictError with a raw reason, the server no longer surfaces it in prod.

### Unit 2 — Reflection-based trace redactor (realtime-2)

Replace the concrete type-switch in `redactSecrets`/`redactSlice`/the `Emit` dispatch with a single reflection-based deep redactor `redactValue(v any) any` in `internal/trace/redact.go` that handles **any** value:
- `map[string]any` (and other `map[string]T` via reflection): copy, redact sensitive keys (`IsSensitiveKey`), recurse into values; preserve the cookie-container narrow redaction.
- Slices/arrays of **any** element type (`[]any`, `[]map[string]any`, `[]T`): copy, recurse into each element.
- Scalars/other: returned as-is.
- `Emit` calls `redactValue(event.Data)` for all non-`*api.HTTPResponse` data (the `*api.HTTPResponse` path keeps `redactHTTPResponse`, whose Body now routes through `redactValue`).

Reflection keeps this robust against future node return shapes rather than chasing one concrete type. Guard against cycles is unnecessary (trace data is JSON-serializable trees), but cap recursion depth defensively.

### Unit 3 — `stream_key` and adjacent key redaction (realtime-3)

Add `stream_key`, `signing_key`, `private_key` to `sensitiveContains` in `internal/trace/redact.go`. (`_key`-suffixed secrets are the LiveKit/webhook family; `stream_key` is the reported one.)

### Unit 4 — Dev `/ws/trace` Origin check (realtime-4)

In `RegisterTraceWebSocket` (`internal/trace/websocket.go`), reject the WebSocket upgrade when the request's `Origin` header is cross-origin. Implement as a pre-upgrade Fiber handler on `/ws/trace` (runs before `websocket.New`) that allows only same-host / localhost origins (reuse the localhost predicate shape from `middleware.go`), returning 403 otherwise; an empty `Origin` (non-browser client, e.g. the CLI) is allowed. Document that dev mode binds loopback. The no-op trace WS (non-dev) is unchanged (it streams nothing).

### Unit 5 — Open-redirect `/\` guard (nodes-1)

In `response.redirect`, before the existing prefix checks, normalize the target for the authority check: for a URL beginning with `/`, reject when the **second byte is `/` or `\`** (`//x`, `/\x`, `/\\x`). Concretely, replace the `strings.HasPrefix(urlStr, "//")` check with: reject if `len(urlStr) >= 2 && urlStr[0] == '/' && (urlStr[1] == '/' || urlStr[1] == '\\')`. Keep the CRLF and scheme checks. Absolute `http(s)://` URLs are still allowed as today.

### Unit 6 — Cross-user send guard (nodes-2)

In the `ws.send` and `sse.send` executors, after resolving `channel`, reject when it contains `*` (or any wildcard metacharacter the matcher honors) with `fmt.Errorf("ws.send: channel must be a literal name, not a pattern")`. A send targets one literal channel; wildcards are only meaningful for subscription-side `channels.pattern`. (Empty channel already handled by required-field validation.)

### Unit 7 — Image output-dimension ceiling (edge-io-1)

In `image.resize` and `image.crop`, before building `bimg.Options`, enforce a ceiling on the requested output dimensions:
- Defaults: `maxWidth = 10000`, `maxHeight = 10000`, `maxPixels = 40_000_000` (~40 MP).
- Overridable per node via `max_width` / `max_height` / `max_pixels` config (resolved ints).
- Reject with `fmt.Errorf("image.resize: output dimensions %dx%d exceed limit", width, height)` when `width > maxWidth`, `height > maxHeight`, or `width*height > maxPixels` (guard the multiply against int overflow — compare as int64).
- A shared helper `enforceDimensionLimit(width, height, cfg)` in the image plugin, used by both nodes.

## Testing (per finding)

- **data-1/server-2:** `MapErrorToHTTP` returns a generic message for a `ConflictError`/`ServiceUnavailableError` when `devMode=false` and the full detail when `devMode=true`; the db plugin builds `ConflictError.Reason == "unique constraint violation"` (not the raw errMsg) for a duplicate-key error.
- **realtime-2:** emit an event whose `Data` is `[]map[string]any{{"password": "p", "id": 1}}`; assert the streamed JSON has `password` redacted. Add a nested `[]map[string]any` case too.
- **realtime-3:** a map with `stream_key` is redacted.
- **realtime-4:** a `/ws/trace` upgrade with a cross-origin `Origin` header is rejected (403); same-origin/localhost and empty-Origin are accepted.
- **nodes-1:** `response.redirect` with `/\evil.com` (and `/\\evil.com`) returns an error; `/path`, `http://x`, `https://x` still succeed.
- **nodes-2:** `ws.send`/`sse.send` with a resolved channel `user.*` (or `*`) returns an error; a literal channel succeeds.
- **edge-io-1:** `image.resize` with `width: 100000` (or `width*height > maxPixels`) returns an error before calling bimg; a normal size succeeds; a per-node `max_width` override is honored.

Gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/... ./plugins/...`.

## Mechanics

- Worktree `.worktrees/edge-trace-hardening`, branch `feat/edge-trace-hardening` off `main`.
- Subagent-driven execution per task: implementer → spec-compliance reviewer → code-quality reviewer.
- Spec + plan force-added to the branch.
- CHANGELOG "Security" entry.
- At merge: add a "Shipped 2026-07-06" note for these findings to `REVIEW-FINDINGS-2026-07-05.md` (on review PR #262's branch).

## Out of scope

auth-1/auth-2 (separate auth-anti-enumeration tranche); the other realtime/edge findings not listed (realtime-1 CSWSH was already downgraded and is the app's WS endpoints generally — handled separately; realtime-5/6, edge-io-2/3/4 not in this set). No change to the connmgr wildcard matcher itself (subscription patterns remain valid).
