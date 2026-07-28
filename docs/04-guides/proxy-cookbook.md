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

`http.*` nodes take a `query` object, URL-encoded and appended to `url`. To forward
the caller's parameters wholesale, map the query map into the workflow input on the
route, then hand it to the node.

**`query.*` is not available inside a node config.** The node expression context is
exactly `input`, `auth`, `trigger`, `nodes`, and `secrets` (see
[Expressions](../01-getting-started/expressions.md)). `query`, `params`, `body`, and
`headers` exist *only* in a route's `trigger.input` mapping. A node config that says
`{{ query.limit }}` fails at runtime with `cannot fetch limit from <nil>` — and `??`
does not rescue it, because the member access on the nil root fails before the
fallback is considered. That is why the map is carried through `input`.

```json
// routes/list-items.json
{
  "id": "list-items",
  "method": "GET",
  "path": "/api/items",
  "trigger": {
    "workflow": "list-items",
    "input": {
      "q": "{{ query }}"
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
        "url": "/items",
        "query": "{{ input.q }}"
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

To forward a chosen subset instead, map the parameters individually on the route and
name them in the node's `query`:

```json
// workflows/list-items.json (subset variant)
{
  "id": "list-items-subset",
  "nodes": {
    "fetch": {
      "type": "http.get",
      "services": { "client": "inventory" },
      "config": {
        "url": "/items",
        "query": {
          "limit": "{{ input.limit }}",
          "cursor": "{{ input.cursor }}"
        }
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

A parameter the caller omitted resolves to null and is dropped, so `cursor` above
needs no conditional. Values are URL-encoded, so a parameter containing `&`, `=`, or a
space is safe — which hand-built URL concatenation is not.

Do not put a query string in `url` *and* set `query`: that is an error, not a merge.

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

If a single endpoint needs logic that branches on the upstream status (e.g. log differently, trigger a refresh workflow), do it in the workflow itself with `control.if`. `limit` and `offset` reach the node as `input.*` because the route maps each one individually into `trigger.input` (`"limit": "{{ query.limit }}"`, `"offset": "{{ query.offset }}"` — `query.*` still only exists there, never in a node config), and the workflow reads them back as `input.limit` / `input.offset`:

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
