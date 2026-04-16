# Feature Request: Parameterized service binding (shared proxy workflows)

## Problem

Tester's project calls 3–4 backend API services. All of them need the same proxy logic: forward the request, remap status codes, transform response shape, log. The natural instinct is one shared proxy workflow, parameterized by which backend service to target.

Today this isn't possible. A node's `services` block takes a **static** name:

```json
{
  "type": "http.request",
  "services": { "client": "backend-a" },  // ← hardcoded
  "config": { ... }
}
```

There's no way to say "use whichever service the caller picks." Result: the tester copy-pasted the same workflow four times, one per backend. Every future change multiplies by four.

Internally, `internal/engine/subworkflow.go:89` already supports `services.WithOverrides(...)` — but it's wired only for database transactions, not exposed through `workflow.run` config.

## Proposed shape

Extend `workflow.run` to allow passing service overrides:

```json
{
  "type": "workflow.run",
  "config": {
    "workflow": "proxy-to-backend",
    "services": {
      "client": "backend-a"   // ← override: sub-workflow's `client` slot uses this
    },
    "input": { ... }
  }
}
```

Inside `proxy-to-backend`, nodes reference the `client` slot as usual:

```json
{
  "type": "http.request",
  "services": { "client": "client" },  // resolves via parent's override
  "config": { "url": "{{ input.path }}", "method": "{{ input.method }}" }
}
```

The plumbing already exists in the engine; this exposes it via config.

## Alternative (rejected)

Expression syntax in the `services` block — e.g. `"services": { "client": "{{ input.target }}" }`. This couples service resolution to expression evaluation and complicates config validation (you can no longer statically determine which services a workflow touches). The subworkflow-override pattern keeps static analysis intact.

## Benefits

- One shared proxy workflow instead of N copies
- Same pattern enables shared DB query workflows targeting read-replica vs primary, shared cache workflows targeting different cache namespaces, etc.
- The engine already does it — this is a config/docs surface, not a new engine capability

## Open questions

- Should the override also support service *instances* being optional (e.g., sub-workflow declares `client` as required, caller decides at runtime)? Current validation would need to accept runtime-resolved bindings.
- How does this interact with the editor's service-slot visualization? Likely the sub-workflow shows `client` as a generic slot and the caller fills it, matching how inputs/outputs already work.
