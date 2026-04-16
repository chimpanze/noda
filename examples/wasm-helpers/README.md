# wasm-helpers — query-only Wasm example

A minimal Wasm module that provides two pure-function helpers Noda's expression engine doesn't ship:

- `format_phone_e164(raw, default_cc)` — strip non-digits, prepend `+`, use `default_cc` when no country code is detectable
- `mask(value, keep_start, keep_end, mask_char)` — redact the middle of a string for logs

This is the pattern to use when you need a small pure function for a workflow expression and don't want to file a feature request.

## What makes it "query-only"

The module exports only `initialize` and `query`. No `tick`, no state. The config omits `tick_rate` entirely — Noda skips the tick loop for modules without a `tick` export.

Compare to [`../wasm-counter`](../wasm-counter), which uses `tick` + in-memory state.

## Build

```bash
cd wasm/helpers
tinygo build -o ../helpers.wasm -target wasi -buildmode=c-shared .
```

`-buildmode=c-shared` is needed on tinygo ≥ 0.40 so exports are callable directly by Extism without running `_start` first. Without it, `initialize` panics with `wasmExportCheckRun`.

## Run

```bash
noda start
```

## Try it

```bash
curl -X POST http://localhost:3000/api/format-phone \
  -H 'Content-Type: application/json' \
  -d '{"raw":"+49 30 1234 5678"}'
# → {"formatted":"+4930123456789 ..."}

curl -X POST http://localhost:3000/api/mask-email \
  -H 'Content-Type: application/json' \
  -d '{"email":"marten@example.com"}'
# → {"masked":"ma**********.com"}
```

## Files

- `noda.json` — one `wasm_runtimes.helpers` entry, no `tick_rate`
- `wasm/helpers/main.go` — `initialize` + `query` only
- `workflows/format-phone.json`, `workflows/mask-email.json` — call the helper via `wasm.query`
- `routes/format-phone.json`, `routes/mask-email.json` — HTTP entry points
