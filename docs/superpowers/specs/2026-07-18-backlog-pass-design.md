# Backlog Pass 2026-07-18 — Design

**Date:** 2026-07-18
**Scope:** 13 open issues: #349, #350, #354–#359, #361, #363–#365, #368 (skipping #351, the harness re-run — deferred to a dedicated session).
**Centerpiece:** #363 — implement the cross-instance connection sync bridge. The other 12 are bug fixes, wiring gaps, and doc corrections with root causes already identified in their issues.

## Part 1 — Cross-instance connection sync bridge (#363)

### Problem

`connections.json` schema requires a `sync.pubsub` block and crossref validation checks it points at a pubsub service, but no runtime code consumes it. `ws.send`/`sse.send` deliver only via the local in-process `connmgr.Manager`. `docs/02-config/connections.md` §Cross-Instance Message Routing promises functionality (including a "Redis routing table") that does not exist.

### Decisions (user-approved)

1. **Implement the bridge** (not doc-only retreat).
2. **Hook point: wrap `EndpointService`** — `Send`/`SendSSE` deliver locally then publish; a per-endpoint subscriber goroutine feeds remote messages into the local `Manager`. `connmgr.Manager` core and the send nodes are untouched; future `ConnectionService` callers get sync for free.
3. **Publish failure fails the node** (error output), local delivery having already happened. Rationale: sync is explicitly configured; silent local-only delivery hides multi-instance breakage. Matches the project's fail-loudly direction. Documented so workflows can wire the error edge for best-effort.
4. **`sync` becomes optional** in the schema. Absent → local-only delivery (current behavior, documented honestly). Present → bridge active. Backward-compatible with every existing config; removes the forced-Redis tax for single-instance deployments.

### Components

**`internal/connmgr/sync.go` — `SyncBridge`**

- Fields: `pubsub api.PubSubService`, `instanceID string` (UUID generated once per server), `logger *slog.Logger`.
- Depends only on `pkg/api` — no Redis import in connmgr.
- Redis channel per endpoint: `noda:sync:<endpoint>`.
- `Publish(ctx, endpoint string, env Envelope) error`
- `Run(ctx, endpoint string, mgr *Manager)` — blocking subscribe loop, started as a goroutine; reconnects with backoff (~1s) on subscribe error; exits when ctx is cancelled.

