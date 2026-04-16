# Feature Request: Proxy status remap helper

## Problem

Public-facing APIs that proxy to an internal backend commonly need to remap upstream status codes before returning them to the caller. The canonical case is 403 → 401: the internal service returns "forbidden" because it doesn't know the caller's identity, but the public edge should return "unauthorized" so the client knows to retry with credentials.

This works today in Noda via `control.if` on the proxy response status, followed by `response.error` with the remapped code — but every public endpoint repeats the same 3–5 node boilerplate. Tester flagged this as friction on ~30+ endpoints in a real build.

## Proposed shape

A single node (or middleware) that takes a map of upstream-status → outgoing-status and applies it in one step. Strawman node config:

```json
{
  "type": "http.response.remap",
  "config": {
    "map": { "403": 401, "502": 503 },
    "default": "passthrough"
  }
}
```

Either:

- **As a node**: pipelines into the workflow after an `http.*` call, mutates the response before `response.*` returns it.
- **As middleware**: declared at the route or group level, applies to any workflow that produces an HTTP-shaped response.

The middleware form is closer to how Laravel/Express users think about this and probably composes better with CORS and other response-layer concerns.

## Non-goals

- Full status-code rewriting DSL. A flat map + passthrough default covers the common cases.
- Body rewriting. That's already `transform.set` territory.

## Open questions

- Does it belong in core (`plugins/core/response/`) or as middleware (`internal/server/middleware.go`)? Middleware likely, since the mapping is cross-cutting per route group.
- Should it also rewrite the body (e.g., strip internal error detail when remapping 5xx → 502)? Probably not — keep it status-only, compose with `transform.set` if body rewriting is needed.
