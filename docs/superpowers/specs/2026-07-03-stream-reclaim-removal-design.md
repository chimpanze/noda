# Stream Plugin Consume-Side API Removal — Design

Date: 2026-07-03
Status: approved
Origin: deferred fast-follow from the PR #244 review (duplicate XAutoClaim reclaim implementations).

## Problem

`plugins/stream/service.go` carries a consume-side API — `Subscribe`, `Ack`,
`PendingCount`, plus the internal `reclaimPending` / `dispatchMessage` helpers
and the `reclaimInterval` / `reclaimMinIdle` constants — that duplicates the
worker runtime's stream consumption. Two independent XAutoClaim reclaim
implementations now exist with contradictory policies:

- The worker reaper (since #244) reclaims at `retry.min_idle`, clamped to at
  least the handler timeout + 30s margin, so it can never steal a message from
  a live consumer.
- `Subscribe`'s reclaim uses a hardcoded 60-second min-idle. If a `Subscribe`
  consumer ever shared a worker's topic + group, it would steal messages from
  live worker handlers (default timeout 5m) at 60s idle — exactly the failure
  the worker-side clamp exists to prevent.

Exploration found that the consume-side API has **no production callers**:

- `pkg/api.StreamService` (the stable plugin contract) contains only `Publish`.
- The worker runtime consumes streams directly through the raw Redis client
  (`plugin.RedisClientProvider` → `Client()`).
- The Wasm host dispatcher (`internal/wasm/hostapi.go` `dispatchStream`)
  supports only `emit`/`publish`.
- `event.emit` uses only `Publish`.
- The only callers are the plugin's own tests.

The Subscribe reclaim was added in the 2026-04-23 runtime-hardening tranche
(finding H10) to fix pending-message loss for `Subscribe` consumers; PR #244's
worker reaper supersedes that mechanism for the platform's actual consumption
path.

## Decision

Remove the consume-side API from the stream plugin rather than parameterizing
or policy-aligning it. Rejected alternatives:

- **Parameterize & share** (make min-idle a `Subscribe` parameter; extract a
  shared XAutoClaim pager for `Subscribe` and the worker reaper): preserves an
  unused capability at the cost of a new shared package and continued API
  surface. YAGNI.
- **Align policy only** (raise the constant to match the worker default):
  leaves the duplication and drift risk in place; does not close the finding.

## Changes

### `plugins/stream/service.go`

Remove: `Subscribe`, `Ack`, `PendingCount`, `reclaimPending`,
`dispatchMessage`, `reclaimInterval`, `reclaimMinIdle`, and any imports left
unused (`log/slog`).

Keep: `Service`, `Publish`, `Client()` (the worker runtime depends on it via
`plugin.RedisClientProvider`), and the `var _ api.StreamService = (*Service)(nil)`
assertion. `pkg/api` is untouched — no plugin-contract change.

### `plugins/stream/plugin_test.go`

Delete the tests that exercise `Subscribe` / `Ack` / `PendingCount` /
reclaim. Publish and service-creation tests remain the package's coverage.

### Docs

- `docs/04-guides/observability.md`: replace the "Stream consumer
  auto-reclaim" section (documents the 60s `stream.Subscribe` reclaim) with a
  short section pointing to the worker runtime's reclaim (`retry.min_idle`,
  `dead_letter`) in `docs/02-config/workers.md`.
- `docs/04-guides/plugin-development.md`: drop `Ack` from the `StreamService`
  interface example so it matches `pkg/api` (`Publish` only).
- The 2026-04-23 spec/plan documents stay untouched (point-in-time records).

### `CHANGELOG.md`

"Removed" entry: the stream plugin's unused consume-side API, superseded by
the worker runtime's reclaim.

## Risk & compatibility

- `api.StreamService` is unchanged, so plugin authors coding against `pkg/api`
  are unaffected.
- Only code importing the concrete `plugins/stream.Service` for the removed
  methods would break at compile time; nothing in the repo does.
- Package coverage stays healthy: the removed code's tests are removed with it.

## Verification

`go build ./...`; `go test ./plugins/stream/ ./internal/worker/ -race`;
`go vet ./...`; full `go test ./...` before the PR.
