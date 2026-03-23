# Noda Code Review Findings

**Date:** 2026-03-23
**Scope:** All 365 non-test Go source files across `cmd/`, `pkg/`, `internal/`, `plugins/`
**Result:** `go vet ./...` passes clean

---

## Critical / High Severity

### Bugs

| # | Location | Description |
|---|----------|-------------|
| 1 | `internal/engine/parse.go:10-66` | **Workflow `timeout` field never parsed from config.** `ParseWorkflowFromMap` never reads `raw["timeout"]`, so configured workflow timeouts are silently ignored in production. Tests set `Timeout` directly on the struct, masking this. |
| 2 | `internal/engine/retry.go:59-64` | **Retry swallows hard errors.** `dispatchNode` errors (panics, unknown node types, missing services) trigger `continue` instead of returning, causing infinite retry loops on structural failures. |
| 3 | `internal/server/editor_codegen.go:86-88` | **Nil dereference in `listModels`.** `resolvedConfig()` can return nil; accessing `rc.Models` would panic. |
| 4 | `internal/server/editor_nodes.go:161` | **Nil dereference in `listSchemas`.** Same pattern — no nil guard before `rc.Schemas`. |
| 5 | `plugins/http/request.go:161` | **Unbounded `io.ReadAll` on HTTP response.** No size limit — a malicious upstream can cause OOM. Should use `io.LimitReader`. |
| 6 | `internal/generate/migration.go:454-470,544` | **SQL injection via `OnDelete` value.** Config value interpolated directly into generated SQL without validation. |
| 7 | `internal/generate/migration.go:389,423,499` | **SQL injection via `Default` column value.** Same class of issue — config values spliced into SQL without quoting. |
| 8 | `internal/wasm/gateway.go:177-206` | **Heartbeat goroutine leak.** Calling `Configure` multiple times starts new `heartbeatLoop` goroutines without stopping old ones. |
| 9 | `internal/devmode/watcher.go:89,113-119` | **Data race on `lastPath`.** Variable captured by `time.AfterFunc` closure by reference while being written concurrently. Fix: capture by value. |
| 10 | `plugins/core/oidc/refresh.go:113-119` | **Silent ID token verification failure.** Refreshed token verification errors are swallowed — security concern. `exchange.go` correctly returns these errors. |

### Security

| # | Location | Description |
|---|----------|-------------|
| 11 | `cmd/noda/main.go:708-711` | **Path traversal in dev-mode editor.** `strings.HasPrefix(absPath, editorDist)` without trailing separator allows accessing sibling directories like `editor/dist-evil/`. |
| 12 | `plugins/db/where.go:237-254` | **Missing `ValidateSQLFragment` on `having` map branch.** Object-format `having` query string not validated, unlike the string branch. |
| 13 | `plugins/db/where.go:25-57` | **`where_clause.query` not validated.** Expression-resolved query template embedded in SQL without `ValidateSQLFragment` check. |

---

## Medium Severity

### Bugs

| # | Location | Description |
|---|----------|-------------|
| 14 | `internal/registry/bootstrap.go:115` | **"Startup validation passed" logged on failure.** Log fires unconditionally before the error check. |
| 15 | `internal/registry/lifecycle.go:62-78` | **Goroutine leak on service creation timeout.** Late-returning `CreateService` goroutine is never cleaned up. |
| 16 | `internal/registry/lifecycle.go:132-146` | **Goroutine leak on service shutdown timeout.** Same pattern. |
| 17 | `internal/config/crossrefs.go:404-409` | **Potential infinite loop in cycle detection.** If `parent` chain is broken, `for cur != next` never terminates. |
| 18 | `internal/server/trigger.go:29-31` | **`X-Request-Id` trace ID silently overwritten.** `MapTrigger` sets trace ID from header, but `buildRouteHandler` always overwrites it. |
| 19 | `internal/server/editor_codegen.go:412-419` | **Optional param bug in `convertPath`.** `:param?` becomes `{param?}` instead of `{param}` in editor OpenAPI spec. |
| 20 | `internal/mcp/plugins.go:26-45` | **MCP plugin list out of sync.** Missing OIDC and LiveKit plugins. |
| 21 | `internal/generate/migration.go:279-285` | **`columnChanged` ignores precision/scale/enum changes.** No migration generated for these column changes. |
| 22 | `internal/trace/redact.go:24-38` | **`redactSecrets` doesn't recurse into slices.** Sensitive values in arrays leak to trace WebSocket. |
| 23 | `plugins/livekit/plugin.go:80` | **Health check with no timeout.** `context.Background()` — hangs indefinitely if LiveKit is unreachable. |
| 24 | `plugins/image/watermark.go:113-128` | **Negative watermark positions.** When watermark is larger than image, negative offsets could crash bimg. |
| 25 | `plugins/stream/service.go:72-84` | **Stream `Subscribe` never ACKs messages.** Messages stay in PEL forever; pending messages from crashed consumers are lost. |
| 26 | `internal/wasm/gateway.go:314-366` | **Reconnect race with `CloseAll`.** Reconnected connection can be orphaned with no cleanup path. |
| 27 | `cmd/noda/main.go:1222-1229` | **Worker middleware only uses first worker's config.** Other workers silently get wrong middleware. |

---

