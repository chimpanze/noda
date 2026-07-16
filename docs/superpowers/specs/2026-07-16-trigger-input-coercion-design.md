# Trigger Input Coercion — Design (#331)

**Date:** 2026-07-16
**Issue:** #331 — `coerceNumeric` silently retypes every numeric-looking trigger input string
**Decision:** Option A (source-based coercion) + per-trigger opt-out, approved by user 2026-07-16.

## Problem

`MapTrigger` (`internal/server/trigger.go:83`) pipes **every** resolved trigger-input string
through `coerceNumeric` (Atoi → ParseFloat). `{{ body.zip }}` = `"0042"` arrives in the
workflow as `42` even though the JSON body preserved the string type. Leading zeros are lost;
TEXT-column writes fail. Reproduced against `docs/01-getting-started/realtime.md`'s own example.

## Decision

Coerce **by transport typing** of the referenced source, not unconditionally:

| Input expression shape | Coerced? | Rationale |
|---|---|---|
| Bare ref to `params.*`, `query.*`, `headers.*` (or `request.` alias) | **Yes** | These transports are string-typed; `{{ query.limit }}` ergonomics preserved (every deployed route relies on it, incl. homebase prod) |
| Bare ref to `body.*` (or `request.body.*`), request body is form-encoded (`Content-Type` contains `form`) | **Yes** | Form bodies are string-typed transport, same as query |
| Bare ref to `body.*`, JSON body | **No** | JSON carries its own types — this is the bug class |
| Computed expression (anything that isn't a single pure member-access chain) | **No** | The expression's result type is authoritative |
| Literal string input value (e.g. `"9"`) | **No** | Behavior change: literals used to coerce; a literal's type is what the author wrote |

"Bare ref" = the whole input value is one `{{ … }}` template whose expression is a single
member-access chain rooted at `params`/`query`/`headers`/`body` (dot or bracket access,
`request.` alias allowed). Detected by regex on the raw template string.

**Opt-out:** `"coerce": false` on the route trigger disables coercion entirely for that route
(for numeric-looking path/query IDs like `"0042"` order numbers). Default `true`.

## Behavior changes (documented in CHANGELOG)

1. JSON-body-sourced and computed/literal trigger inputs are no longer numerically coerced.
2. New `trigger.coerce` boolean (route.json schema + docs).
3. `params`/`query`/`headers`/form-body bare refs behave exactly as before.

## Not doing

- Option B (per-field type schemas on routes) — new config surface, deferred.
- Coercion in worker/scheduler/ws trigger paths — `coerceNumeric` has exactly one call site
  (HTTP `MapTrigger`); nothing else coerces today and we're not adding any.

## Touched surfaces

- `internal/server/trigger.go` (+ tests)
- `internal/config/schemas/route.json` (add `trigger.coerce`)
- `docs/02-config/routes.md`, `docs/01-getting-started/realtime.md` (pitfall callout references #331)
- CHANGELOG `[Unreleased]`
