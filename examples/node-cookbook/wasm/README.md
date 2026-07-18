# Cookbook: wasm (`wasm.send`, `wasm.query`)

Runnable example for `wasm.send` (fire-and-forget command) and `wasm.query`
(synchronous request/response) against a stateful Wasm guest module — a
running total ("tally"). Every step below is verified in CI by
[`verify.json`](verify.json).

## Control structure: send is async, query is sync

`wasm.send` (`workflows/send.json`) hands a command to the module's inbox
and returns immediately (`202`) — it does **not** wait for the command to be
applied. The guest's `tick` export, called by the host at the configured
`tick_rate` (Hz), drains the inbox and mutates the module's in-memory state
(`total += value` for an `add` command).

`wasm.query` (`workflows/query.json`) calls the guest's `query` export
directly and returns its result inline (`200`), reflecting whatever state
the last tick left behind.

Because `send` only queues the command, a `query` issued immediately after
a `send` can race the next tick and still see the old total. `verify.json`'s
last step polls with `retry_timeout: "5s"` for exactly this reason — with
`tick_rate: 10` (10 Hz → ~100ms/tick) the add is applied well within that
window, but the poll makes the test robust rather than timing-dependent.

## Run

This project has no external dependencies. Run it directly:

```bash
go run ./cmd/noda start --config examples/node-cookbook/wasm
```

Then:

```bash
curl http://localhost:3000/api/tally                        # {"total":0}
curl -X POST http://localhost:3000/api/tally -d '{"value":5}' \
  -H 'Content-Type: application/json'                        # 202, {"sent":true}
curl http://localhost:3000/api/tally                        # {"total":5} (after the next tick)
```

## Config shape

`noda.json` declares the `tally` Wasm runtime (module path is resolved
relative to the project directory):

```json
{
  "wasm_runtimes": {
    "tally": {
      "module": "wasm/tally.wasm",
      "tick_rate": 10,
      "encoding": "json",
      "config": { "initial": 0 }
    }
  }
}
```

`tick_rate` is in Hz (ticks per second) — confirmed against
`docs/02-config/noda-json.md` and `examples/wasm-counter/noda.json`. `10`
keeps tick latency low (~100ms) so the cookbook's polled assertion resolves
quickly without a long `retry_timeout`.

## The guest module

`wasm/tally/main.go` mirrors `examples/wasm-counter/wasm/counter/main.go`:
a package-level `total int64` persists in Wasm linear memory across ticks,
and the module exports the standard `initialize` / `tick` / `query` /
`shutdown` quartet via `//go:wasmexport`. It supports a single command,
`{"op": "add", "value": N}`; `query` returns `{"total": <n>}`, which
`workflows/query.json` reads back as `nodes.ask.total` — a workflow node
sees a Wasm query's result as the JSON object the guest returned from
`query()`, unwrapped (mirrors `examples/wasm-helpers`' `nodes.format.value`
pattern, where `helpers`' query guest returns `{"value": ...}`).

The committed `wasm/tally.wasm` is rebuilt by CI's "Build example guest
modules" step on every run (drift would fail that job, though it does not
currently diff the artifact — see `examples/wasm-counter` for the same
convention). To rebuild it locally after editing `main.go`:

```bash
cd examples/node-cookbook/wasm/wasm/tally
tinygo build -o ../tally.wasm -target wasi -buildmode=c-shared .
```

Requires tinygo (this repo is built against 0.40.1) — see
`docs/04-guides/wasm-development.md` for the full guest-authoring guide.
