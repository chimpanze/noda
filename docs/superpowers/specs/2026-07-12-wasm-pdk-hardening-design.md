# Wasm/PDK Hardening — Design

**Date:** 2026-07-12
**Issues:** #293 (Query stopping guard), #294 (pdk DecodeInto), #295 (outstandingCalls invariant), #296 (CI tinygo guest builds), #265 (gateway check order), #267 (envelope WriteBytes collapse — upstream-blocked), #268 (test polish)
**Branch:** `feat/wasm-pdk-hardening`

## Problem

Seven wasm/pdk follow-ups from the tranche-A and quick-wins reviews:

- **#293:** `Module.Query` (`internal/wasm/module.go:264`) checks only
  `m.failed` at entry; a query racing or arriving after `Stop` buffers into
  `queryCh` (cap 16) which nothing drains once the tick loop exits — the
  caller blocks for its FULL timeout during shutdown/devmode reload instead
  of failing fast like the fixed `SendCommand` path. Cannot panic (Query
  never touches `outstandingCalls`), just wastes the timeout.
- **#295:** the Add-not-concurrent-with-Wait-at-zero invariant on
  `outstandingCalls` is enforced by one guard (`SendCommand` checks
  `stopping` under `m.mu`) plus comments at the two `hostapi.go` Add sites.
  Nothing structural stops a future Add site from reintroducing the
  shutdown panic.
- **#294:** the PDK delivers `Command.Data`/`ClientMessage.Data`/
  `IncomingWSMsg.Data` as codec-decoded `any` with no typed-decode path;
  both example modules carry copy-pasted `decodeInto` helpers (the gap that
  caused #269).
- **#296:** example guest modules are compiled nowhere in CI — the next
  PDK/type change ships broken examples silently, exactly like #269.
  (`examples/wasm-helpers/wasm/helpers/main.go` is also the repo's one
  standing gofmt violation; the issue says fix it when touching this.)
- **#265:** `gateway.go Connect` checks duplicate connection id BEFORE the
  whitelist, so a non-whitelisted URL with a reused id gets
  `VALIDATION_ERROR: already in use` instead of `PERMISSION_DENIED` —
  marginal state-info leak, assessed non-exploitable (per-module gateway,
  serialized callers), fix is defense-in-depth.
- **#267:** in `runtime.go noda_call writeEnvelope`, if `p.WriteBytes` for
  the (already error-carrying) envelope itself fails, the fallback
  `stack[0]=0` reads as VOID SUCCESS in the PDK — the unsafe collapse
  direction. extism go-sdk v1.7.1 exposes no host-side SetError; offset 0 is
  the only void sentinel.
- **#268:** two test-quality items — no assertion that `m.failed` is true
  after a genuine tick timeout, and a comment in `TestHostCall_Msgpack`
  saying numbers "arrive as int64" while the assertion checks `int8`.

## Decisions (user-approved 2026-07-12)

1. **#295 gets structural encapsulation via a dedicated `addMu` mutex** (not
   documented-only): one `tryAddOutstanding()` helper used by every Add
   site; `Stop` sets `stopping` under `addMu` before `Wait`.
2. **#267: check upstream first, else document + close.** If an extism
   go-sdk newer than v1.7.1 exposes host-side SetError/equivalent, bump and
   fix properly; otherwise strengthen the `writeEnvelope` comment (naming
   the collapse direction and #267) and close as revisit-when-upstream-moves.
   No custom sentinel ABI.

## Design

### 1. Query stopping guard (#293)

`internal/wasm/module.go Query`:

- Early fast path at entry (after the `failed` check):
  `if m.stopping.Load() { return nil, fmt.Errorf("module %q stopping", m.Name) }`.
- `case <-m.stopCh:` added to BOTH selects (the enqueue select and the
  await select), returning the same "module %q stopping" error.

Test: start a module, `Stop` it, call `Query` with a generous timeout
(e.g. 30s), assert it returns promptly (< ~1s) with the stopping error —
against current code this either times out or hangs the test long enough to
fail a deadline assertion (polarity holds).

### 2. addMu encapsulation (#295)

`internal/wasm/module.go` (+ the Add sites in `internal/wasm/hostapi.go`):

- New field `addMu sync.Mutex` on `Module`, documented as a LEAF lock:
  held only for the stopping-check+Add pair; never held across guest calls,
  channel ops, or `m.mu` acquisition.
- New helper:

  ```go
  // tryAddOutstanding registers one outstanding host call unless the module
  // is stopping. It is the ONLY permitted way to Add to outstandingCalls:
  // Stop sets stopping under the same addMu before Waiting, so an Add can
  // never race Wait-at-zero (the shutdown-panic class from #266).
  func (m *Module) tryAddOutstanding() bool {
      m.addMu.Lock()
      defer m.addMu.Unlock()
      if m.stopping.Load() {
          return false
      }
      m.outstandingCalls.Add(1)
      return true
  }
  ```

- `Stop` sets `m.stopping.Store(true)` while holding `addMu` (in addition
  to / replacing the current under-`m.mu` store — the plan pins the exact
  final locking sequence after reading the current Stop body; the invariant
  is: no `tryAddOutstanding` can observe `stopping == false` after Stop's
  store completes).
- `SendCommand` and both `hostapi.go` Add sites route through
  `tryAddOutstanding()`, replacing their per-site guards/comments. Callers
  that get `false` behave as SendCommand does today when stopping (drop
  quietly / return a stopping error, matching each site's current
  semantics).
- The three per-site safety comments collapse into the helper's one; the
  comment at `Stop`'s `Wait` is updated to reference the helper.

Race-style test (`-race`): N goroutines hammering `tryAddOutstanding` (with
matching `Done()` on success) racing one `Stop`; assert no panic and that no
Add succeeds after Stop's store is observed. Must fail (panic under race /
assert) if the guard is removed.