**Envelope** (JSON via the pubsub service's own marshal):

```json
{
  "v": 1,
  "instance": "<boot uuid>",
  "kind": "ws" | "sse",
  "channel": "chat:42",
  "payload": "<pre-marshaled string>",
  "event": "…",   // SSE only
  "id": "…"       // SSE only
}
```

The sender marshals the payload once — the same bytes the local manager produces — so delivery is byte-identical on every instance and avoids JSON round-trip mangling (`[]byte` → base64, number widening). `Manager.Send`/`SendSSE` pass string payloads through unmodified, so the receive side needs no special casing.

**Send path** (`EndpointService`): marshal payload → local `Manager.Send`/`SendSSE` → `bridge.Publish`. Publish error returns from `Send`, surfacing on the node's error output. Bridge nil (no sync configured) → local-only, no behavior change.

**Receive path**: subscriber skips envelopes whose `instance` equals its own (Redis echoes to the publisher). The subscribe handler **never returns an error** — the pubsub service's `Subscribe` treats a handler error as fatal to the subscription. Malformed envelopes, unknown versions/kinds: log at warn, drop, continue.

**Wiring** (`internal/server/connections.go` `registerConnections`): per connection-config file with a `sync.pubsub` block, look up the named service in the registry, assert `api.PubSubService`, build one bridge; pass it to each endpoint's `NewEndpointService(mgr, name, bridge)` and start one subscriber goroutine per endpoint bound to a server lifecycle context (cancelled during shutdown, before manager Stop).

### Config & validation

- `internal/config/schemas/connections.json`: drop `"required": ["sync"]`.
- Existing crossref checks (service exists, plugin == "pubsub") remain, applied only when `sync` is present.

### Docs & changelog

- Rewrite `connections.md` §Cross-Instance Message Routing: per-endpoint fan-out to all subscribed instances; **no routing table** (claim removed); publish-failure = node error, local delivery already done; `sync` optional, absent = local-only.
- Update `examples/node-cookbook/realtime` README (currently documents the non-implementation).
- CHANGELOG: `sync` no longer required (validation relaxation); cross-instance sync now functional (feature).

### Testing

- **Unit** (fake in-memory `PubSubService`, two managers bridged): ws cross-delivery, sse cross-delivery (event/id preserved), self-echo skipped, publish failure → error from `Send`, malformed envelope dropped without killing the loop, subscriber reconnect after subscribe error, ctx cancel stops the loop.
- **Integration** (real Redis, same gating as existing Redis-backed tests): two `EndpointService`s + bridges on one Redis; message sent through one reaches connections registered on the other.

## Part 2 — Companion fixes (issues with root cause in hand)

| Issue | Fix |
|---|---|
| #354 strict mode rejects trigger mappings | Add transport namespaces (`body`, `query`, `params`, `headers`, `request`, `raw_body`) to `knownContextEnv` in `internal/expr/compiler.go`; strict-mode test compiling a representative trigger mapping. |
| #355 `noda test` can't evaluate `secrets.*` | Wire the SecretsManager loaded in `newTestCmd` into the test runner's engine execution (`engine.WithSecrets`), mirroring the server runtime. Un-mock the saas-backend webhook test if it can now run real. |
| #356 polish pair | Headers patcher `StringNode` copies the original node's source location; `hmac_verify` trims the `algorithm=` prefix case-insensitively. |
| #357 saas-backend `sync-github-issue` | Worker mapping coerces the id to string (`string(message.payload.issue.id)`); the workflow gains a `db.query` step on the `opened` path that picks the oldest project as the landing zone (supplying its `project_id`/`workspace_id`), logging-and-skipping when no project exists. GitHub payloads carry no workspace/project routing — the landing-zone convention is documented in the example README. |
| #358 workflow.run dynamic-ports docs | Fix `docs/03-nodes/workflow.run.md` to describe real `[success, error]` routing + success-collapse (consistent with `workflow.output.md`); delete dead `setOutputs`. |
| #359 `subWorkflowRunner` nil | Build the sub-workflow runner in `Setup()` after the self-built cache exists (keep the constructor path for injected caches). Regression test: server built without `WithWorkflowCache` executes `workflow.run`. |
| #361 unwired error ports degrade typed errors | Executor wraps the original node error with `%w` (retaining output data in the message) so `MapErrorToHTTP`'s `errors.As` matches; ValidationError → 422 etc. even with no error edge wired. Behavior change → CHANGELOG. Reproduce with `upload.handle` type-rejection. |
| #364 http body shallow-resolve | `doRequest` resolves `body` with `ResolveDeepAny` (matching sse/ws/event/wasm nodes). Behavior change → CHANGELOG. Update http.post/http.request docs; simplify the cookbook http workaround to the natural nested-template form (it becomes a live verification). |
| #365 wasm partial-load leak | In `cmd/noda/runtime.go` createWasm (and the cookbook runner mirror): unload already-loaded modules when a later `LoadModule`/registration fails. |
| #368 lk.* no timeout | Add an opt-in `timeout` config to the livekit **service** (context deadline applied per API call in the service wrapper); no default, so existing behavior is unchanged unless configured. On expiry the node fails with an operation-scoped timeout error; twirp errors returned before the deadline pass through unchanged (a call killed by our deadline cannot also yield the server's error — docs say so). Per-node override deferred (YAGNI). Cookbook livekit family sets it, shortening the slow error-path CI steps. |
| #349 validate/reload consistency | (1) `Reloader.HandleChange` runs the same dry-run startup validation as boot/CLI/editor and refuses the swap (emits `file:error`) on failure. (2) Editor `validateFile` scopes dry-run errors to the requested file's workflows via the resolved config's file→workflow mapping. |
| #350 multipart normalization | Both multipart branches (fasthttp + manual fallback) normalize repeated form values to `[]any` like urlencoded; loop-over-repeated-field test via multipart. Document the uppercase-multipart file-upload limitation in trigger/upload docs. |

## Sequencing & delivery

Three PRs off `backlog-2026-07-18`-style branches, grouped by review surface:

1. **PR A — small fixes batch:** #354, #355, #356, #357, #358, #359, #365 (no behavior changes beyond bug fixes).
2. **PR B — behavior changes:** #361, #364, #350, #349 (each with CHANGELOG + doc updates).
3. **PR C — sync bridge:** #363 feature + schema relaxation + docs, plus #368 (both touch service-level config).

Each PR: tests first (TDD), full `go test ./...`, lint, CHANGELOG where behavior changes, issue closed via PR body `Fixes #N`.
