# Fix Plans for Code Review Findings

Findings are split into 4 fix plans by priority and risk. Each plan is a self-contained PR.

---

## Plan A: Critical Bug & Security Fixes

**Scope:** Findings #1-13 (10 bugs + 3 security issues)
**Risk:** Low — targeted fixes, no refactoring
**Estimated files changed:** ~12

### A1. Workflow timeout never parsed (#1)
- **File:** `internal/engine/parse.go`
- **Fix:** Add `wf.Timeout = MapStrVal(raw, "timeout")` after line 13
- **Test:** Add test case in `parse_test.go` with `"timeout": "5s"`, verify `wf.Timeout == "5s"`

### A2. Retry swallows hard errors (#2)
- **File:** `internal/engine/retry.go`
- **Fix:** Replace `continue` at line 63 with `return "", execErr`
- **Test:** Add test case where `dispatchNode` returns a structural error, verify retry returns immediately

### A3. Nil dereference in `listModels` (#3)
- **File:** `internal/server/editor_codegen.go`
- **Fix:** Add nil guard after line 86: `if rc == nil { return c.Status(500).JSON(map[string]any{"error": "no config available"}) }`

### A4. Nil dereference in `listSchemas` (#4)
- **File:** `internal/server/editor_nodes.go`
- **Fix:** Add same nil guard after line 161

### A5. Unbounded HTTP response read (#5)
- **File:** `plugins/http/request.go`
- **Fix:** Replace `io.ReadAll(resp.Body)` with `io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))` where `maxResponseSize` is a const (e.g., 100MB). Return an error if truncated.
- **Test:** Add test with large response body

### A6. SQL injection in migration generation (#6, #7)
- **File:** `internal/generate/migration.go`
- **Fix:** Add `validateOnDelete(value string) error` that checks against `CASCADE|SET NULL|RESTRICT|NO ACTION|SET DEFAULT`. For `Default`, wrap in SQL quoting or validate against injection patterns.
- **Test:** Add test with malicious `OnDelete` and `Default` values

### A7. Heartbeat goroutine leak (#8)
- **File:** `internal/wasm/gateway.go`
- **Fix:** Add a `heartbeatCancel context.CancelFunc` field to `gatewayConn`. In `Configure`, cancel the previous heartbeat before starting a new one. In `heartbeatLoop`, select on the context.
- **Test:** Call `Configure` twice, verify only one heartbeat loop is running

### A8. Data race in watcher (#9)
- **File:** `internal/devmode/watcher.go`
- **Fix:** Capture `lastPath` by value in the closure:
  ```go
  path := lastPath
  timer = time.AfterFunc(w.debounce, func() {
      w.logger.Info("config file changed", "path", path)
      w.onChange(path)
  })
  ```

### A9. OIDC refresh silent verification failure (#10)
- **File:** `plugins/core/oidc/refresh.go`
- **Fix:** Return errors from `verifier.Verify` and `idToken.Claims` like `exchange.go` does, instead of silently ignoring them

### A10. Path traversal in dev-mode editor (#11)
- **File:** `cmd/noda/main.go`
- **Fix:** Change line 709 to: `if !strings.HasPrefix(absPath, editorDist+string(filepath.Separator))`

### A11. Missing SQL fragment validation (#12, #13)
- **File:** `plugins/db/where.go`
- **Fix:** Add `ValidateSQLFragment(queryStr)` call at line 249 (after resolving the having map query). Add same validation in `resolveWhereClause` for the `where_clause.query` field.

---

## Plan B: Medium Severity Fixes

**Scope:** Findings #14-27
**Risk:** Low-Medium — some involve concurrency patterns
**Estimated files changed:** ~14

### B1. Misleading validation log (#14)
- **File:** `internal/registry/bootstrap.go`
- **Fix:** Move `slog.Info("startup validation passed")` below the `len(allErrors) > 0` check (after line 119)

### B2. Goroutine leaks on service timeout (#15, #16)
- **File:** `internal/registry/lifecycle.go`
- **Fix:** Use `context.WithTimeout` and pass the context to `CreateService`/`Shutdown` if the `Plugin` interface supports it. If not, document the limitation and add a warning log when timeout fires.

