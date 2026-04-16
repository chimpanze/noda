# Feature Request: Custom middleware (user-defined)

## Problem

Noda ships several built-in middlewares (auth, CORS, rate limit, etc.), but has no first-class way for a user to add their own. Tester hit this with `X-Request-Id`: Laravel auto-generates a UUID per request if the header is missing, and public APIs rely on that ID in logs and responses.

The only workaround today is `transform.set` with `$uuid()` in every workflow — which is (a) easy to forget, (b) impossible to apply to workflows that bypass transforms, (c) pure boilerplate repeated across 30+ endpoints.

`X-Request-Id` is just the trigger — this is really about **allowing users to inject behavior into the request/response lifecycle without editing Go code or writing a plugin**.

## Proposed shapes (pick one)

### Option A — declarative middleware in config

A new top-level config entry that chains Noda nodes as request/response interceptors:

```json
{
  "middleware": {
    "request-id": {
      "on": "request",
      "scope": "global",
      "steps": [
        {
          "type": "transform.set",
          "config": { "headers.X-Request-Id": "{{ request.headers['X-Request-Id'] ?? $uuid() }}" }
        }
      ]
    }
  }
}
```

Pro: fits the config-driven ethos, no new plugin surface.
Con: workflow engine has to run outside the workflow boundary; unclear what the "request" context looks like.

### Option B — Wasm middleware hook

Extend the Wasm runtime with a `middleware` export that receives the request/response and can mutate headers/status/body.

Pro: powerful, already sandboxed.
Con: heavy for one-line helpers like `X-Request-Id`.

### Option C — pluggable middleware via PDK

Publish a small Go interface in `pkg/api/` that third-party code can implement, similar to how plugins register services. Ship as Go plugins or compile into a custom binary.

Pro: full power, full speed.
Con: requires Go + recompile; not config-only.

## Near-term vs long-term

- **Near-term (cheap win):** ship a built-in `request-id` middleware that auto-generates `X-Request-Id` if missing and threads it through logs. Solves the specific case without a full framework.
- **Long-term:** pick one of A/B/C above based on how many more of these cases show up. Candidates likely in the same bucket: geo-IP enrichment, request body size guards, response compression policies, header normalization, PII scrubbing of logs.

## Related

- `FR-proxy-status-remap.md` is a narrower middleware request; if Option A is adopted, both could share the same mechanism.
- Wasm already covers per-workflow custom logic; this FR is specifically about the **request/response edge**, which workflows can't intercept.
