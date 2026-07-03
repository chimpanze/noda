# Stream Plugin Consume-Side API Removal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the stream plugin's unused consume-side API (`Subscribe`, `Ack`, `PendingCount` and its 60s XAutoClaim reclaim), which duplicates and conflicts with the worker runtime's reclaim.

**Architecture:** Pure removal. `plugins/stream.Service` keeps `Publish` and `Client()` (the worker runtime's access path via `plugin.RedisClientProvider`); the stable `pkg/api.StreamService` interface (`Publish` only) is untouched. Docs that described the removed reclaim point to the worker runtime's reclaim instead.

**Tech Stack:** Go, go-redis/v9, miniredis (existing tests).

**Spec:** `docs/superpowers/specs/2026-07-03-stream-reclaim-removal-design.md`

## Global Constraints

- `pkg/api` interfaces are stable — no changes to `pkg/api/services.go`.
- `Client()` must remain on `plugins/stream.Service` (worker runtime depends on it).
- Historical docs under `docs/superpowers/` from 2026-04-23 stay untouched.
- All commits end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.

---

### Task 1: Remove consume-side code and its tests

**Files:**
- Modify: `plugins/stream/service.go` (remove lines: the `reclaimInterval`/`reclaimMinIdle` const block at 14-24, `dispatchMessage` at 26-35, `reclaimPending` at 37-56, `Subscribe` at 84-163, `Ack` at 165-172, `PendingCount` at 174-181; prune now-unused imports)
- Modify: `plugins/stream/plugin_test.go` (remove `TestService_PublishAndSubscribe`, `TestService_Ack`, `TestService_ConsumerGroupAutoCreation`, `TestService_SubscribeCancellation`, `TestService_Subscribe_ReclaimsAbandonedMessages`; keep `TestPlugin_*`, `newTestService`, `TestService_ImplementsStreamService`, `TestService_Publish`; prune now-unused imports)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `plugins/stream.Service` with exactly `Publish(ctx, topic string, payload any) (string, error)`, `Client() *redis.Client`, and the `var _ api.StreamService = (*Service)(nil)` assertion. Task 3 verifies the repo builds against this reduced surface.

Removal inverts TDD: the deleted code's tests go first, then the code; the gate is that the build fails while dead references remain and the remaining suite passes when done.

- [ ] **Step 1: Delete the five consume-side tests**

In `plugins/stream/plugin_test.go`, delete the entire functions `TestService_PublishAndSubscribe` (lines 82-119), `TestService_Ack` (121-151), `TestService_ConsumerGroupAutoCreation` (153-173), `TestService_SubscribeCancellation` (175-192), and `TestService_Subscribe_ReclaimsAbandonedMessages` (194-end). Keep everything above line 82 intact.

- [ ] **Step 2: Verify the package still compiles and remaining tests pass (code not yet removed — removal is safe to stage test-first)**

Run: `go test ./plugins/stream/ -v 2>&1 | grep -E "^(--- |ok|FAIL)"`
Expected: PASS for `TestPlugin_Metadata`, `TestPlugin_CreateService_MissingURL`, `TestPlugin_CreateService_Success`, `TestPlugin_HealthCheck`, `TestPlugin_Shutdown`, `TestService_ImplementsStreamService`, `TestService_Publish`; `ok` for the package. If imports in the test file became unused, the build fails naming them — remove exactly those imports and re-run.

- [ ] **Step 3: Delete the consume-side production code**

In `plugins/stream/service.go` delete, in this order (top to bottom):
1. The whole `const (...)` block containing `reclaimInterval` and `reclaimMinIdle` (lines 14-24).
2. `dispatchMessage` (lines 26-35).
3. `reclaimPending` (lines 37-56).
4. `Subscribe` including its doc comment (lines 84-163).
5. `Ack` (lines 165-172).
6. `PendingCount` (lines 174-181).

Then prune imports that the compiler reports as unused (expected: `log/slog`, `time`; `encoding/json` stays for `Publish`, `fmt` stays for error wrapping).

- [ ] **Step 4: Verify build, tests, and vet**

Run: `go build ./... && go test ./plugins/stream/ ./internal/worker/ -race && go vet ./plugins/stream/`
Expected: build OK, both packages `ok`, vet silent. Also run `grep -rn "\.Subscribe(\|PendingCount\|reclaimPending\|dispatchMessage" --include="*.go" . | grep -v pubsub | grep -v "hub\.Subscribe"` — expected: no matches (proves no dangling references).

- [ ] **Step 5: Commit**

```bash
git add plugins/stream/service.go plugins/stream/plugin_test.go
git commit -m "refactor(stream): remove unused consume-side API (Subscribe/Ack/PendingCount)

The worker runtime is the platform's only stream consumer and (since #244)
has the sole XAutoClaim reclaim, policy-clamped to the handler timeout.
The plugin's parallel 60s reclaim had no production callers and would steal
live worker messages if pointed at a shared group. pkg/api.StreamService
(Publish only) is unchanged.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task 2: Update docs and CHANGELOG

**Files:**
- Modify: `docs/04-guides/observability.md` (the "Stream consumers" section, lines 57-78)
- Modify: `docs/04-guides/plugin-development.md` (the `StreamService` example, lines 387-391)
- Modify: `CHANGELOG.md` (Unreleased)

**Interfaces:**
- Consumes: the reduced `plugins/stream.Service` surface from Task 1 (docs must not mention removed methods).
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Replace the observability.md auto-reclaim section**

Replace everything from `## Stream consumers` through the end of the section (currently `### Stream consumer auto-reclaim` and its four paragraphs, lines 57-78) with:

```markdown
## Stream consumers

Workers are the platform's stream consumers. A per-worker reaper reclaims
pending messages from crashed or timed-out consumers via `XAUTOCLAIM` once
they have been idle longer than `retry.min_idle` (clamped to the handler
`timeout` plus a 30-second safety margin), then retries, dead-letters, or
drops them according to the worker's `retry` and `dead_letter` config. See
[Workers](../02-config/workers.md) for the full disposition rules.
```

- [ ] **Step 2: Fix the plugin-development.md StreamService example**

In `docs/04-guides/plugin-development.md`, change:

```go
// StreamService allows event streaming
type StreamService interface {
    Publish(ctx context.Context, topic string, payload map[string]any) (string, error)
    Ack(ctx context.Context, topic, group, id string) error
}
```

to (matching `pkg/api/services.go`):

```go
// StreamService allows event streaming
type StreamService interface {
    Publish(ctx context.Context, topic string, payload any) (string, error) // returns message ID
}
```

- [ ] **Step 3: Add CHANGELOG entry**

In `CHANGELOG.md` under `## [Unreleased]`, add a `### Removed` section (after `### Fixed`) if absent, with:

```markdown
### Removed
- Stream plugin consume-side API (`Subscribe`, `Ack`, `PendingCount`): unused by the platform (workers consume streams directly) and its hardcoded 60s reclaim conflicted with the worker reaper's timeout-clamped policy. `Publish` and the `pkg/api.StreamService` contract are unchanged.
```

- [ ] **Step 4: Verify docs contain no stale references**

Run: `grep -rn "stream.Subscribe\|PendingCount" docs/01-getting-started docs/02-config docs/03-nodes docs/04-guides docs/05-examples README.md`
Expected: no matches.

- [ ] **Step 5: Commit**

```bash
git add docs/04-guides/observability.md docs/04-guides/plugin-development.md CHANGELOG.md
git commit -m "docs: stream consumption/reclaim is the worker runtime's job

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

### Task 3: Full verification and PR

**Files:**
- None created/modified (verification only; PR creation).

**Interfaces:**
- Consumes: the completed Tasks 1-2 commits on `feat/stream-reclaim-removal`.
- Produces: an open PR against `main`.

- [ ] **Step 1: Full suite**

Run: `go build ./... && go test ./... 2>&1 | grep -v "^ok\|no test files\|^ld: warning" ; go vet ./...`
Expected: no test failures printed (only the empty grep result), vet silent. Known flake: `TestEmailSend_Engine` in `plugins/email` (Mailpit container startup race) — unrelated to this change; re-run once if it fails.

- [ ] **Step 2: Push and open PR**

```bash
git push -u origin feat/stream-reclaim-removal
gh pr create --title "refactor(stream): remove unused consume-side API (fast-follow to #244)" --body "$(cat <<'EOF'
Fast-follow to #244 (deferred review finding: duplicate XAutoClaim reclaim implementations).

## Problem

`plugins/stream.Service` carried `Subscribe`/`Ack`/`PendingCount` with a hardcoded 60s `XAUTOCLAIM` reclaim. No production code calls them — `pkg/api.StreamService` is `Publish`-only, the worker runtime consumes streams via the raw Redis client, the Wasm host and `event.emit` only publish. If a `Subscribe` consumer ever shared a worker's topic+group, its 60s reclaim would steal messages from live worker handlers (default timeout 5m) — the exact failure the worker-side `min_idle` clamp prevents.

## Change

Removed the consume-side API and its tests; kept `Publish`, `Client()`, and the `api.StreamService` assertion. Docs now point at the worker runtime's reclaim (`retry.min_idle`, `dead_letter`); the `plugin-development.md` `StreamService` example matches `pkg/api` again. No `pkg/api` changes.

Spec: `docs/superpowers/specs/2026-07-03-stream-reclaim-removal-design.md`

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR URL printed.

- [ ] **Step 3: Watch CI**

Run: `gh pr checks --watch --fail-fast`
Expected: all checks pass.