### B3. Infinite loop in cycle detection (#17)
- **File:** `internal/config/crossrefs.go`
- **Fix:** Add a safety counter to the `for cur != next` loop: `for i := 0; cur != next && i < len(parent)+1; i++`. Log a warning if the counter hits the limit.

### B4. X-Request-Id overwritten (#18)
- **File:** `internal/server/routes.go`
- **Fix:** Check if `triggerResult.Trigger.TraceID` is already set before overwriting. If it was set from `X-Request-Id`, preserve it.

### B5. Optional param bug in convertPath (#19)
- **File:** `internal/server/editor_codegen.go`
- **Fix:** Add `name = strings.TrimSuffix(name, "?")` in `convertPath`, matching `fiberToOpenAPIPath` in `openapi.go`

### B6. MCP plugin list out of sync (#20)
- **File:** `internal/mcp/plugins.go`
- **Fix:** Add `&coreoidc.Plugin{}` and `&livekitplugin.Plugin{}` to `corePlugins()`, matching `cmd/noda/main.go`

### B7. columnChanged ignores precision/scale/enum (#21)
- **File:** `internal/generate/migration.go`
- **Fix:** Add comparisons for `Enum`, `Precision`, `Scale` in `columnChanged`

### B8. redactSecrets slice recursion (#22)
- **File:** `internal/trace/redact.go`
- **Fix:** Add a `case []any` branch that iterates and recurses into map elements within slices

### B9. LiveKit health check timeout (#23)
- **File:** `plugins/livekit/plugin.go`
- **Fix:** Replace `context.Background()` with `context.WithTimeout(context.Background(), 5*time.Second)`

### B10. Negative watermark positions (#24)
- **File:** `plugins/image/watermark.go`
- **Fix:** Clamp `Left` and `Top` to `0` in `calculatePosition`

### B11. Stream Subscribe ACK (#25)
- **File:** `plugins/stream/service.go`
- **Fix:** Add `s.client.XAck(ctx, stream, group, msg.ID).Err()` after successful handler execution. Or document that ACK is the caller's responsibility.

### B12. Gateway reconnect race (#26)
- **File:** `internal/wasm/gateway.go`
- **Fix:** Check `g.conns` map after reconnection succeeds to see if the connection was removed by `CloseAll`. If so, close the new connection.

