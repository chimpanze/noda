# Worker Pending-Reclaim & Poison-Message Disposition — Design

Date: 2026-07-01
Issue: #243 (Worker: pre-handler panic strands message in PEL, bypassing dead-letter)
Scope: `internal/worker/` (+ its config schema and docs)

## Problem

The worker consumer reads only new stream entries (`XReadGroup` with `">"`) and
has **no pending-reclaim path** — there is no `XClaim`, `XAutoClaim`, or `"0"`
re-read anywhere in `internal/worker/`. As a result:

1. **No redelivery.** Any message left pending is never re-processed in-process.
   Both failure paths that "leave the message pending for redelivery" (the
   `wfErr` non-ack path and the panic-recover path added in #242) are therefore
   dead ends — the message sits in the pending-entries list (PEL) until the
   process exits, and nothing ever claims it.
2. **Dead-letter is effectively inert for `after > 1`.** `getDeliveryAttempts`
   reads the pending entry's `RetryCount`, which only increments on redelivery or
   claim. With neither happening, `RetryCount` stays at `1`, so
   `attempts >= dead_letter.after` fires only when `after <= 1`.
3. **Poison messages strand (the #243 symptom).** A payload that panics in
   pre-handler setup (deserialize, `engine.ResolveInput`, `NewExecutionContext`,
   middleware `Wrap`) is caught by the top-level `recover()`, logged, and then
   `processMessage` returns without acking and without the dead-letter check —
   growing the PEL unboundedly and silently defeating any configured
   dead-letter.

#243 is one symptom of the deeper gap: the worker's retry/dead-letter story
assumes a reclaim step that was never built. This design adds that step and
routes all failure classes through one disposition.

## Decisions (locked during brainstorming)

- **Scope: root fix.** Add pending reclaim so retry and dead-letter actually
  work, and route panics through the same retry → dead-letter flow.
- **No-DLQ policy: hard cap → drop + loud error.** When a message keeps failing
  and no `dead_letter` is configured, drop it after a hard `max_attempts` cap and
  emit an ERROR instructing the operator to configure `dead_letter` to retain
  poison messages. `dead_letter`, when set, diverts before the cap.
- **Reclaim architecture: one dedicated reaper goroutine per worker** (not folded
  into the per-consumer read loop). Keeps the hot `XReadGroup` path untouched and
  avoids redundant claims across the N consumers.
- **No backward-compatibility constraints.** The project is pre-release with no
  external configs in the wild; config semantics may change freely. The design
  states the target shape directly.

## Config

New optional `retry` block on each worker; the existing `dead_letter` block is
unchanged in shape but its `after` becomes functional.

```jsonc
{
  "subscribe":   { "topic": "orders", "group": "fulfillment" },
  "timeout":     "5m",                       // existing: per-message handler timeout
  "retry":       { "min_idle": "5m", "max_attempts": 10 },
  "dead_letter": { "topic": "orders.dlq", "after": 5 }
}
```

Fields:

- `retry.min_idle` (duration string) — a pending entry must be idle at least this
  long before the reaper reclaims it. **Correctness constraint:** `min_idle` must
  be `>=` the handler `timeout`, otherwise the reaper can steal a message a live
  consumer is still legitimately processing, causing double execution. Default:
  the worker's `timeout` (with a floor of `60s`). If configured below the
  timeout, it is clamped up to the timeout and a WARN is logged at parse time.
- `retry.max_attempts` (int) — hard cap on delivery attempts. When
  `dead_letter` does not divert first, the message is ack-dropped once
  `attempts >= max_attempts`, with an ERROR log. Default: `10`.
- `dead_letter.topic` / `dead_letter.after` (existing) — when set,
  `attempts >= after` diverts the message to the dead-letter topic (and acks the
  original). `after` should be `<= max_attempts` to take effect; this is not
  enforced but documented.

Struct changes in `WorkerConfig`:

```go
type WorkerConfig struct {
    // ... existing fields ...
    DeadLetter *DeadLetterConfig
    Retry      RetryConfig   // new; zero value resolved to defaults at parse
}

type RetryConfig struct {
    MinIdle     time.Duration
    MaxAttempts int
}
```

`ParseWorkerConfigs` gains parsing of the `retry` block (mirroring the existing
`timeout` duration and `dead_letter` parsing), resolves defaults, and applies the
`min_idle >= timeout` clamp with a WARN.

## Reaper goroutine

`Start()` spawns one `reap(ctx, w, client)` per worker, alongside the
`Concurrency` `consume` goroutines, tracked by the same `wg`.

```
reap(ctx, w, client):
  ticker := interval derived from min_idle (e.g. min(min_idle, 30s), floor 5s)
  for each tick (until ctx done):
    cursor := "0"
    loop:
      msgs, nextCursor := XAutoClaim(topic, group, consumerID="reaper",
                                     minIdle, start=cursor, count=N)
      for msg in msgs:
        processMessage(ctx, w, client, "reaper", msg)   // same disposition
      cursor = nextCursor
      if cursor == "0": break        // full pass complete
```

- `XAutoClaim` is atomic and idle-gated, so it never steals in-flight work and
  never double-dispatches across a concurrent reaper.
- Uses the `opCtx` snapshot like `processMessage` does, so in-flight reclaimed
  work finishes within the shutdown deadline.
- `XAutoClaim` errors never kill the reaper: log and retry on the next tick.
- Reclaimed messages whose entries have been ack'd/deleted meanwhile are skipped
  by Redis automatically (returned in the deleted-IDs set, which we ignore).

## Unified disposition (the #243 fix)

Restructure `processMessage` so panic and error share one exit path. The
deserialize → input-map → context-build → handler sequence moves into an inner
function returning `error`, with a `recover()` that converts a panic into an
error **capturing `debug.Stack()`** (also closing the review's finding that the
timeout-path recover dropped the stack). A single disposition switch then runs on
the outcome:

| Outcome | Action |
|---|---|
| `nil` (success) | `XAck` |
| deterministic bad input-mapping | `XAck`-drop; if `dead_letter` set, copy to DLQ first (forensics) |
| `wfErr` **or** recovered panic | resolve `attempts` via `getDeliveryAttempts`; then: **if** `dead_letter` set and `attempts >= after` → `moveToDeadLetter` (acks); **else if** `attempts >= max_attempts` → `XAck`-drop + ERROR (`"configure dead_letter to retain poison messages"`); **else** leave pending (reaper retries after `min_idle`) |

Notes:
- Input-mapping failures remain a deterministic drop (retrying can't help), now
  with an optional DLQ copy for forensics rather than a silent ack.
- A panic during a *reclaimed* reprocess is caught by the same inner recover and
  flows through the same switch — so a truly poison payload is retried up to the
  threshold and then dead-lettered or dropped, never stranded.
- The bare top-level `recover()` from #242 is subsumed by the inner recover; a
  thin outer safety recover may remain as a last resort but should no longer be
  the primary panic handler.

## Error handling & shutdown

- Reaper honors `ctx` cancellation (read-loop ctx) and uses the `opCtx` swap for
  processing, matching the consumer contract: `Stop` drains in-flight reclaimed
  work within the shutdown deadline.
- All Redis errors in the reaper are logged and retried on the next tick; the
  reaper goroutine never exits on a transient error, only on ctx cancellation.

## Testing

Integration (real Redis via `internal/testing/containers`, matching existing
worker integration tests):

1. A pending message (left unacked by a transient `wfErr`) is reclaimed and
   reprocessed after `min_idle`, then acked on success.
2. A transient `wfErr` that succeeds on the second attempt is retried via reclaim
   and acked — no dead-letter, no drop.
3. Poison panic **with** `dead_letter`: retried up to `after`, then dead-lettered
   (message on DLQ topic carries original payload + error; original acked).
4. Poison panic **without** `dead_letter`: dropped + ERROR logged after
   `max_attempts`; PEL returns to empty.
5. Reaper does **not** steal a message a slow-but-live handler is still
   processing (`min_idle` floor / `>= timeout` guarantee).

Unit:

6. Table-driven disposition function over `{outcome, attempts, dead_letter?,
   max_attempts}` → expected action (`ack` / `dead-letter` / `drop` / `pending`).
7. `ParseWorkerConfigs`: `retry` defaults resolve correctly; `min_idle < timeout`
   is clamped up with a WARN; absent `retry` yields `min_idle = timeout` (floor
   60s), `max_attempts = 10`.

## Files touched

- `internal/worker/runtime.go` — `WorkerConfig`/`RetryConfig`, `Start` (spawn
  reaper), new `reap`, restructured `processMessage` disposition, config parsing.
- `internal/worker/runtime_test.go` (+ possibly a new `reclaim_test.go`) — tests
  above.
- `internal/config/schemas/worker.json` — add the `retry` block schema.
- `docs/02-config/workers.md` — document `retry`, the `min_idle >= timeout`
  constraint, the functional `dead_letter.after`, and the no-DLQ drop behavior.
- `CHANGELOG.md` — Fixed/Changed entries.

## Out of scope

- Interruptible/idempotent handler execution (at-least-once delivery means a
  reclaimed message may re-run a partially-applied workflow; deduplication is the
  workflow author's responsibility and unchanged by this work).
- Per-attempt backoff beyond the `min_idle` idle gate.