### 3. PDK DecodeInto (#294)

`pdk/go/noda` (new function beside the codec):

```go
// DecodeInto re-encodes a codec-decoded value (e.g. Command.Data) with the
// module's active codec and unmarshals it into dst — the typed-decode path
// for Data-any fields.
func DecodeInto(v any, dst any) error
```

- Implementation: `activeCodec.Marshal(v)` then `activeCodec.Unmarshal(b, dst)`
  (codec round-trip, NOT hardcoded JSON — msgpack modules keep type
  fidelity).
- `examples/wasm-counter` and `examples/discord-bot` switch to
  `noda.DecodeInto`, deleting their local `decodeInto` copies.
- Documented in the wasm dev guide (`docs/04-guides/` — the file that
  documents guest authoring; plan names it exactly).
- PDK unit test: round-trip a struct through `DecodeInto` under BOTH codecs
  (set `activeCodec` accordingly), including a numeric-type case that would
  betray a hardcoded-JSON implementation under msgpack.
- Both examples must still `tinygo build` (Task/CI below proves it).

### 4. CI tinygo guest builds (#296)

`.github/workflows/ci.yml`: new separate job `wasm-guests`:

- Installs tinygo via the standard setup action (version pinned to a recent
  release; locally verified with tinygo 0.40.1 / go 1.25).
- Runs `tinygo build` for `examples/wasm-counter/wasm/counter`,
  `examples/discord-bot/wasm/bot`, `examples/wasm-helpers/wasm/helpers`,
  using each module's documented build invocation (check each example's
  README/Makefile for flags — e.g. `-target wasip1 -buildmode=c-shared` or
  the extism target the repo uses; the plan pins the exact commands after
  reading them).
- NON-BLOCKING by default: it is not added to the repo's required-checks
  set (promoting it later is the user's call). It should still fail red on
  breakage so it's visible.
- Drive-by (issue-sanctioned): `gofmt -w examples/wasm-helpers/wasm/helpers/main.go`
  — removes the repo's one standing gofmt violation.

### 5. Gateway check order (#265)

`internal/wasm/gateway.go Connect`: move the `isAllowed(wsURL)` whitelist
check ABOVE the duplicate-id check (fail closed on permission first).
Tests:

- The existing test that observed "already in use" via a non-whitelisted
  URL switches to a whitelisted (`AllowWS`) URL so the dup-id path stays
  covered.
- New case pinning the order: non-whitelisted URL + duplicate id →
  `PERMISSION_DENIED` (not `VALIDATION_ERROR`).

### 6. #267 resolution (upstream check, else document + close)

Plan step, in order:

1. Check the extism go-sdk releases newer than v1.7.1 for a host-side
   error mechanism (`CurrentPlugin.SetError` or equivalent). Consult the
   changelog/source, not memory.
2. If available: bump the dependency, use it in the `writeEnvelope`
   WriteBytes-failure branch (guest sees a host error instead of void
   success), test if mockable.
3. If not: strengthen the comment at the fallback branch to name the unsafe
   collapse direction ("guest reads offset 0 as void success — a real
   PERMISSION_DENIED/NOT_FOUND becomes apparent success"), reference #267,
   and close #267 from the PR body with the check's outcome documented.

### 7. Test polish (#268)

- `TestModule_Tick_HangingTickKilledByTimeout` (internal/wasm): add
  `assert.True(t, m.failed.Load())` (or the test-visible equivalent) after
  the timeout path, pinning "interrupt is fatal".
- `TestHostCall_Msgpack` comment: "arrive as int64" → match the actual
  `int8` assertion.

## Testing

- `go test ./internal/wasm/ ./pdk/...` including the new tests; the addMu
  race test under `-race`.
- All three guest modules `tinygo build` locally AND in the new CI job.
- Full `go test ./...` at the end.
- CHANGELOG [Unreleased]: Fixed (#293 shutdown stall, #265 ordering),
  Added (`noda.DecodeInto`, CI guest builds), plus a Changed/internal note
  for the addMu restructure if house style wants it.

## Execution shape

Standing conventions: worktree `.worktrees/wasm-pdk-hardening`, branch
`feat/wasm-pdk-hardening` off latest ORIGIN main (fetch first — local main
goes stale; learned in tranche 3); spec + plan `git add -f`'d onto the
branch; subagent-driven; whole-branch review before PR.

Task split: (1) #293 + #268.1 (both module.go/test), (2) #295 addMu,
(3) #294 DecodeInto + examples + guide, (4) #265 + #268.2 + #267 check,
(5) #296 CI job + gofmt drive-by + CHANGELOG, (6) final verification /
review / PR. PR closes #293 #294 #295 #296 #265 #268 and closes #267 with
the upstream-check outcome in the close text.

## Out of scope

- Custom host↔PDK sentinel ABI for #267 (explicitly rejected).
- Making the wasm-guests CI job a required check (user's later call).
- Any behavioral change to the tick loop, gateway reconnect logic, or
  envelope format beyond the items above.