### B13. Worker middleware first-only (#27)
- **File:** `cmd/noda/main.go`
- **Fix:** Either resolve middleware per-worker (loop through each worker's config), or document clearly that all workers share the first worker's middleware.

---

## Plan C: Dead Code Cleanup

**Scope:** Findings #28-48
**Risk:** Very low — removing unused code
**Estimated files changed:** ~15

### Test-only exports to remove or unexport:
- `registry.PluginRegistry.Prefixes()` (#28)
- `registry.ServiceRegistry.ByPrefix()` (#29)
- `registry.ServiceRegistry.Order()` (#30)
- `registry.ServiceRegistry.GetWithPlugin()` (#31)
- `expr.Compiler.CompileAll` (#32)
- `expr.FunctionRegistry.Register` (#33)
- `scheduler.Runtime.NextRun()` (#34)
- `scheduler.Runtime.History()` (#35)
- `trace.SubscriberCount()` (#36)
- `secrets.EnvPattern()` (#37)
- `secrets.Manager.Keys()` (#38)
- `connmgr.Manager.GetConnection()` (#40)
- `livekit.Service.NewAuthProvider()` (#47)
- `workflow.RunExecutor.SetOutputs` (#48)

**Strategy:** For each, check if there's a reasonable public API argument for keeping it. If it's purely a test helper, either unexport it or move the logic into the test file.

### Other dead code:
- `connmgr.EndpointService.endpoint` field (#39) — remove field
- `wasm.DefaultMaxModuleSize` (#41) — unexport to `defaultMaxModuleSize`
- `generate.GenerateMigration.migrationsDir` param (#42) — remove parameter (breaking change for callers — check usage)
- `generate.crud.go` opSet check (#43) — remove dead check
- `server.ResponseInterceptKey` (#44) — remove constant
- `server.openapi.go` unused `rc` params (#45) — remove from `addRequestBody` and `addResponses`
- `plugins/db/helpers.go` (#46) — delete empty file

---

## Plan D: Quality & Consistency Improvements

**Scope:** Findings #49-70
**Risk:** Medium — involves refactoring
**Estimated files changed:** ~25

### D1. Extract shared start/dev setup (#49, #50)
- **File:** `cmd/noda/main.go`
- **Fix:** Extract shared logic (config loading, tracing, metrics, plugin bootstrap, wasm setup, scheduler setup, lifecycle+signal handler) into helper functions. Unify tracing error handling.
- **Note:** This is the largest refactor. Consider doing it as a separate PR.

### D2. Cache schema compiler (#51)
- **File:** `internal/config/validator.go`
- **Fix:** Cache compiled schemas in a `sync.Once` or package-level map

### D3. Replace reimplemented stdlib functions (#52, #53)
- **Files:** `internal/config/validator.go`, `internal/config/crossrefs.go`
- **Fix:** Replace `joinPath` with `strings.Join(parts, "/")`, `formatCycle` with `strings.Join(ids, " -> ")`

### D4. Fix misleading comments (#54, #65, #66, #69)
- Fix "LRU" → "FIFO" in `internal/expr/compiler.go:78`
- Add `debug.Stack()` to panic recovery in `internal/engine/dispatch.go`
- Remove unused `nodeID` param in `internal/engine/compiler.go:345`
- Fix "Kahn's algorithm" → "DFS" in `internal/generate/migration.go:592`

### D5. Fix IsTruthy missing types (#55)
- **File:** `internal/plugin/truthy.go`
- **Fix:** Add `int32`, `float32`, `uint`, `uint64`, `int64` cases (or use `reflect` for all numeric types)

### D6. Consolidate duplicate code (#57, #59, #60, #63)
- Unify `resolveDeep` (add depth protection to `internal/plugin/resolve.go`, remove duplicate from `response/json.go`)
- Extract shared `depthTracker`/`SubWorkflowRunner` interfaces to `pkg/api` or `internal/plugin`
- Standardize config resolution on `internal/plugin.ResolveString` across all plugins

### D7. Fix non-deterministic map iterations (#58, #70)
- **Files:** `internal/server/routes.go`, `internal/generate/crud.go`
- **Fix:** Sort keys before iterating

### D8. Fix plugin name consistency (#62)
- **Files:** 5 plugin.go files (event, upload, ws, sse, wasm)
- **Fix:** Add `core.` prefix to `Name()` return values

### D9. Fix NotFoundError formatting (#64)
- **File:** `pkg/api/errors.go`
- **Fix:** Omit `: %s` suffix when `ID` is empty

### D10. Fix tick interval inconsistency (#67)
- **File:** `internal/wasm/tick.go`
- **Fix:** Use `time.Second / time.Duration(m.tickRate)` (matching `module.go`)

### D11. Fix slice append aliasing (#68)
- **File:** `internal/config/refs.go`
- **Fix:** Use `cycle := append([]string{}, seen...)` then `cycle = append(cycle, refName)`

### D12. Fix inconsistent ResolveOptional signatures (#56)
- **File:** `internal/plugin/resolve.go`
- **Note:** This is a breaking API change. `ResolveOptionalMap`/`ResolveOptionalArray` would need to return 3 values. Check all callers.

### D13. Fix control node output descriptions (#61)
- **Files:** `plugins/core/control/if.go`, `plugins/core/control/switch.go`
- **Fix:** Update output descriptions to accurately describe what is returned (the evaluated expression result, not the input data)

---

## Recommended Order

1. **Plan A** first — critical bugs and security fixes, ship ASAP
2. **Plan B** second — medium severity, ship within a week
3. **Plan C** third — dead code cleanup, low risk, can batch into one PR
4. **Plan D** last — quality improvements, break into 2-3 PRs:
   - D1 separately (large refactor of main.go)
   - D2-D5, D7-D11, D13 together (small targeted fixes)
   - D6, D12 separately (cross-cutting changes with more callers to update)
