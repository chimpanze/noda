# Plan: docverify code-bug tranche (issues #327, #329, #330, #333, #334)

Date: 2026-07-16 · Branch: `fix/docverify-code-bugs` · Source: docs-verification campaign 2026-07-15

Five well-scoped code bugs from the campaign. The two design-heavy ones (#331 coerceNumeric,
#332 ConfigSchema enforcement) are deliberately **not** in this tranche.

## Task 1 — #333 storage.write returns empty map

`plugins/core/storage/write.go:83` returns `map[string]any{}` while `OutputDescriptions()["success"]`
promises "Object with path of the written file".

**Fix:** return `map[string]any{"path": path}`.
**Test:** success-path test asserting output contains `path` == written path.

## Task 2 — #334 email plugin ignores string port

`plugins/email/plugin.go:32-37` only accepts float64/int; `$env()` substitution produces strings,
so `"port": "{{ $env('SMTP_PORT') }}"` silently dials 587.

**Fix:** accept float64/int/int64; for strings do strict `strconv.Atoi(strings.TrimSpace(v))`
and return a hard error on unparseable/empty values (silent fallback is the hazard). Validate
range 1–65535. Key absent → default 587 unchanged.
**Audit result:** only other numeric-config assertion in service plugins is http `timeout`
(float64|string) — its string path parses as duration and errors loudly, not silently. No other
`port` fields exist. No further changes.
**Tests:** string port "1025" → 1025; "  465 " trimmed; "abc"/"" → error; absent → 587;
useTLS derives from string-sourced 465.

## Task 3 — #327 MCP + test-runner registries miss auth plugin

Runtime registers nodes from ALL plugins (`internal/registry/bootstrap.go:45`), including
`serviceOnlyPlugins()` — so auth.* works in production. But `internal/mcp/plugins.go#corePlugins()`
and `cmd/noda/main.go#corePlugins()` (feeds `buildCoreNodeRegistry` → workflow test runner)
both omit auth → MCP discovery and test-runner registry miss 8 auth.* node types.

**Fix:** add `plugins/auth` to `internal/mcp/plugins.go` corePlugins(); move `authplugin` from
`serviceOnlyPlugins()` to `corePlugins()` in `cmd/noda/main.go` (comment on serviceOnlyPlugins —
"provide services but no nodes" — stays true: stream/pubsub/storage). Consistent with db/cache/email,
which are service-backed node plugins already in corePlugins.
**Tests:** MCP test asserting `auth.create_user` (and count 8 of auth.*) present in node registry;
cmd-level: registerCorePlugins still registers every plugin exactly once (no duplicate-prefix error).

## Task 4 — #329 test runner reads evicted outputs

Engine evicts non-terminal outputs as soon as consumers finish (`executor.go:210`,
`eviction.go`); test runner collects outputs only after the workflow ends
(`internal/testing/runner.go:300`), so assertions on intermediate nodes read nothing.

**Fix:** add `retainOutputs` flag to `ExecutionContextImpl` + `WithRetainOutputs()` option;
`EvictOutput` becomes a no-op when set. Test runner passes the option. Engine behavior in
production unchanged.
**Tests:** engine: EvictOutput no-ops under WithRetainOutputs; runner: workflow A→B where
assertion targets intermediate node A's output — fails before fix, passes after.

## Task 5 — #330 response.json output not navigable in tests

`api.HTTPResponse`/`api.Cookie` have no json tags → normalizeToMap yields capitalized keys;
`extractPath` has no struct fallback → dot paths never match.

**Fix:** (a) add lowercase snake_case json tags to `api.HTTPResponse` (`status`, `headers`,
`cookies`, `body`) and `api.Cookie` (`name`, `value`, `path`, `domain`, `max_age`, `secure`,
`http_only`, `same_site`) — matches the canonical trace shape (`redactHTTPResponse`) and the
node's own config schema; (b) in the test runner's `collectOutputs`, normalize non-map outputs
via JSON round-trip to `map[string]any` (keep raw value if round-trip fails or yields non-map).
Production storage/expressions untouched (normalization is test-runner-only).
**Impact check:** nothing outside the matcher marshals HTTPResponse (server writes fields
directly; trace uses redactHTTPResponse's hand-built map).
**Tests:** runner test with real response.json node — dot-path `resp.body.x` and `resp.status`
assertions pass; partial-match `outputs` sees lowercase keys.

## Gates

Per task: `go test ./<pkg>/...`. Before PR: `gofmt -l .` from repo root (must be empty),
`go vet ./...`, `golangci-lint run`, full `go test ./...`, CHANGELOG [Unreleased] entries.
One commit per task, PR closes #327 #329 #330 #333 #334.
