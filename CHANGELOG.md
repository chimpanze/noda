# Changelog

All notable changes to Noda will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- A node's `error`-edge output now carries a `code` field — `VALIDATION_ERROR`, `NOT_FOUND`, `CONFLICT`, `SERVICE_UNAVAILABLE`, `TIMEOUT`, or `INTERNAL_ERROR` — so a workflow can branch on the cause of a failure without reading the raw `error` string (#417). The `error` string is unchanged and remains a diagnostic field: it may carry driver, network, or filesystem detail and should not be forwarded to clients. The workflow-error HTTP body uses the same vocabulary, now derived from one shared `api.ErrorCode` so those two surfaces cannot drift from each other (other server-side error codes, e.g. `RATE_LIMITED` or the Wasm host-call codes, are unrelated and still minted independently).
- `ServiceConfigSchema` on `api.Plugin` + `noda_get_service_schema` MCP tool — plugin service configs are declared, validated, and discoverable (#375 #376). **Upgrade note:** external `api.Plugin` implementations must add this method — return `nil` for plugins with no services.
- livekit service accepts an optional `timeout` (per-API-call deadline); unset keeps unbounded calls (#368)
- `auth.jwt` optional claim validation: `audience`, `issuer`, and `require_expiry` (all default off — when unset, behavior is unchanged)
- Prometheus metrics endpoint (`/metrics`) with OTel metrics API
- Trace sampling rate configuration (`observability.tracing.sampling_rate`)
- Circuit breaker support for outbound HTTP services (`circuit_breaker` config)
- Per-workflow timeout configuration (`timeout` field in workflow definitions)
- Redis-backed distributed rate limiting (`storage: "redis"` in limiter config)
- JWT support for RSA (RS256/384/512) and ECDSA (ES256/384/512) algorithms
- Request ID propagation from HTTP middleware through workflow execution and logs
- Secrets redaction in trace events and log output
- Idempotency middleware with Redis backend
- Health check timeout (default 5s) to prevent hung service checks
- `gosec` security linter with tuned exclusion rules
- CHANGELOG.md
- `server.trust_proxy` — trusted-proxy support so `c.IP()` (rate limiting, session IPs) sees the real client behind a reverse proxy (#300)
- numeric `server.*` settings (`port`, `body_limit`, `expression_memory_budget`) accept `{{ $env('NAME') }}` strings (#301)
- `noda.DecodeInto` in the Go PDK — typed decode for `Command.Data`/`ClientMessage.Data`/`IncomingWSMsg.Data`; both example guests use it (#294)
- CI now compiles every example wasm guest module with tinygo, so PDK/ABI changes can't silently break them (#296)
- `hmac_verify(data, key, algorithm, signature)` expression function — constant-time webhook signature verification.
- Node cookbook (`examples/node-cookbook/`): runnable per-family example projects verified end-to-end in CI by a new `verify.json` harness (`internal/testing/cookbook`). Tranche 1 covers the control, transform, response, util, and workflow families (20 node types).
- Node cookbook tranche 2: db, cache, storage, upload, image, and email families (24 node types) verified against real Postgres/Redis/Mailpit containers; harness gains dependency provisioning, migrations, seeded storage, multipart requests, and Mailpit inbox assertions.
- Node cookbook tranche 3: events, realtime, http, and wasm families (8 node types) verified end-to-end with real WebSocket/SSE test clients and worker/wasm runtimes in the harness (cumulative 52/81 node types covered).
- Node cookbook tranche 4: auth and oidc families (11 node types) verified against real Postgres and a [Dex](https://dexidp.io/) OIDC provider container, including a real authorization-code exchange (`oidc.exchange`) (cumulative 63/81 node types covered).
- Node cookbook tranche 5 (final): livekit family (18 node types) verified against a real LiveKit dev-server container; Runnable-example links added to all 81 node docs pages; CI coverage gate (`TestCookbookCoverage`) enforces every node type ships a cookbook example. Node cookbook complete at 81/81 node types covered.
- Cross-instance WebSocket/SSE delivery via `sync.pubsub` is now implemented (#363).
- `$ref` resolves bare JSON Schema files under `schemas/` by filename (`schemas/greeting.json` → `schemas/greeting`), alongside the existing named-definitions convention; unresolved-`$ref` errors now list every registered ref and the naming rule (#373).
- MCP & AI Agents guide (`docs/04-guides/mcp-and-ai-agents.md`) — client setup for Claude Code and other MCP clients, all 12 tools with parameters, the 10 doc resources and 2 URI templates, a recommended tool-ordering loop, and the limitations that bite (no MCP tool runs tests; schemas come from the compiled binary).
- `noda init` now scaffolds `.mcp.json`, wiring the Noda MCP server into new projects out of the box.

### Changed
- Caller-triggerable SQL failures now map to typed errors instead of a blanket 500 (#403). A new `internal/dberr` package classifies Postgres SQLSTATEs and SQLite extended result codes: constraint violations become 409/422, the `22xxx` data-exception family (invalid input syntax, numeric value out of range, datetime format/overflow, string truncation) becomes 422, `08xxx` connection exceptions become 503, serialization failures and deadlocks 503, statement timeouts 504. **Behavior change:** endpoints that previously returned 500 for these now return 4xx/503/504, so a caller treating any 5xx as retryable will see different statuses. Class 42 errors (undefined column/table/function) deliberately stay 500 — those are author bugs, not caller faults. Unmapped codes are unchanged.
- `plugins/auth`'s own SQL failures are now classified like `plugins/db`'s (#418). Serialization failures and deadlocks become 503, statement timeouts 504, data exceptions 422, and constraint violations 409 (for example, `auth.create_session` or `auth.create_token` with a `user_id` that no longer exists), where every auth node previously returned a blanket 500. `internal/dberr.ClassifyOr` is now exported and shared by both plugins. **Behavior change:** a caller treating any 5xx from an auth route as retryable will see different statuses. Node output edges are unchanged — `create_user` still returns `"exists"` for a unique violation (it branches on `IsUniqueViolation`, not `Classify`, so a foreign-key violation cannot be misreported as "email already registered"), and `verify_credentials` still returns `"invalid"` for an unknown email with unchanged timing.
- Session-authenticated routes now return **503** (or **504** on a statement timeout) when the database is unavailable during session validation, instead of 500 (#418). `internal/server/session_middleware.go` previously mapped every `AuthenticateSession` failure to a blanket 500. Conflict and validation errors deliberately stay 500 on this path: a session lookup is a `SELECT` on a hashed token, so neither is reachable by a caller. Response bodies remain static and never render driver detail — this path has no dev-mode gate.
- `api.ValidationError`, `api.ConflictError`, and `api.TimeoutError` gain an optional `Cause` field and `Unwrap` method, matching `api.ServiceUnavailableError` (#403). Additive for existing *keyed* struct literals (`&api.ConflictError{Resource: "users", Reason: "dup"}`), which are unaffected. A third-party plugin using an unkeyed literal (`&api.ConflictError{"users", "dup"}`) will fail to compile, and `==` comparisons on these values now also compare `Cause`. `TimeoutError.Error()` deliberately does not render the cause, because it is sent to prod clients unconditionally.
- The workflow `error`-edge payload for `db.create`/`db.upsert` conflicts now includes driver detail (e.g. constraint and column names), because `ConflictError.Error()` now renders `Cause` and `internal/engine/dispatch.go` puts `execErr.Error()` into node output on the `error` edge. This channel is **not** dev-gated: a workflow that interpolates a node's error text (e.g. `{{ nodes.create.error }}`) into a client-facing response can now surface constraint/column names that it previously would not have. Every other `db.*` error already carried driver text on this channel — conflicts were the sole exception, and that exception is now gone.
- Production code adopts modern Go stdlib idioms across 31 files (#410, tranche 1 of 3): `maps.Copy` (15), `slices.Contains` (14), `min`/`max` (8), `strings.SplitSeq` (5), `strings.Cut`/`CutPrefix`/`Contains` (6), `range n` (3), `slices.Backward` (2), and a `strings.Builder` in the migration generator. All rewrites are behavior-preserving and were produced by `modernize -fix`; the `WaitGroup.Go` and `omitempty` findings are deliberately excluded and handled separately.
- Test code adopts modern Go stdlib idioms across 35 files (#410, tranche 2 of 3): `range n` (57), `t.Context()` (9), Go 1.22 loopvar copy removal (6), `new(x)` for pointer-to-value helpers (5), `strings.SplitSeq`/`CutPrefix` (4), `atomic.Int32` (2), plus `maps.Copy` and `slices.Contains`. No production code is touched. All rewrites are behavior-preserving and were produced by `modernize -fix`; the 7 test-side `WaitGroup.Go` findings are deliberately excluded and handled separately.
- `modernize` is now enforced in CI via `.golangci.yml` (#410, tranche 3 of 3). This is what keeps the tranche 1–2 cleanup from drifting back: the analyzer was never wired into golangci-lint, so its findings only ever surfaced via gopls in-editor and the count silently climbed to 148. Also completes the two categories held back for individual review — the 9 `WaitGroup.Go` rewrites (all 9 were the canonical `Add(1)` + `defer Done()` shape, so `wg.Go` is exactly equivalent) and the 2 ineffective `omitempty` tags on non-pointer struct fields in the cookbook verifier, which become `omitzero`.
- **Breaking:** minimum Go version raised to **1.26** (from 1.25). The root module, `pdk/go`, and all four example Wasm guest modules now declare `go 1.26.0` with `toolchain go1.26.5`; the Docker builder moves to `golang:1.26-bookworm`. Go 1.25 leaves security support as soon as Go 1.27 ships (1.27rc2 is already out), so this gets ahead of that. Building Noda or a Go plugin against it now requires Go 1.26+.
- CI's TinyGo pin moves 0.40.1 → 0.41.1, required by the Go 1.26 bump: TinyGo added Go 1.26 support in 0.41.0, and 0.40.1 hard-fails with `requires go version 1.19 through 1.25, got go1.26`. 0.41.1 (not 0.41.0) is used because 0.41.0 shipped a `net` module regression fixed the next day.
- **Breaking:** livekit node types renamed to snake_case for consistency with every other plugin (`lk` prefix unchanged, `lk.token` unchanged). There are no aliases — old names now fail validation as unknown node types. Full mapping:

  | Old | New |
  |---|---|
  | `lk.roomCreate` | `lk.room_create` |
  | `lk.roomList` | `lk.room_list` |
  | `lk.roomDelete` | `lk.room_delete` |
  | `lk.roomUpdateMetadata` | `lk.room_update_metadata` |
  | `lk.participantGet` | `lk.participant_get` |
  | `lk.participantList` | `lk.participant_list` |
  | `lk.participantRemove` | `lk.participant_remove` |
  | `lk.participantUpdate` | `lk.participant_update` |
  | `lk.muteTrack` | `lk.mute_track` |
  | `lk.sendData` | `lk.send_data` |
  | `lk.ingressCreate` | `lk.ingress_create` |
  | `lk.ingressDelete` | `lk.ingress_delete` |
  | `lk.ingressList` | `lk.ingress_list` |
  | `lk.egressStartRoomComposite` | `lk.egress_start_room_composite` |
  | `lk.egressStartTrack` | `lk.egress_start_track` |
  | `lk.egressStop` | `lk.egress_stop` |
  | `lk.egressList` | `lk.egress_list` |
- Built-in plugin list consolidated into `plugins/all`; runtime, MCP server, and the ServiceConfigSchema audit consume one source (#384).
- Validation now rejects workflow edges whose `output` names an undeclared node output (boot already did; validate/editor/MCP now agree) (#379).
- Service configs are now validated against each plugin's declared schema on every surface (validate/boot/editor/MCP/hot-reload) — was: `valid: true` for configs whose plugin would refuse to boot (#376).
- Dev-mode hot reload now runs the same dry-run startup validation as boot/validate/editor and refuses the swap on failure (emits `file:error`) — was: node-config violations hot-reloaded "successfully" (#349). Editor per-file validation scopes dry-run errors to the saved file — was: unrelated workflows' errors shown with empty file attribution.
- http.post/http.request `body` now deep-resolves nested expression templates like sse.send/ws.send/event.emit — was: maps/slices passed through verbatim with `{{ … }}` text unevaluated (#364).
- Typed node errors (ValidationError, NotFoundError, …) now map to their HTTP statuses even when no error edge is wired — was: generic 500 INTERNAL_ERROR (#361).
- Inbound trigger header keys are now lowercase (previously fasthttp-canonical, e.g. `X-Github-Event`). Constant-key lookups like `{{ headers['X-GitHub-Event'] }}` are compile-time normalized and keep working in any case; expressions that iterate the headers map or use dynamic keys now see/need lowercase keys.
- `noda validate` and server startup now validate every workflow node's `config` against the node's ConfigSchema: missing required fields, wrong types, and unknown top-level fields are errors. Expression values (`{{ … }}`) satisfy any declared type (#332). **Upgrade note:** validation errors name the workflow, node, and field; configs newly rejected by this check were already broken or silently ignored at runtime, so fixing the named field is the complete upgrade path.
- Node ConfigSchemas audited against executor behavior across all plugins; `required` lists and types now reflect what executors actually accept (improves editor forms and MCP guidance).
- New `trigger.coerce` route option (default `true`) disables trigger-input numeric coercion per route. Literal and computed trigger-input values are no longer coerced. **Migration note:** computed defaults like `{{ query.page ?? '1' }}` now arrive as strings — switch to a bare reference (`{{ query.page }}`) or wrap numeric consumers in `toInt(...)`.
- invalid `server.*` scalar values (bad numbers, malformed durations, invalid trust_proxy entries) now fail config validation/startup instead of silently falling back to defaults
- `lk.token` now errors on invalid `canPublishSources` (unknown names, non-string entries, non-array values) instead of silently minting a token with wrong publish permissions (#309)
- the wasm module's outstandingCalls invariant is now structural (`tryAddOutstanding`), not comment-enforced (#295)
- wasm: a guest shutdown export calling trigger_workflow now gets a "module stopping" error instead of silently spawning a doomed workflow run against an already-cancelled context (#295)
- config validation now rejects route triggers whose `files` entries lack a matching `trigger.input` key — configs that previously booted with silently-broken uploads fail `noda validate` (#302)
- `lk.participantUpdate` with empty `permissions: {}` no longer performs a GetParticipant + Permission full-replace round-trip (#292)
- Wasm runtime hardening (tranche A) — **BREAKING (guest ABI):** host calls now return a `{ok,data,error}` envelope decoded by the PDK into `HostError`; rebuild guest modules against the updated PDK. Guest execution is now interruptible; default 16 MiB memory cap.
- Route-group middleware now resolves **deterministically**: overlapping group prefixes (e.g. `/api` and `/api/admin`) **merge** their middleware (outermost-first, deduped) instead of one winning at random, and prefix matching is path-segment aware (`/api` no longer matches `/api-docs`). **Upgrade note:** a config that nested groups with a cross-group ordering conflict (e.g. a parent group placing `casbin.enforce` before a child group's `auth.jwt`) previously booted non-deterministically but now fails fast at route registration with a clear ordering error — reorder the affected groups to fix.
- Dockerfile: non-root user, HEALTHCHECK directive, embedded editor build, version metadata via ldflags
- WebSocket/SSE connections are now gracefully closed during shutdown
- The committed `testdata/auth` fixture is regenerated from the current auth templates (verification-first register, constant-time pads, atomic reset) and a new drift-guard test fails whenever the templates change without a fixture regeneration; the auth engine e2e now exercises the hardened flows.
- Worker reaper polls at `retry.min_idle / 2` (30s floor) instead of a fixed 30s, and fetches delivery counts for each reclaimed page in one `XPENDING` call instead of one per failed message — fewer idle Redis scans, same redelivery semantics
- wildcard channel matching is removed from the connection manager entirely — Send/SendSSE reject pattern channels at the chokepoint (all production callers already rejected them; the Manager-level wildcard delivery capability was unreachable and is deleted) (#279)
- scheduled job runs record job history entries for same-instance overlap skips (`skipped` with a new `SkipReason: "overlap"` distinguishing them from `SkipReason: "lock"` distributed-lock skips) (#284)
- the worker's per-message timeout is applied once (runtime-owned); the `worker.timeout` middleware keeps its config name but is now the panic-to-error shield only (#285)
- Int-typed node config fields (db.find limit/offset, upload.handle max_size, image dimensions, …) now accept numeric strings — `{{ query.limit ?? '20' }}`-style computed defaults work without `toInt(...)` (#340)
- The editor validate endpoints and MCP noda_validate_config now run the same dry-run startup validation as noda validate, so they report node-config and reference errors they previously passed (#345).
- `api.PubSubService` now includes `Subscribe` — custom services satisfying the old Publish-only shape must add it (#363).
- `connections` `sync` block is now optional — was: schema-required while unused.
- Multipart repeated form values now normalize to `[]any` like urlencoded — was: `[]string`, which broke `control.loop` and type-switched expressions (#350).
- `noda init` and `noda_scaffold_project` now generate a unique 32-byte `JWT_SECRET` into `.env` — was: a shared 23-byte placeholder that failed auth.jwt's own minimum at boot (#381).
- A schema file with a top-level `properties`/`items` and no `type`/`$schema`/`$ref`/`enum`/`oneOf`/`anyOf`/`allOf` is now rejected as ambiguous instead of being silently assumed to be a bare schema — add `"type"` to disambiguate. **Upgrade note:** a project relying on the old assumption (e.g. `{"properties": {...}, "required": [...]}` with no `"type"`) now fails validation at boot (#405)
- **Breaking:** `/openapi.json` and `/docs` are no longer exposed by default. Add a `server.openapi` block with `"enabled": true` to serve them. New options: `docs` (default `true`, serves the Scalar docs UI; only takes effect when `enabled` is `true`), `path` (default `/openapi.json`), `docs_path` (default `/docs`); `path` and `docs_path` must each start with `/` and must differ from each other. The runtime and the editor's OpenAPI tab now share one generator (OpenAPI 3.1.0); the editor tab shows a notice when exposure is off. **Upgrade note:** a deployment that relied on `/openapi.json`/`/docs` being reachable must add `server.openapi: { "enabled": true }` to `noda.json`.

### Fixed
- Three SQLite driver conditions were left unclassified and returned a generic 500 where Postgres returned a 4xx (#418): a duplicate explicit `rowid` (`SQLITE_CONSTRAINT_ROWID`), a non-integer value in an `INTEGER PRIMARY KEY` (`SQLITE_MISMATCH`), and a wrong-typed value in a **STRICT** table (`SQLITE_CONSTRAINT_DATATYPE`). The first now returns 409 like any other unique violation, the other two 422 like Postgres `22P02`. `dberr.IsUniqueViolation` also recognises the rowid case, restoring parity with the message-matching helper it replaced in #403.
- SQLite unique-constraint violations returned a generic 500 instead of 409 (#403). The db plugin detected conflicts by matching the message text for lowercase `"unique constraint"`, but SQLite emits `"UNIQUE constraint failed"` — so only Postgres matched, and only by accident of wording. Detection is now by driver error code on both drivers.
- `db.update` had no conflict handling at all, so a unique violation returned 500 from `db.update` while the identical violation returned 409 from `db.create` (#403).
- The OpenAPI generator no longer emits a spurious `"default": {"description": ""}` response on every operation. `openapi3.NewResponses()` pre-seeds a match-all `default` entry, which leaked into all output and — because the container was therefore never empty — made the intended `200`/`Success` fallback unreachable, so routes declaring no responses were documented with only that empty `default` instead of a `200` (#408)
- Schema `$ref` name collisions are rejected instead of silently resolving to a nondeterministic winner — two definitions registering the same ref name (e.g. two files in one directory sharing a top-level key) previously collapsed to whichever one Go's randomized map iteration wrote last, so a route could validate against a different schema on each boot (#405)
- A named-definitions schema file whose definition names collide with JSON Schema keywords (`{"type": {...}}`) is no longer misread as a bare schema document, which silently discarded every definition in the file (#405)
- Trigger input coercion is now lossless-only: all-digit strings are converted to numbers only when the number re-formats to the identical string, so `"007"`, `"1.50"`, and 64-digit tokens reach workflows as strings instead of being mangled to `7`, `1.5`, and `1e+64` (#398). Previously such values bound as numbers against text columns and produced opaque 500s.
- Go tooling no longer sweeps in code vendored inside `editor/node_modules` (the npm package `flatted` ships a Go implementation under the module root). `make test`/`make test-coverage`, `golangci-lint`, and `govulncheck` all now exclude it. That third-party 0%-covered package skewed local coverage ~0.9pp below CI's figure; CI avoided it only by accident of step ordering (these steps run before `make build` triggers `npm install`), so a reordering would have silently changed lint, vuln, and coverage results. Local and CI now both report 81.5%.
- `noda init` declared the MCP server in `.claude/settings.json`, which has no `mcpServers` key — the config was inert, so scaffolded projects silently started with no Noda MCP server. The server is now declared in `.mcp.json` at the project root (the only project-scoped location Claude Code reads), and `.claude/settings.json` carries `enableAllProjectMcpServers` to auto-approve it.
- The `node error with no error edge` warning now includes the node's error text — previously the only server-side record of why such a workflow 500'd was an opaque INTERNAL_ERROR (#396).
- Cross-instance connection sync no longer corrupts binary (non-UTF-8) WebSocket/SSE payloads: they ride base64-encoded in the envelope and arrive byte-exact on remote instances. All sync envelopes are now version 2; v1 envelopes are dropped, so all instances in a cluster must run the same Noda version (#372).
- Docs described a `schemas/File#Key` `$ref` syntax that never resolved; corrected to the real `schemas/<Key>` rule across docs and the MCP crud example (#373).
- A top-level `connections` key in `noda.json` is now rejected with a pointer to the `connections/*.json` convention; previously the root schema advertised it while the runtime silently ignored it. ws.send/sse.send endpoint crossref errors also state when no connections endpoints are defined anywhere (#380).
- `noda validate` (and MCP/editor validation) now errors on `services.*` entries whose `plugin` name is unknown, even when no node references the service (#385).
- db/storage service schemas accept an explicit empty `driver`/`backend` string, matching the parsers' treat-empty-as-default behavior (#386).
- `response.file` now accepts a string `data` value (sent as-is), matching its documented contract; previously only `[]byte` was accepted and strings errored.
- Trigger bodies with non-lowercase multipart Content-Type (e.g. MULTIPART/FORM-DATA) now parse via a manual fallback; previously they fell through to a raw string (#339).
- Trigger inputs sourced from JSON bodies keep their JSON types; numeric coercion now applies only to bare references into string-typed transports (path params, query, headers, form bodies) (#331).
- `parseBody` now recognizes form/JSON `Content-Type` values regardless of case (previously only exact-lowercase matches parsed; anything else fell through to a raw string), and duplicate urlencoded keys (`a=1&a=2`) now yield an array of values instead of silently keeping only the last one (#331).
- `storage.write` returns `{"path": ...}` in its success output as its descriptor and docs promise, instead of an empty map (#333)
- email plugin parses string `port` values (the shape `$env()` substitution produces) instead of silently dialing 587; unparseable or out-of-range ports now fail service creation loudly (#334)
- the MCP server and the workflow test runner's node registry now include the auth plugin's 8 node types (`auth.*`), previously invisible to `noda_list_nodes`/`noda_get_node_schema` and `noda test` (#327)
- workflow test assertions can target intermediate (non-terminal) node outputs — the test runner now retains all outputs instead of reading already-evicted ones (#329)
- unmocked `response.json` output is navigable in workflow tests: `api.HTTPResponse`/`api.Cookie` carry lowercase snake_case json tags and the test runner normalizes struct outputs to maps, so dot paths like `resp.body.email` and lowercase partial-match keys work (#330). Tests that matched the old capitalized field names (`Body`, `Status`) must switch to lowercase.
- homebase: `GET /drops` returns 400 (not a Postgres-cast 500) on a malformed `before` cursor; pagination gains a `(created_at, id)` tuple cursor (`before_id`/`next_before_id`) so same-timestamp rows can't be skipped (#303)
- homebase: concurrent `/setup` can no longer create two accounts — single-row unique index on `auth_users` (#304)
- homebase: Caddy moved to a `docker-compose.edge.yml` override; an unset `DOMAIN` fails at parse time again instead of an opaque ACME error (#305)
- `examples/saas-backend` upload-attachment route never delivered the multipart file (missing `"file"` input mapping) (#302)
- `wasm.query` no longer burns its full timeout when the module is stopping (shutdown/devmode reload) — fails fast with a stopping error (#293)
- wasm gateway checks the outbound-WS whitelist before the duplicate-connection-id check (fail closed on permission first) (#265)
- `lk.token` `canPublishSources` values are now case-insensitive; unknown values (including `UNKNOWN`) error instead of silently minting a token that cannot publish (#309)
- Worker process no longer crashes when a message handler panics inside the timeout middleware's goroutine; the panic is recovered and surfaced as an error
- Worker consumers survive a panic in pre-handler setup (deserialization, input mapping, middleware construction) instead of permanently losing a consumer goroutine
- Wasm gateway reconnect no longer resurrects a torn-down outbound WebSocket when a close races with an in-flight reconnect; also fixed a data race between the heartbeat loop and reconnect's reassignment of the connection stop channel
- WebSocket broadcast no longer head-of-line-blocks: each connection has a bounded outbound queue with a write deadline, so one slow client can't stall the whole channel
- Data race in workflow test runner trace callback (concurrent map access from parallel nodes)
- Health endpoint documentation now matches actual paths (`/health/live`, `/health/ready`, `/health`)
- Deployment docs corrected for `sampling_rate` config field name
- Worker now reclaims idle pending messages via XAutoClaim, so failed messages are actually redelivered and dead-letter (`dead_letter.after`) and retry limits are enforced (previously pending messages were never re-processed).
- Worker pre-handler panics are now retried and dead-lettered/dropped through the normal disposition instead of being stranded in the pending-entries list (#243); panic errors now include a stack trace.
- Lifecycle/devmode hardening: a shutdown signal received during startup is now honored (stops what started, aborts the rest) instead of being swallowed until a second signal; dev-mode config reloads are serialized (the latest change wins) and awaited at shutdown so no reload callback fires into a closing system; the dev-mode file watcher now reacts to config files created under new subdirectories and to config-file deletions; a service whose creation times out is properly shut down via its plugin if it completes late (no leaked connection pool).
- New worker `retry` config (`min_idle`, `max_attempts`); without a `dead_letter` topic a repeatedly-failing message is dropped with a loud error after `max_attempts`.
- Worker ack/dead-letter disposition runs on a fresh context, so a message that fails by exhausting the handler timeout is still counted, dead-lettered, or dropped instead of retrying forever.
- Worker `dead_letter.after` defaults to `retry.max_attempts` when omitted, so a topic-only `dead_letter` config dead-letters poison messages instead of silently dropping them; an empty `dead_letter.topic` disables dead-lettering with an ERROR log instead of publishing to a stream named `""`.
- Legacy worker `retry.dlq` (documented pre-reclaim) is honored as an alias for `dead_letter.topic` instead of being silently ignored.
- Worker `min_idle` is clamped to `timeout` + 30s margin (not exactly `timeout`), so the reaper cannot reclaim a message whose consumer is still finishing or acknowledging.
- Worker reaper processes reclaimed messages with the worker's `concurrency` instead of serially, so one slow poison message no longer head-of-line-blocks redelivery.
- Worker/scheduler hardening: a worker's configured `timeout` is honored by the middleware chain (no longer capped by a shared default); the pending-message reaper claims only as many messages as it processes concurrently (closing a duplicate-execution window under contention); sub-minute schedules with distributed locking get a per-second lock key (no longer skip fires within a minute); a scheduled job that runs longer than its interval skips overlapping same-instance runs instead of self-overlapping.
- Scaffold/runtime alignment: generated route triggers now run (added a `request.*` alias to the route context and switched generators to canonical `params`/`body`); `noda auth init` and `noda migrate` accept both `db` and `postgres` plugin names; generated multi-tenant CRUD scopes by the URL path param (no cross-tenant write via request body); MCP example configs are valid (`util.jwt_sign` secret/expiry, `$ref` object form); `noda init` and the MCP scaffold refuse to overwrite existing files (`noda init --force` to override); strict expression mode accepts `$item`/`$index`.
- Engine execution safety: a workflow that times out now returns a `504`/error instead of silently reporting success on a truncated run; parallel branches failing with different error types no longer crash the process; starved AND-joins fail loudly; join classification is deterministic; alias/node-ID and duplicate workflow-ID collisions are rejected at load.
- `lk.participantUpdate` now merges `permissions` with the participant's current permission set (one extra `GetParticipant` call) instead of full-replacing it — a partial map like `{"canPublish": false}` no longer silently revokes `canSubscribe`/`canPublishData`/`hidden`. Unknown or non-boolean permission keys are now rejected instead of silently ignored.
- `auth.set_password` gained an optional `token` config that consumes a `reset_password` one-time token and updates the password in a single transaction (new `invalid` output); the scaffolded reset-password flow uses it, so a failure after token consumption (rejected password, DB error) no longer burns the reset token. Password length validation now counts characters (runes) instead of bytes, matching the scaffolded route schemas' code-point semantics.
- Quick-wins batch: wasm gateway reconnection settings are honored under msgpack encoding (`max_attempts`/`initial_delay` no longer silently coerce to zero); `wasm.send` during module shutdown can no longer trip the Go WaitGroup Add/Wait misuse panic (commands to a stopping module are dropped with a warning); the route handler's response select is now deterministic: a response the workflow already produced wins over a synthesized workflow error or response timeout, a workflow completing exactly at the response deadline gets its real outcome instead of a coin-flip 504, and a workflow error suppressed by a produced response is always logged (previously a scheduling race could return 500/504 despite a produced response); the wasm-counter and discord-bot example guest modules compile under tinygo again; the dev `/ws/trace` origin check compares hostnames case-insensitively.
- Scheduler distributed locking now keys each fire on the cron-scheduled tick time instead of the wall clock at dispatch, so two instances handling the same tick but straddling a second boundary (GC pause, load) can no longer compute different keys and both run the job.
- sub-workflow timeouts inherited from a parent deadline report the child's actual budget instead of "timeout after 0s" (#273); any TimeoutError without a duration now omits it from the message (previously "after 0s")
- `{{ request.raw_body }}` now mirrors `{{ raw_body }}` on the request alias (#275)
- dev-mode shutdown no longer waits unboundedly for a stuck in-flight reload — bounded by the lifecycle stop budget (#287)
- SSE connections now flush headers and an initial `: connected` comment immediately on connect; previously no bytes reached the client until the first event or heartbeat (up to 30s).
- Strict expression mode now admits the transport namespaces used by trigger mappings (`body`, `query`, `params`, `headers`, `request`, `raw_body`, `method`, `path`, `message`, `schedule`) in `knownContextEnv` (#354)
- `noda test` now evaluates `secrets.*` expressions: `RunTestSuite` takes a `secretsCtx` param, the CLI passes the loaded `SecretsManager`'s expression context, and the dev-mode editor test-run endpoint is wired through as well (#355)
- The headers-patcher now preserves source location on patched keys (previously lost, breaking error line/column reporting); `hmac_verify` accepts uppercase `<ALGORITHM>=` signature prefixes, not just lowercase (#356)
- `examples/saas-backend` GitHub sync example: the issue id is now a string (matching GitHub's payload) and routes to the correct landing-zone project (#357)
- `workflow.output` docs now describe the real success/error routing (the parent's `workflow.run` routes any non-`"error"` name through its `success` port; the name is available to the parent as data, not as a separate port); the dead `setOutputs` reference was removed (#358)
- Servers built without `WithWorkflowCache` now get a working `subWorkflowRunner` sourced from `Setup`'s self-built workflow cache, instead of silently failing to run sub-workflows (#359)
- Wasm modules that were loaded but never started are now closed on `Stop`; a partial multi-module load now unloads the modules it already loaded before failing (#365)
- OpenAPI generation now resolves `$ref` schema references correctly — components are keyed by ref name (`schemas/User` → `#/components/schemas/User`) instead of file path, so `$ref`s in request/response bodies no longer dangle when a schema file's name differs from its ref name.

### Security
- Edge & trace hardening: DB conflict/unavailable error bodies no longer leak driver/constraint detail in production (detail gated behind dev mode); trace redaction now covers slice-typed node data (e.g. `db.query` rows) and `stream_key`; the dev `/ws/trace` endpoint rejects cross-origin connections; `response.redirect` rejects `/\`-authority open redirects; `ws.send`/`sse.send` (and the Wasm host connection API) reject wildcard channels — **broadcasting via a wildcard send is no longer supported; subscribe connections to a shared literal channel instead**; `image.resize`/`crop`/`thumbnail` cap output dimensions.
- Bumped `github.com/buger/jsonparser` v1.1.1 → v1.1.2 (GO-2026-4514, DoS in the parser; the package is imported transitively but the vulnerable symbol is not called) and `golang.org/x/crypto` v0.51.0 → v0.53.0 (clears 13 module-level `ssh/*` advisories; `golang.org/x/crypto/ssh` is not imported, so there was no call-path exposure — `argon2`/`bcrypt` used by auth are unchanged). `govulncheck` reports no vulnerabilities.
- Bumped `golang.org/x/text` v0.38.0 → v0.39.0 (GO-2026-5970, infinite loop on invalid input in `golang.org/x/text/unicode/norm`). Unlike the bumps above this advisory **was** call-reachable: `govulncheck` traced it through `gorm`/`database/sql` (`db.Plugin.HealthCheck` → `sql.DB.Ping`) and the websocket dialer (`wasm.Gateway.Connect`), so a malformed UTF-8 DSN or URL could hang a goroutine. `golang.org/x/mod` moves v0.36.0 → v0.37.0 as a transitive consequence. No source changes; the `go` and `toolchain` directives are unchanged.
- Auth scaffold anti-enumeration: `noda auth init` now generates a **verification-first** register flow — both a new and an already-registered email return an identical `200` with no session cookie and send an email, so registration no longer discloses which addresses exist (it no longer auto-logs-in; users verify then log in). The password-reset and resend-verification flows now respond at a **fixed ~500 ms deadline** on every branch (via `util.timestamp` + `util.delay`), so the synchronous SMTP send on the known-account path no longer leaks account existence (or verified-vs-unverified status) through response timing. For a hard guarantee, move the email send to an async worker (`event.emit` + a worker consumer). Also: `util.delay` now resolves its `timeout` per request, enabling computed delays.
- The scaffolded login flow now pads invalid-credential responses to a fixed ~500 ms deadline (`util.timestamp` + `util.delay`, same pattern as password-reset/resend-verification), closing a timing oracle that re-opened account enumeration after an argon2 cost raise: stored hashes verify at their embedded old params while unknown emails burn the new, heavier dummy hash, so wrong-password-on-real-account responded measurably faster than unknown-email. Projects scaffolded earlier should add the pad manually (see the authentication guide); if argon2 verification alone approaches 500 ms, raise the deadline.
- The trace redactor now fails closed in both situations where it cannot classify keys: past its recursion depth cap (`[REDACTED: max depth]`) and for non-string-keyed maps (`[REDACTED: unclassifiable keys]`), instead of returning the raw value — a secret nested deeper than 32 levels or inside e.g. a `map[int]any` can no longer bypass redaction.

### Removed
- Stream plugin consume-side API (`Subscribe`, `Ack`, `PendingCount`): unused by the platform (workers consume streams directly) and its hardcoded 60s reclaim conflicted with the worker reaper's timeout-clamped policy. `Publish` and the `pkg/api.StreamService` contract are unchanged.
- **BREAKING:** the `slim` Docker image variant and the `-full` tag suffix. There is now one image — `debian:bookworm-slim` with libvips, built with cgo — published under unsuffixed tags (`latest`, `0.0.8`, `0.0`, `0`). Existing `-full` tags remain in GHCR but receive no new versions; pullers of the unsuffixed tag get the former `-full` image, which is larger (≈119 MB vs ≈36 MB compressed on amd64) and no longer distroless. It also runs as uid 999 (`noda`) rather than uid 65532 (`nonroot`), so bind mounts and named volumes chowned to 65532 must be re-chowned (see #426). In exchange, `image.*` nodes now work in every image. The slim variant built with `CGO_ENABLED=0` and had not compiled since `internal/dberr` landed (#425).
- **BREAKING:** the prebuilt Windows release binary. It was the last `CGO_ENABLED=0` build target and was broken by the same cgo dependency. On Windows, use the Docker image or build from source with libvips — see the [installation guide](docs/01-getting-started/installation.md#windows).
- The `noimage` build tag. With one build configuration there is nothing to gate: the image plugin is now an unconditional member of the plugin list, and libvips is a hard requirement to build from source.
- `.goreleaser.yaml`. It was referenced by no workflow, Makefile, or script — `release.yml` is and remains the release path — and described a target set that no longer matches what ships.

## [1.0.0] - 2026-03-18

### Added
- Configuration-driven API runtime with JSON config files
- 72 built-in node types across 15 categories
- Visual editor (React + React Flow) with embedded serving
- Workflow engine with parallel execution, retry, and eviction
- Expression language with 50+ built-in functions
- Plugin system: PostgreSQL, Redis (cache/streams/pubsub), HTTP, email, storage, image processing
- WebSocket and SSE real-time connection management
- Worker runtime with Redis Streams consumer groups and dead letter queue
- Scheduler runtime with cron expressions and distributed locking
- Wasm module runtime via Extism (wazero)
- JWT, OIDC, Casbin RBAC, CORS, CSRF, rate limiting, helmet middleware
- OpenTelemetry distributed tracing with OTLP export
- Database migration management with up/down SQL files
- OpenAPI 3.1.0 spec generation from config
- Graceful lifecycle management with ordered startup/shutdown
- Dev mode with hot reload and live trace WebSocket
- Load testing with k6 scenarios
- 80.8% test coverage across all packages
- LiveKit integration for real-time audio/video
- MCP server integration
