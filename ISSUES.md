# Docs / DX Clarity Issues

Tester hit these while building with Noda. The features exist — the discoverability doesn't.

## Plugin naming

- **Redis cache plugin** — tester used `"plugin": "redis"`, real name is `"cache"`. Same likely applies to `"stream"` and `"pubsub"`. Docs should make plugin registration names prominent, and error messages for unknown plugin names should suggest close matches.

## Auth / JWT

- **RS256 / RS384 / RS512 + file-based keys** are supported (`public_key_file`) but tester didn't find them. Needs a visible example in the auth docs, not just the config schema.

## HTTP proxying

- **Query param forwarding** — `{{ query }}` is available in the request context, but tester concluded "no way to forward query params". Needs an explicit example for the common proxy/pagination pattern.
- **Binary response passthrough** — `http.get → response.file` works end-to-end, but tester wasn't sure. Needs a documented PDF/image passthrough example.
- **403 → 401 remapping** — pattern works via `control.if` on proxy status, but boilerplate is heavy. Either document the pattern or add a helper.

## Observability

- **Prometheus metrics** — implemented via OTel prometheus exporter at `/metrics`, but tester claimed it was "documented but not implemented". Docs need to confirm it's live, show the endpoint, and list exposed metrics.

## Server config

- **CORS** — configurable via `security.cors` (`allow_origins`, `allow_methods`, `allow_headers`, `allow_credentials`), but tester didn't configure it. Needs to appear in the getting-started / server config docs.

## Environment variables & secrets (biggest gap)

Three mechanisms exist; tester found `$env()`, failed with it in workflows, and invented `env.VAR` instead of discovering `secrets.*`.

| Mechanism | Syntax | Scope |
|---|---|---|
| `$env()` | `{{ $env('NAME') }}` | Root config only (load time) |
| `$var()` | `{{ $var('NAME') }}` | All config sections (from `vars.json`) |
| `secrets.*` | `{{ secrets.NAME }}` | Workflow expressions (runtime) |

Actions:
- When `$env()` appears in a workflow expression, the error should point to `secrets.NAME`.
- `env.VAR` is not a Noda pattern — if it silently evaluates to undefined in workflows, that should also produce a helpful error.
- Put the 3-mechanism table on the first page a tester lands on, not only in `docs/02-config/variables.md`.

## Service `base_url` + relative URLs

Tester's first "fix" during the build was switching from `$env()` per-workflow URLs to setting `base_url` on the service and using relative URLs in workflows. That's the **intended** Noda pattern, but discovering it required reverse-engineering.

Actions:
- Document the "proxy in 5 minutes" cookbook pattern: service with `base_url` + relative paths in workflows + `{{ query }}` forwarding.
- The services config reference should lead with `base_url` as the recommended pattern, with `$env()`-per-URL called out as an anti-pattern.

## Expression fallback operator (`||` vs `??`)

Tester switched `||` → `??` and thought `||` was broken. Verified: in expr-lang, `||` is strictly a **boolean** operator — `a || "fallback"` is a compile error when `a` is a string. `??` is the only null-coalesce and triggers on `nil` only (empty string, `false`, `0` pass through unchanged).

Actions:
- Expression docs need a firm rule: **use `??` for fallbacks, never `||`**. JavaScript-origin developers will reach for `||` first and get a cryptic "mismatched types" compile error.
- Call out `??`'s nil-only semantics explicitly — `{{ request.query.page ?? 1 }}` returns `"0"` (not `1`) when `page` is `"0"`.
- Accessing a missing key at the top level is a compile error (both operators). Document the pattern for optional fields (dict access, or ensure the key always exists in the env).

## Wasm extensibility story

Wasm is the escape hatch for domain-specific logic Noda won't ship in core (phone E.164, PII masking, custom protocols). The runtime is fully implemented and `wasm.query` exists — but the docs frame Wasm as heavy, so nobody reaches for it to add a small helper.

Actions:
- `docs/04-guides/wasm-development.md` use-case list is all stateful (game servers, bot gateways, custom protocols). Add a "Pure-function helpers" use case and a minimal `query`-only example (no `tick`, no `initialize` boilerplate).
- Every example has a `tick` export. Make clear that `tick` + `initialize` are optional when only `query` is used, and that `tick_rate` / `tick_timeout` are ignored for query-only modules.
- `pdk/` has no README. Needs an entry point with "build → register → call from workflow" in one page.
- No guidance on "when to build a Wasm helper vs. file a feature request". A short decision paragraph: generic & reusable → FR; domain-specific → Wasm.
- Only example project using Wasm is `examples/wasm-counter` (stateful). Add a second example showing a stateless query helper (e.g., phone formatting or string masking) as the copy-paste starting point.