## Dead Code

| # | Location | Description |
|---|----------|-------------|
| 28 | `internal/registry/plugins.go:129-138` | `Prefixes()` — only used in tests |
| 29 | `internal/registry/services.go:94-108` | `ByPrefix()` — only used in tests |
| 30 | `internal/registry/services.go:118-125` | `Order()` — only used in tests |
| 31 | `internal/registry/services.go:55-64` | `GetWithPlugin()` — only used in tests |
| 32 | `internal/expr/compiler.go:171-185` | `CompileAll` — only used in tests |
| 33 | `internal/expr/functions.go:205-207` | `Register` (without info) — only used in tests |
| 34 | `internal/scheduler/runtime.go:160-171` | `NextRun()` — only used in tests |
| 35 | `internal/scheduler/runtime.go:147-156` | `History()` — only used in tests |
| 36 | `internal/trace/events.go:100` | `SubscriberCount()` — only used in tests |
| 37 | `internal/secrets/resolve.go:13` | `EnvPattern()` — only used in tests |
| 38 | `internal/secrets/manager.go:55-62` | `Keys()` — only used in tests |
| 39 | `internal/connmgr/endpoint.go:14` | `EndpointService.endpoint` field — stored but never read |
| 40 | `internal/connmgr/manager.go:298-302` | `GetConnection()` — only used in tests |
| 41 | `internal/wasm/runtime.go:37` | `DefaultMaxModuleSize` — exported but only used internally |
| 42 | `internal/generate/migration.go:78` | `migrationsDir` parameter — accepted but never used |
| 43 | `internal/generate/crud.go:70-73` | `opSet` check — always true, dead logic |
| 44 | `internal/server/response.go:9` | `ResponseInterceptKey` constant — never referenced |
| 45 | `internal/server/openapi.go:232,270` | `rc` parameter unused in `addRequestBody` and `addResponses` |
| 46 | `plugins/db/helpers.go` | Empty file — only `package db` |
| 47 | `plugins/livekit/service.go:17-19` | `NewAuthProvider()` — only used in tests |
| 48 | `plugins/core/workflow/run.go:77-79` | `SetOutputs` — only used in tests |

---

## Quality / Consistency

| # | Location | Description |
|---|----------|-------------|
| 49 | `cmd/noda/main.go` | **Heavy duplication** between `newStartCmd` and `newDevCmd` (~500 lines of near-identical logic). |
| 50 | `cmd/noda/main.go` | **Tracing error handling asymmetry:** warning in start, fatal in dev (opposite of typical intent). |
| 51 | `internal/config/validator.go:75-94` | **Schema compiler re-created per file.** Reads/compiles schema from embedded FS on every validation call. Should cache. |
| 52 | `internal/config/validator.go:144-153` | `joinPath` reimplements `strings.Join`. |
| 53 | `internal/config/crossrefs.go:476-485` | `formatCycle` reimplements `strings.Join`. |
| 54 | `internal/expr/compiler.go:78` | **Comment says "LRU" but eviction is FIFO.** |
| 55 | `internal/plugin/truthy.go:12-28` | **Missing numeric types.** `int32(0)` would be truthy. |
| 56 | `internal/plugin/resolve.go:219-260` | **Inconsistent return signatures.** `ResolveOptionalMap`/`Array` return 2 values, others return 3. |
| 57 | `internal/server/` | **Duplicate OpenAPI generation.** Two separate implementations (`openapi.go` vs `editor_codegen.go`) producing different OpenAPI versions. |
| 58 | `internal/server/routes.go:45` | **Non-deterministic route registration** from map iteration. |
| 59 | `plugins/core/control/` + `workflow/` | **Duplicated interfaces** (`depthTracker`, `SubWorkflowRunner`) across packages. |
| 60 | `plugins/core/response/json.go:179-213` | **Duplicate `resolveDeep`** with divergent depth protection (100-deep limit here, none in `internal/plugin`). |
| 61 | `plugins/core/control/if.go:48-51` | **Misleading output descriptions.** Says "input data passed through" but returns the condition result. |
| 62 | Various plugins | **5 core plugins missing `core.` prefix** in `Name()`: event, upload, ws, sse, wasm. |
| 63 | Various plugins | **3 different config resolution patterns** used across plugins (should standardize on `internal/plugin.ResolveString`). |
| 64 | `pkg/api/errors.go:50` | `NotFoundError.Error()` produces trailing `: ` when `ID` is empty. |
| 65 | `internal/engine/dispatch.go:19-24` | **Panic recovery loses stack trace.** Should capture `runtime/debug.Stack()`. |
| 66 | `internal/engine/compiler.go:345` | **Unused `nodeID` parameter** in `hasCommonConditionalAncestor`. |
| 67 | `internal/wasm/tick.go:10` vs `module.go:109` | **Tick interval inconsistency.** Two different formulas for the same calculation produce different values for non-divisor tick rates. |
| 68 | `internal/config/refs.go:110` | **`append(seen, refName)` may mutate caller's slice** if backing array has spare capacity. |
| 69 | `internal/generate/migration.go:592` | **Comment says "Kahn's algorithm" but implementation is DFS.** |
| 70 | `internal/generate/crud.go:228-251` | **Non-deterministic column order** from map iteration produces unstable output. |
