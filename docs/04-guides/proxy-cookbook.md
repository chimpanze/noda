# Proxy Cookbook

Patterns for building a Noda API that proxies to one or more backend services (REST APIs, legacy systems, internal gateways).

## 1. Service with `base_url` + relative URLs (the right way)

Put the host in the service config, use relative paths in workflows. This is the recommended pattern — it keeps each workflow portable between environments and avoids sprinkling `$env()` through every URL.

```json
// noda.json
{
  "services": {
    "inventory": {
      "plugin": "http",
      "config": {
        "base_url": "{{ $env('INVENTORY_URL') }}",
        "timeout": "10s",
        "headers": {
          "Authorization": "Bearer {{ secrets.INVENTORY_TOKEN }}"
        }
      }
    }
  }
}
```

```json
// workflows/get-item.json
{
  "id": "get-item",
  "nodes": {
    "fetch": {
      "type": "http.get",
      "services": { "client": "inventory" },
      "config": {
        "url": "/items/{{ input.id }}"
      }
    },
    "respond": {
      "type": "response.json",
      "config": { "body": "{{ nodes.fetch.body }}" }
    }
  },
  "edges": [{ "from": "fetch", "to": "respond" }]
}
```

**Anti-pattern.** Avoid `{{ $env('INVENTORY_URL') }}/items/...` inside per-workflow URLs. `$env()` doesn't resolve in workflow expressions anyway (it only runs on `noda.json` at load time), and even if it worked, you'd scatter the host across the codebase.

## 2. Forwarding query parameters

`http.get`'s config accepts only `url`, `headers`, `body`, and `timeout` — there is no dedicated query-parameter field, and no way to forward the incoming `query` map wholesale. Forward parameters explicitly, in two steps.

**`query.*` is not available inside a node config.** The node expression context is exactly `input`, `auth`, `trigger`, `nodes`, and `secrets` (see [Expressions](../01-getting-started/expressions.md)). `query`, `params`, `body`, and `headers` exist *only* in a route's `trigger.input` mapping. A node config that says `{{ query.limit }}` fails at runtime with `cannot fetch limit from <nil>` — and `??` does not rescue it, because the member access on the nil root fails before the fallback is considered.

So: map the query parameters into the workflow input on the route, then read them as `input.*` in the node.

```json
// routes/list-items.json
{
  "id": "list-items",
  "method": "GET",
  "path": "/api/items",
  "trigger": {
    "workflow": "list-items",
    "input": {
      "limit": "{{ query.limit }}",
      "offset": "{{ query.offset }}"
    }
  }
}
```

```json
// workflows/list-items.json
{
  "id": "list-items",
  "nodes": {
    "fetch": {
      "type": "http.get",
      "services": { "client": "inventory" },
      "config": {
        "url": "/items?limit={{ input.limit }}&offset={{ input.offset }}"
      }
    },
    "respond": {
      "type": "response.json",
      "config": { "body": "{{ nodes.fetch.body }}" }
    }
  },
  "edges": [{ "from": "fetch", "to": "respond" }]
}
```

Provide defaults for parameters that might be missing from the request. A query parameter that was not sent maps to `null`, so `??` in the *node* config supplies the fallback:

```json
{
  "type": "http.get",
  "services": { "client": "inventory" },
  "config": {
    "url": "/items?page={{ input.page ?? 1 }}&per_page={{ input.per_page ?? 20 }}&sort={{ input.sort ?? '' }}"
  }
}
```

Keep the matching keys in `trigger.input` (`"page": "{{ query.page }}"`, and so on) — a key that is never mapped is absent from `input` entirely, which `??` also handles, but mapping it keeps the route self-documenting and lets `query.schema` validate it.

## 3. Binary passthrough (PDFs, images)

For endpoints that return raw bytes — invoice PDFs, product images — pipe `http.get` into `response.file`. Body is returned verbatim; content-type flows through.

```json
{
  "id": "invoice-pdf",
  "nodes": {
    "fetch": {
      "type": "http.get",
      "services": { "client": "inventory" },
      "config": { "url": "/invoices/{{ input.id }}/pdf" }
    },
    "send": {
      "type": "response.file",
      "config": {
        "data": "{{ nodes.fetch.body }}",
        "content_type": "{{ nodes.fetch.headers['content-type'] ?? 'application/pdf' }}",
        "filename": "invoice-{{ input.id }}.pdf"
      }
    }
  },
  "edges": [{ "from": "fetch", "to": "send" }]
}
```

## 4. Remapping 403 → 401 at the public edge

Internal services often return 403 when they don't know who the caller is. At the public edge you want 401 so clients retry with credentials. Declare the remap once on the route group:

```json
{
  "middleware": {
    "response.status_remap": {
      "map": { "403": 401 }
    }
  },
  "middleware_presets": {
    "public": ["security.cors", "response.status_remap"]
  },
  "route_groups": {
    "/api/public": { "middleware_preset": "public" }
  }
}
```

Every route under `/api/public` gets the 403→401 rewrite automatically. Workflow logic never sees the remapped status — it's applied on the way out the door.

### Advanced: per-endpoint remap in the workflow

If a single endpoint needs logic that branches on the upstream status (e.g. log differently, trigger a refresh workflow), do it in the workflow itself with `control.if`. `limit` and `offset` reach the node as `input.*` via the route's `trigger.input` mapping shown in section 2:

```json
{
  "id": "list-items-remap",
  "nodes": {
    "fetch": {
      "type": "http.get",
      "services": { "client": "inventory" },
      "config": {
        "url": "/items?limit={{ input.limit ?? 20 }}&offset={{ input.offset ?? 0 }}"
      }
    },
    "remap": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.fetch.status == 403 }}" }
    },
    "unauthorized": {
      "type": "response.error",
      "config": { "status": 401, "code": "UNAUTHORIZED", "message": "Unauthorized" }
    },
    "pass_through": {
      "type": "response.json",
      "config": { "status": "{{ nodes.fetch.status }}", "body": "{{ nodes.fetch.body }}" }
    }
  },
  "edges": [
    { "from": "fetch", "to": "remap" },
    { "from": "remap", "to": "unauthorized", "output": "then" },
    { "from": "remap", "to": "pass_through", "output": "else" }
  ]
}
```

The middleware approach is the right default; reach for the workflow pattern only when you need per-endpoint branching.

## 5. Proxying 3+ backends — one shared workflow?

A common wish: one reusable proxy workflow, parameterized by which backend service to call. Today this isn't supported at the config level — `services` entries on nodes are static strings. See `FR-parameterized-service-binding.md` for the proposed feature.

Until then, the pragmatic options are:

1. **Duplicate the workflow** per backend. Keep the bodies tiny and mostly just `http.get` → `response.json`.
2. **Generate the workflow JSON** from a template script at build time.
3. **Subworkflow with per-call overrides** — the engine supports it internally for transactions; surface is not yet exposed.
