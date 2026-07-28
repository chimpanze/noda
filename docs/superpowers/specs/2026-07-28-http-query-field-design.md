# `query` config field for `http.*` nodes — design

**Issue:** #451
**Date:** 2026-07-28
**Status:** approved

## Problem

`http.get`, `http.post`, and `http.request` accept only `url`, `headers`, `body`, and
`timeout`. Query parameters have to be concatenated into `url` by hand, with no
URL-encoding — so any parameter value containing `&`, `=`, or a space produces a
malformed request.

`docs/04-guides/proxy-cookbook.md` §2 documented a `query` field that never existed.
The doc-snippet schema sweep for #445 caught it and rewrote the section around explicit
per-parameter forwarding.

### Correcting the issue's premise

#451 gives this as the cost of doing it by hand today:

```
{{ '/items' + (len(query) > 0 ? '?' + join(map(keys(query), {# + '=' + query[#]}), '&') : '') }}
```

That expression does not work. `internal/engine/context.go:381` builds the node
expression context as exactly `input`, `auth`, `trigger`, `nodes`, and `secrets` —
there is no `query`, so the member access fails on a nil root. `query`, `params`,
`body`, and `headers` exist only in a route's `trigger.input` mapping, which is what
the rewritten §2 already says.

So the real cost today is a route mapping *plus* hand-built concatenation over
`input.*`. The gap is wider than the issue states, not narrower.

Also relevant: inbound `query` reaches a workflow through `parseQuery`
(`internal/server/trigger.go:242`), which uses fiber's `c.Queries()` and returns
`map[string]string`. Repeated parameters (`?tag=a&tag=b`) already collapse to a single
value before any workflow sees them.

## Decisions

Three questions were settled before design.

**1. Scope: config field only.** `query` becomes a node config field. It is *not* added
to the node expression context. Wholesale forwarding therefore costs one route line:

```json
// routes/list-items.json
"trigger": { "workflow": "list-items", "input": { "q": "{{ query }}" } }
```

```json
// workflows/list-items.json
{ "type": "http.get", "config": { "url": "/items", "query": "{{ input.q }}" } }
```

Rejected: adding `query` to `buildExprContext`. It would change the scoping contract
for every node type, and #445 has just rewritten the docs to state the opposite. If
wanted, it belongs in its own issue.

**2. Merge rule: reject at runtime.** If the URL already carries a query string and
`query` is non-empty, error. No precedence rule to guess at or document, and the
failure is loud at the first request rather than a silently wrong upstream call.

This must be a runtime check: `url` is usually an expression, so it cannot be
inspected at validate time.

**3. Values: scalars and arrays; nulls omitted.** Arrays become repeated parameters. A
null or absent value drops the key entirely, which is what makes an optional pagination
parameter work without a conditional. Nested objects are rejected.

## Design

### Config field and schema

Added to the `ConfigSchema` of `http.get`, `http.post`, and `http.request` — the three
nodes sharing `doRequest`. (`plugins/http/redirect.go` is transport configuration, not
a node type.)

```go
"query": map[string]any{
    "description": "Query parameters, URL-encoded and appended to url",
    "oneOf": []any{
        map[string]any{"type": "object"},
        map[string]any{"type": "string"},
    },
},
```

The `oneOf` exists for the **editor**, not the Go validator.
`editor/src/components/panels/NodeConfigPanel.tsx:170` maps a property with
`type: "object"` and no `properties` to the `keyValueMap` widget — a per-key grid with
no way to enter `{{ input.q }}` as the whole value. With `oneOf`, `p.type` is
undefined and the panel falls through to `flexibleValue`, which accepts either form.
This is the same reason `body` carries no `type`.

`description` is in `annotationKeywords` (`internal/registry/configschema.go:17`), so
it may sit beside `oneOf` without tripping the sibling audit at line 123.

The Go-side walker short-circuits any string containing `{{`
(`configschema.go:150`), so the string branch only matters to the editor's stricter
ajv validation.

A string that is *not* an expression (e.g. `"limit=5&sort=name"`) passes schema
validation but is rejected at runtime by the not-a-map check. Accepting a raw query
string is deliberately out of scope: it reintroduces the hand-encoding bug this field
exists to remove.

### Resolution and encoding

