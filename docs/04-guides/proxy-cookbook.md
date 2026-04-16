# Proxy Cookbook

Patterns for building a Noda API that proxies to one or more backend services (REST APIs, legacy systems, internal gateways).

## 1. Service with `base_url` + relative URLs (the right way)

Put the host in the service config, use relative paths in workflows. This is the recommended pattern — it keeps each workflow portable between environments and avoids sprinkling `$env()` through every URL.

```json
// noda.json
{
  "services": {
    "trashboard": {
      "plugin": "http",
      "config": {
        "base_url": "{{ $env('TRASHBOARD_URL') }}",
        "timeout": "10s",
        "headers": {
          "Authorization": "Bearer {{ secrets.TRASHBOARD_TOKEN }}"
        }
      }
    }
  }
}
```

```json
// workflows/get-box.json
{
  "id": "get-box",
  "nodes": {
    "fetch": {
      "type": "http.get",
      "services": { "client": "trashboard" },
      "config": {
        "url": "/boxes/{{ input.id }}"
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

**Anti-pattern.** Avoid `{{ $env('TRASHBOARD_URL') }}/boxes/...` inside per-workflow URLs. `$env()` doesn't resolve in workflow expressions anyway (it only runs on `noda.json` at load time), and even if it worked, you'd scatter the host across the codebase.

## 2. Forwarding query parameters

The HTTP request context exposes `query` as a map. Forward it wholesale to the backend for pagination and filters:

```json
{
  "type": "http.get",
  "services": { "client": "trashboard" },
  "config": {
    "url": "/boxes",
    "query": "{{ query }}"
  }
}
```

Or forward specific keys:

```json
{
  "config": {
    "url": "/boxes",
    "query": {
      "page": "{{ query.page ?? 1 }}",
      "per_page": "{{ query.per_page ?? 20 }}",
      "sort": "{{ query.sort }}"
    }
  }
}
```

## 3. Binary passthrough (PDFs, images)

For endpoints that return raw bytes — invoice PDFs, box screenshots — pipe `http.get` into `response.file`. Body is returned verbatim; content-type flows through.

```json
{
  "id": "invoice-pdf",
  "nodes": {
    "fetch": {
      "type": "http.get",
      "services": { "client": "trashboard" },
      "config": { "url": "/invoices/{{ input.id }}/pdf" }
    },
    "send": {
      "type": "response.file",
      "config": {
        "body": "{{ nodes.fetch.body }}",
        "content_type": "{{ nodes.fetch.headers['content-type'] ?? 'application/pdf' }}",
        "filename": "invoice-{{ input.id }}.pdf"
      }
    }
  },
  "edges": [{ "from": "fetch", "to": "send" }]
}
```

## 4. Remapping 403 → 401 at the public edge

Internal services often return 403 when they don't know who the caller is. At the public edge you want 401 so clients retry with credentials. Today this is manual with `control.if`:

```json
{
  "id": "list-boxes",
  "nodes": {
    "fetch": {
      "type": "http.get",
      "services": { "client": "trashboard" },
      "config": { "url": "/boxes", "query": "{{ query }}" }
    },
    "remap": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.fetch.status == 403 }}" }
    },
    "unauthorized": {
      "type": "response.error",
      "config": { "status": 401, "message": "Unauthorized" }
    },
    "pass_through": {
      "type": "response.json",
      "config": { "status": "{{ nodes.fetch.status }}", "body": "{{ nodes.fetch.body }}" }
    }
  },
  "edges": [
    { "from": "fetch", "to": "remap" },
    { "from": "remap", "to": "unauthorized", "when": "true" },
    { "from": "remap", "to": "pass_through", "when": "false" }
  ]
}
```

A dedicated helper is tracked at `FR-proxy-status-remap.md`.

## 5. Proxying 3+ backends — one shared workflow?

A common wish: one reusable proxy workflow, parameterized by which backend service to call. Today this isn't supported at the config level — `services` entries on nodes are static strings. See `FR-parameterized-service-binding.md` for the proposed feature.

Until then, the pragmatic options are:

1. **Duplicate the workflow** per backend. Keep the bodies tiny and mostly just `http.get` → `response.json`.
2. **Generate the workflow JSON** from a template script at build time.
3. **Subworkflow with per-call overrides** — the engine supports it internally for transactions; surface is not yet exposed.