Resolve with `plugin.ResolveOptionalDeepAny`, which handles both the whole-map
expression and per-key expressions in one call.

| resolved value | result |
|---|---|
| string | `k=v` |
| number, bool | stringified with `%v`, as `ResolveHeaders` already does |
| `[]any` | one repeated parameter per element, each by the scalar rule |
| `nil` | key omitted entirely |
| `[]any{}`, empty map, absent field | nothing appended; no bare `?` |
| nested object or map | error: `query value for "k" must be a scalar or array of scalars` |
| `query` itself not a map | error naming the resolved type |

A `nil` *element* inside an array is omitted, on the same rule as a nil value. An array
whose elements are not all scalars is rejected with the same message as a nested
object: the scalar rule applies per element, and there is no sane encoding for a nested
value either way. An array of all-nil elements contributes nothing, exactly as an empty
array does.

Encoding uses `net/url.Values.Encode()`: RFC 3986 percent-encoding, and sorted by key,
so output is deterministic. Determinism is a property this repo tests for — #450 and
#460 were both unsorted-iteration bugs.

`Encode()` renders a space as `+` (`application/x-www-form-urlencoded`). This is the
standard query-string form and is accepted universally; it is called out here so it is
not later mistaken for a bug.

### Order of operations in `doRequest`

The new step sits after `base_url` joining and before
`http.NewRequestWithContext`:

1. resolve `url`
2. join with `svc.baseURL` (existing behavior, unchanged)
3. **resolve and encode `query`; if the encoded result is non-empty and the joined URL
   already contains `?`, error; otherwise append**
4. resolve headers, body, timeout (existing, unchanged)

The non-empty qualifier matters: a `query` that resolves to an empty map, or to a map
whose every value is null, encodes to `""` and appends nothing — so it must not reject
a URL that legitimately carries its own query string. Only a query that would actually
contribute parameters conflicts.

Checking after the join means a `base_url` that itself carries a query string is
caught, rather than producing a malformed URL. The error message names the full joined
URL so the source is obvious.

### Code layout

New file `plugins/http/query.go` with one pure function:

```go
func encodeQuery(raw any) (string, error)
```

It takes the already-resolved value and returns the encoded string without a leading
`?`. No context, no service, no network — so the whole value-rule table is unit
testable directly. The call site in `doRequest` is roughly six lines: resolve, encode,
check for an existing `?`, append.

## Error handling

All failures are runtime errors on the node's `error` output, consistent with the rest
of `doRequest`. Messages are prefixed `http.request:` by the existing convention.

- `query` resolves to a non-map → name the resolved type
- a value is a nested object → name the offending key
- URL already has a query string → name the full joined URL

None of these are validate-time checks, because all inputs may be expressions.

## Testing

**Unit (`plugins/http/query_test.go`)** — one case per row of the value table, plus the
cases that motivated the issue:

- a value containing `&`, `=`, and a space (the encoding bug)
- a *key* requiring encoding
- key ordering is deterministic across repeated calls
- empty map and all-nil map produce `""`, so no bare `?` is appended

**Integration** — `doRequest`-level tests that the query string arrives on the wire,
and that the already-has-a-query-string case errors.

**End-to-end** — `examples/node-cookbook/http/` already has an `echo` workflow and
route. Extending it to assert the received query string gives a CI-verified example,
kept alive by the `TestCookbookCoverage` gate.

Each test is written to fail against the unfixed code first.

## Documentation

- `docs/03-nodes/http.get.md`, `http.post.md`, `http.request.md` — document `query`,
  including the null-omission rule and the reject-on-existing-query-string rule.
- `docs/04-guides/proxy-cookbook.md` §2 — restore the wholesale-forwarding pattern the
  section was written for, now with the route-mapping step. Keep its accurate warning
  that `query.*` is not available in a node config; that remains true and is why the
  route mapping is needed.
- `CHANGELOG.md` under `### Added`.

Doc snippets are checked by the #445 gate, so every example added here is validated and
compiled in CI.

## Out of scope

- `query` in the node expression context (see decision 1)
- accepting a raw `"a=1&b=2"` string
- multi-value inbound query parameters — `c.Queries()` collapses them before a workflow
  sees them; changing that is a trigger-layer concern, not this field's
