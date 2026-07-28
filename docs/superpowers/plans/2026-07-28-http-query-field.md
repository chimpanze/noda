# `http.*` query config field — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an optional `query` config field to `http.get`, `http.post`, and `http.request` that URL-encodes an object into the request's query string.

**Architecture:** One pure function, `encodeQuery(raw any) (string, error)`, in a new `plugins/http/query.go`, called from the shared `doRequest` after `base_url` joining and before the request is built. All three node types share `doRequest`, so the executor change is made once; only the three `ConfigSchema` literals are edited separately.

**Tech Stack:** Go, `net/url` (`url.Values.Encode`), `internal/plugin` resolve helpers, testify, `httptest`.

**Spec:** `docs/superpowers/specs/2026-07-28-http-query-field-design.md`

## Global Constraints

- **Read the spec first.** The value-handling table in "Resolution and encoding" is the contract; this plan implements it exactly.
- **TDD is mandatory.** Every task writes a failing test, runs it to watch it fail, then implements. A test that passes on first run is a plan failure — stop and fix the test.
- **Full local gate before any push** (all six, in order):
  ```bash
  gofmt -l . | grep -v node_modules
  go vet ./...
  golangci-lint run --timeout=5m --build-tags=integration
  go test ./...
  go test -tags integration ./...
  govulncheck $(go list ./... | grep -v '/node_modules/')
  ```
  If `golangci-lint` reports findings in paths under `.worktrees/` that do not exist, that is a stale cache: run `golangci-lint cache clean` and re-run. If `govulncheck` prints nothing at all, the tool is broken — a genuine clean run says "No vulnerabilities found."
- **`gofmt -l` must be run from the repo root**, not on named files.
- **Work in the existing worktree** `.worktrees/http-query-field` on branch `feat/http-query-field`. Verify with `git rev-parse --show-toplevel` before every commit.
- **Naming gotcha:** `doRequest` declares a local variable named `url`, which shadows the `net/url` package inside that function. Never reference `url.Values` in `request.go`. This is why the encoder lives in its own file.
- Error messages in this plugin are prefixed with the literal `http.request:` even in `http.get`/`http.post`, because they come from the shared `doRequest`. Match that existing convention.

---

### Task 1: `encodeQuery` — the pure encoder

**Files:**
- Create: `plugins/http/query.go`
- Test: `plugins/http/query_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `func encodeQuery(raw any) (string, error)` — takes an **already-resolved** config value (no expressions remain) and returns a URL-encoded query string **without** a leading `?`. Returns `("", nil)` when the input contributes no parameters. Task 2 calls this.

- [ ] **Step 1: Write the failing test**

Create `plugins/http/query_test.go`:

```go
package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeQuery(t *testing.T) {
	tests := []struct {
		name string
		raw  any
		want string
	}{
		{"nil input", nil, ""},
		{"empty map", map[string]any{}, ""},
		{"single string", map[string]any{"q": "hello"}, "q=hello"},
		{"number stringified", map[string]any{"limit": 10}, "limit=10"},
		{"float stringified", map[string]any{"ratio": 1.5}, "ratio=1.5"},
		{"bool stringified", map[string]any{"active": true}, "active=true"},
		// Sorted by key: url.Values.Encode sorts, so output is deterministic.
		{"keys sorted", map[string]any{"b": "2", "a": "1", "c": "3"}, "a=1&b=2&c=3"},
		// The bug this field exists to remove: hand-concatenation does not encode.
		{"value with reserved chars", map[string]any{"note": "a&b=c d"}, "note=a%26b%3Dc+d"},
		{"key needing encoding", map[string]any{"a b": "1"}, "a+b=1"},
		// Nulls are omitted entirely, so an optional param needs no conditional.
		{"null omitted", map[string]any{"cursor": nil, "limit": 10}, "limit=10"},
		{"all-null map encodes empty", map[string]any{"cursor": nil}, ""},
		// Arrays become repeated params, in element order.
		{"array repeated", map[string]any{"tag": []any{"new", "sale"}}, "tag=new&tag=sale"},
		{"array element order preserved", map[string]any{"t": []any{"z", "a"}}, "t=z&t=a"},
		{"empty array contributes nothing", map[string]any{"tag": []any{}}, ""},
		{"nil element skipped", map[string]any{"tag": []any{"new", nil, "sale"}}, "tag=new&tag=sale"},
		{"all-nil array contributes nothing", map[string]any{"tag": []any{nil}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeQuery(tt.raw)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEncodeQuery_Errors(t *testing.T) {
	tests := []struct {
		name    string
		raw     any
		wantMsg string
	}{
		{"not a map", "limit=5", "must resolve to an object"},
		{"number instead of map", 42, "must resolve to an object"},
		{"nested object value", map[string]any{"f": map[string]any{"a": 1}}, `query value for "f" must be a scalar`},
		{"nested array value", map[string]any{"f": []any{[]any{"x"}}}, `query value for "f" must be a scalar`},
		{"object inside array", map[string]any{"f": []any{"ok", map[string]any{"a": 1}}}, `query value for "f" must be a scalar`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encodeQuery(tt.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

// Determinism matters here for the same reason it did in #450 and #460:
// url.Values.Encode sorts keys, so repeated calls must agree exactly.
func TestEncodeQuery_Deterministic(t *testing.T) {
	raw := map[string]any{"d": "4", "a": "1", "c": "3", "b": "2", "e": "5"}
	first, err := encodeQuery(raw)
	require.NoError(t, err)
	for range 50 {
		got, err := encodeQuery(raw)
		require.NoError(t, err)
		require.Equal(t, first, got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/http/ -run 'TestEncodeQuery' -count=1`
Expected: FAIL to compile with `undefined: encodeQuery`.

- [ ] **Step 3: Write minimal implementation**

Create `plugins/http/query.go`:

```go
package http

import (
	"fmt"
	"net/url"
)

// encodeQuery turns an already-resolved `query` config value into a URL-encoded
// query string with no leading "?". It returns "" when the value contributes no
// parameters, so a caller can tell "append nothing" from "append something"
// without re-inspecting the map.
//
// It lives in its own file because doRequest declares a local named `url`,
// which shadows net/url inside that function.
//
// Encoding is url.Values.Encode: RFC 3986 percent-encoding, keys sorted, and a
// space rendered as "+" (application/x-www-form-urlencoded). The sort is what
// makes output deterministic — the property #450 and #460 were both about.
func encodeQuery(raw any) (string, error) {
	if raw == nil {
		return "", nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return "", fmt.Errorf("query must resolve to an object, got %T", raw)
	}

	values := url.Values{}
	for k, v := range m {
		switch val := v.(type) {
		case nil:
			// A null value drops the key entirely, so an optional parameter
			// such as a pagination cursor needs no conditional at the call
			// site. Distinguishing absent from empty is deliberately not
			// supported; see the spec's "Values" decision.
		case []any:
			for _, el := range val {
				if el == nil {
					continue
				}
				s, err := queryScalar(k, el)
				if err != nil {
					return "", err
				}
				values.Add(k, s)
			}
		default:
			s, err := queryScalar(k, val)
			if err != nil {
				return "", err
			}
			values.Add(k, s)
		}
	}
	return values.Encode(), nil
}

// queryScalar stringifies one query value, rejecting the shapes that have no
// sane query-string encoding. Scalars are formatted with %v, matching what
// plugin.ResolveHeaders already does for header values.
func queryScalar(key string, v any) (string, error) {
	switch v.(type) {
	case map[string]any, []any:
		return "", fmt.Errorf("query value for %q must be a scalar or array of scalars", key)
	}
	return fmt.Sprintf("%v", v), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugins/http/ -run 'TestEncodeQuery' -count=1 -v`
Expected: PASS, all subtests.

- [ ] **Step 5: Run the package suite and gofmt**

Run from repo root:
```bash
gofmt -l . | grep -v node_modules
go test ./plugins/http/ -count=1
```
Expected: no gofmt output; package tests pass.

- [ ] **Step 6: Commit**

```bash
git add plugins/http/query.go plugins/http/query_test.go
git commit -m "feat(http): add encodeQuery, the query-string encoder for #451

Pure function over an already-resolved config value. Scalars stringify with
%v as header values do, arrays become repeated parameters, nulls drop the key
so an optional parameter needs no conditional, and nested values are rejected.

url.Values.Encode sorts keys, so output is deterministic."
```

---

### Task 2: Wire `query` into `doRequest`

**Files:**
- Modify: `plugins/http/request.go` (insert after the `base_url` join at lines 83-90)
- Test: `plugins/http/query_test.go` (append)

**Interfaces:**
- Consumes: `encodeQuery(raw any) (string, error)` from Task 1.
- Produces: `query` handling live for all three node types. Task 3 adds the schema that lets a config declaring it pass validation; Task 4 depends on the wire behavior working.

- [ ] **Step 1: Write the failing test**

Append to `plugins/http/query_test.go` (add `"context"`, `"net/http"`, `"net/http/httptest"`, and the `engine` import — see `http_test.go` for the existing pattern):

```go
// The query field must reach the wire URL-encoded. httptest gives us the raw
// RequestURI, which is the only place the encoding is observable.
func TestDoRequest_QueryReachesTheWire(t *testing.T) {
	var gotRawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"cursor": nil}))
	config := map[string]any{
		"url": ts.URL + "/items",
		"query": map[string]any{
			"limit":  10,
			"note":   "a&b c",
			"cursor": "{{ input.cursor }}",
		},
	}

	output, _, err := doRequest(context.Background(), execCtx, config, newTestService(), "GET")
	require.NoError(t, err)
	assert.Equal(t, api.OutputSuccess, output)
	// Sorted, encoded, and cursor omitted because it resolved to nil.
	assert.Equal(t, "limit=10&note=a%26b+c", gotRawQuery)
}

// A whole-map expression is the wholesale-forwarding shape the proxy cookbook
// documents: the route maps {{ query }} into input, the node reads input.*.
func TestDoRequest_QueryFromWholeMapExpression(t *testing.T) {
	var gotRawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{
		"q": map[string]any{"sort": "name", "limit": "5"},
	}))
	config := map[string]any{"url": ts.URL + "/items", "query": "{{ input.q }}"}

	_, _, err := doRequest(context.Background(), execCtx, config, newTestService(), "GET")
	require.NoError(t, err)
	assert.Equal(t, "limit=5&sort=name", gotRawQuery)
}

// The merge rule: a url that already carries a query string is rejected rather
// than merged, so there is no precedence to guess at.
func TestDoRequest_QueryConflictsWithURLQueryString(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	config := map[string]any{
		"url":   "http://example.invalid/items?sort=name",
		"query": map[string]any{"limit": 10},
	}

	_, _, err := doRequest(context.Background(), execCtx, config, newTestService(), "GET")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has a query string")
	assert.Contains(t, err.Error(), "sort=name", "the error must name the offending url")
}

// An empty query must NOT trigger the conflict rule — it appends nothing, so
// there is nothing to conflict with.
func TestDoRequest_EmptyQueryDoesNotConflict(t *testing.T) {
	var gotRawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	config := map[string]any{
		"url":   ts.URL + "/items?sort=name",
		"query": map[string]any{"cursor": nil},
	}

	_, _, err := doRequest(context.Background(), execCtx, config, newTestService(), "GET")
	require.NoError(t, err, "an all-null query contributes nothing and must not conflict")
	assert.Equal(t, "sort=name", gotRawQuery)
}

// No query field at all must leave the url untouched, including its own "?".
func TestDoRequest_NoQueryFieldLeavesURLAlone(t *testing.T) {
	var gotRawQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	config := map[string]any{"url": ts.URL + "/items?sort=name"}

	_, _, err := doRequest(context.Background(), execCtx, config, newTestService(), "GET")
	require.NoError(t, err)
	assert.Equal(t, "sort=name", gotRawQuery)
}

// The conflict check runs after base_url joining, so a base_url carrying its
// own query string is caught instead of producing a malformed URL.
func TestDoRequest_QueryConflictsWithBaseURLQueryString(t *testing.T) {
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	svc := newTestService()
	svc.baseURL = "http://example.invalid/v1?apikey=abc"
	config := map[string]any{"url": "/items", "query": map[string]any{"limit": 10}}

	_, _, err := doRequest(context.Background(), execCtx, config, svc, "GET")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already has a query string")
	assert.Contains(t, err.Error(), "apikey=abc", "the joined url must be named, not just the url field")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/http/ -run 'TestDoRequest_Query|TestDoRequest_EmptyQuery|TestDoRequest_NoQueryField' -count=1`
Expected: FAIL. `TestDoRequest_QueryReachesTheWire` should report an empty `gotRawQuery` (the field is ignored today), and the two conflict tests should report "An error is expected but got nil".

Note: `TestDoRequest_NoQueryFieldLeavesURLAlone` will PASS at this point — it pins existing behavior against regression from Step 3. That is expected and is the one exception to the fail-first rule in this task.

- [ ] **Step 3: Write minimal implementation**

In `plugins/http/request.go`, insert immediately after the `base_url` join block (currently ending at line 90, `}`), before the `// Resolve headers` comment:

```go
	// Append query parameters (#451). This runs after the base_url join so a
	// base_url carrying its own query string is caught too, rather than
	// producing a malformed URL.
	queryRaw, queryOk, queryErr := plugin.ResolveOptionalDeepAny(nCtx, config, "query")
	if queryErr != nil {
		return "", nil, fmt.Errorf("http.request: resolve query: %w", queryErr)
	}
	if queryOk {
		encoded, encErr := encodeQuery(queryRaw)
		if encErr != nil {
			return "", nil, fmt.Errorf("http.request: %w", encErr)
		}
		// Only a query that would actually contribute parameters can conflict:
		// an empty or all-null query appends nothing, so it must not reject a
		// url that legitimately carries its own query string.
		if encoded != "" {
			if strings.Contains(url, "?") {
				return "", nil, fmt.Errorf("http.request: url %q already has a query string; use either the url query or the query field, not both", url)
			}
			url += "?" + encoded
		}
	}
```

`strings` and `fmt` are already imported by `request.go`. Do **not** add a `net/url` import to this file — the local variable `url` shadows it.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugins/http/ -count=1 -v -run 'TestDoRequest|TestEncodeQuery'`
Expected: PASS, all subtests, including the previously-failing three.

- [ ] **Step 5: Run the full package suite**

Run: `go test ./plugins/http/ -count=1`
Expected: PASS. If any pre-existing test now fails, the insertion point is wrong — the block must come after the `base_url` join and before header resolution.

- [ ] **Step 6: Commit**

```bash
gofmt -l . | grep -v node_modules
git add plugins/http/request.go plugins/http/query_test.go
git commit -m "feat(http): append the query field to the request URL (#451)

Runs after the base_url join, so a base_url carrying its own query string is
caught rather than producing a malformed URL. A url that already has a query
string is rejected rather than merged; an empty or all-null query appends
nothing and so cannot conflict."
```

---

### Task 3: Declare `query` in the three ConfigSchemas

**Files:**
- Modify: `plugins/http/get.go:16-27`, `plugins/http/post.go:16-27`, `plugins/http/request.go:22-33` (the `ConfigSchema` methods)
- Test: `plugins/http/schema_audit_test.go`

**Interfaces:**
- Consumes: the executor behavior from Task 2.
- Produces: configs declaring `query` pass `registry.ValidateNodeConfig`, so Task 4's cookbook project and Task 5's doc snippets validate.

- [ ] **Step 1: Write the failing test**

Append to `plugins/http/schema_audit_test.go`:

```go
// The query field must accept BOTH shapes the docs show: a literal object and
// a whole-map expression string. The oneOf is what makes the second legal for
// the editor's stricter ajv validation; the Go walker short-circuits any
// string containing "{{" before it reaches the schema at all.
func TestQueryFieldAcceptsObjectAndExpression(t *testing.T) {
	schemas := map[string]map[string]any{
		"http.get":     (&getDescriptor{}).ConfigSchema(),
		"http.post":    (&postDescriptor{}).ConfigSchema(),
		"http.request": (&requestDescriptor{}).ConfigSchema(),
	}
	for nodeType, schema := range schemas {
		t.Run(nodeType, func(t *testing.T) {
			assert.Empty(t, registry.CheckSchemaVocabulary(schema))

			base := map[string]any{"url": "/items"}
			if nodeType == "http.request" {
				base["method"] = "GET"
			}

			objForm := map[string]any{}
			exprForm := map[string]any{}
			for k, v := range base {
				objForm[k] = v
				exprForm[k] = v
			}
			objForm["query"] = map[string]any{"limit": "10"}
			exprForm["query"] = "{{ input.q }}"

			assert.Empty(t, registry.ValidateNodeConfig(schema, objForm),
				"a literal object must validate")
			assert.Empty(t, registry.ValidateNodeConfig(schema, exprForm),
				"a whole-map expression must validate")
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/http/ -run 'TestQueryFieldAcceptsObjectAndExpression' -count=1`
Expected: FAIL. The schemas do not declare `query`, and each is `"type": "object"` with `properties` — confirm the failure is an unknown-property rejection, not a compile error. If it unexpectedly passes, the schemas are not strict about unknown keys; stop and report that, because it changes what this test proves.

- [ ] **Step 3: Write minimal implementation**

Add this entry to the `properties` map in all three `ConfigSchema` methods (`get.go`, `post.go`, `request.go`), after `"headers"`:

```go
			"query": map[string]any{
				"description": "Query parameters, URL-encoded and appended to url",
				"oneOf": []any{
					map[string]any{"type": "object"},
					map[string]any{"type": "string"},
				},
			},
```

Two things this shape is doing, neither obvious:

1. **`oneOf` steers the editor.** `editor/src/components/panels/NodeConfigPanel.tsx:170` maps a property with `type: "object"` and no `properties` to the `keyValueMap` widget — a per-key grid with no way to type `{{ input.q }}` as the whole value. With `oneOf`, `p.type` is undefined and the panel falls through to `flexibleValue`, which accepts either form.
2. **`description` beside `oneOf` is legal.** `description` is in `annotationKeywords` (`internal/registry/configschema.go:17`), so it does not trip the sibling audit at line 123. Do **not** add any constraint keyword (`type`, `required`, …) beside `oneOf` — that audit will reject it.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugins/http/ -count=1 -v -run 'TestQueryField|TestConfigSchemasMatchExecutors'`
Expected: PASS. `TestConfigSchemasMatchExecutors` must still pass unchanged — it asserts `CheckSchemaVocabulary` is clean, which is what catches a malformed `oneOf`.

- [ ] **Step 5: Run the package suite plus the node-schema surfaces**

```bash
go test ./plugins/http/ ./internal/registry/ ./internal/mcp/ ./cmd/noda/ -count=1
```
Expected: PASS. These cover the schema-consuming surfaces (`noda_get_node_schema`, node docs generation) that a new property can disturb.

- [ ] **Step 6: Commit**

```bash
gofmt -l . | grep -v node_modules
git add plugins/http/get.go plugins/http/post.go plugins/http/request.go plugins/http/schema_audit_test.go
git commit -m "feat(http): declare query in the get/post/request ConfigSchemas (#451)

oneOf rather than type:object so the editor renders the flexibleValue widget
instead of keyValueMap, which has no way to enter a whole-map expression."
```

---

### Task 4: CI-verified cookbook example

**Files:**
- Modify: `examples/node-cookbook/http/routes/echo.json`, `examples/node-cookbook/http/routes/echo-get.json`, `examples/node-cookbook/http/workflows/echo.json`, `examples/node-cookbook/http/verify.json`
- Create: `examples/node-cookbook/http/workflows/get-query.json`, `examples/node-cookbook/http/routes/get-query.json`

**Interfaces:**
- Consumes: working `query` handling (Tasks 2, 3).
- Produces: an end-to-end example kept alive by the cookbook harness. No later task depends on it.

**Context:** The cookbook project proxies to its own `/api/echo` endpoint. To observe the query string that arrived, the echo workflow must echo it back. Assertions use `LookupPath` (`internal/testing/cookbook/assert.go:14`), which splits on `.`, so `received_query.limit` works.

**One thing deliberately not asserted:** the example sends `tag` as an array to demonstrate the repeated-parameter syntax, but the assertions ignore it. Inbound query parsing goes through fiber's `c.Queries()` (`internal/server/trigger.go:242`), which returns `map[string]string` and collapses `?tag=new&tag=sale` to a single unspecified value. Asserting on it would be flaky. Repeated parameters are covered by Task 1's unit tests, where the encoded string is observable directly.

- [ ] **Step 1: Make the echo endpoint report the query it received**

In `examples/node-cookbook/http/routes/echo-get.json` and `examples/node-cookbook/http/routes/echo.json`, add one line to `trigger.input`:

```json
      "received_query": "{{ query }}"
```

so `echo-get.json`'s trigger reads:

```json
  "trigger": {
    "workflow": "echo",
    "input": {
      "payload": "{{ body }}",
      "method": "{{ method }}",
      "received_query": "{{ query }}"
    }
  }
```

In `examples/node-cookbook/http/workflows/echo.json`, add one line to the response body:

```json
          "received_query": "{{ input.received_query }}",
```

so the `respond` node's `body` reads:

```json
        "body": {
          "echo": "{{ input.payload }}",
          "method": "{{ input.method }}",
          "received_query": "{{ input.received_query }}",
          "marker": "cookbook-echo"
        }
```

- [ ] **Step 2: Create the workflow that sends a query**

Create `examples/node-cookbook/http/workflows/get-query.json`:

```json
{
  "id": "get-query",
  "nodes": {
    "fetch": {
      "type": "http.get",
      "services": { "client": "web" },
      "config": {
        "url": "{{ secrets.COOKBOOK_BASE_URL }}/api/echo",
        "query": {
          "limit": "{{ input.limit }}",
          "note": "a&b c",
          "tag": ["new", "sale"],
          "cursor": "{{ input.cursor }}"
        },
        "timeout": "5s"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "status": "{{ nodes.fetch.status }}",
          "received_query": "{{ nodes.fetch.body.received_query }}"
        }
      }
    }
  },
  "edges": [
    { "from": "fetch", "to": "respond" }
  ]
}
```

- [ ] **Step 3: Create the route that forwards the incoming query**

Create `examples/node-cookbook/http/routes/get-query.json`:

```json
{
  "id": "proxy-get-query",
  "method": "GET",
  "path": "/api/proxy-get-query",
  "summary": "Forwards query parameters to the echo endpoint via http.get's query field. The tag array demonstrates repeated parameters but is not asserted: inbound parsing collapses repeats to one value.",
  "tags": ["http"],
  "trigger": {
    "workflow": "get-query",
    "input": {
      "limit": "{{ query.limit }}",
      "cursor": "{{ query.cursor }}"
    }
  }
}
```

- [ ] **Step 4: Add the verification step**

In `examples/node-cookbook/http/verify.json`, add this object to the `steps` array, after the `http.request` step:

```json
    { "name": "http.get forwards query parameters, encoded",
      "request": { "method": "GET", "path": "/api/proxy-get-query?limit=10" },
      "expect": { "status": 200, "body": [
        { "path": "status", "equals": 200 },
        { "path": "received_query.limit", "equals": "10" },
        { "path": "received_query.note", "equals": "a&b c" }
      ] } }
```

The `note` assertion is the one that matters: the value contains `&` and a space, so it round-trips only if encoding and decoding are both correct. `cursor` is absent from the request, so it resolves to nil and is omitted — if null-omission broke, `received_query.cursor` would appear and `note` would still pass, which is why Task 2's wire test covers omission directly.

- [ ] **Step 5: Validate the project, then run the cookbook suite**

```bash
go build -o bin/noda ./cmd/noda
./bin/noda validate --config examples/node-cookbook/http
go test ./internal/testing/cookbook/ -tags integration -count=1 -run 'TestCookbook/http' -v
```
Expected: `validate` reports all config files valid; the cookbook test passes including the new step. If the harness needs Docker services, follow the cookbook README's dependency setup.

- [ ] **Step 6: Confirm the coverage gate still holds**

Run: `go test ./internal/testing/cookbook/ -tags integration -count=1 -run 'TestCookbookCoverage'`
Expected: PASS at 81/81. No new node type was added, so the count is unchanged.

- [ ] **Step 7: Commit**

```bash
git add examples/node-cookbook/http/
git commit -m "test(cookbook): CI-verified example of http.get's query field (#451)

The echo endpoint now reports the query it received, so the proxy step can
assert a value containing & and a space round-trips encoded. The tag array
demonstrates repeated parameters but is not asserted: c.Queries collapses
repeats to one value inbound."
```

---

### Task 5: Documentation and CHANGELOG

**Files:**
- Modify: `docs/03-nodes/http.get.md`, `docs/03-nodes/http.post.md`, `docs/03-nodes/http.request.md`
- Modify: `docs/04-guides/proxy-cookbook.md` (§2)
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: the finished feature. Produces nothing later tasks use.

**Context:** Every fenced `json` snippet in these files is validated *and* compiled in CI by the #445 doc-snippet gate, so a broken example fails the build. Read each file's existing config-field table before editing and match its column layout exactly.

- [ ] **Step 1: Document the field on all three node pages**

In each of `docs/03-nodes/http.get.md`, `http.post.md`, and `http.request.md`, add a `query` row to the config-field table, matching that file's existing columns. The description:

> Query parameters, URL-encoded and appended to `url`. Accepts an object or an expression resolving to one.

Then add this section to each page, after the config table:

````markdown
### Query parameters

`query` takes an object whose entries are URL-encoded and appended to `url`:

```json
{
  "type": "http.get",
  "services": { "client": "inventory" },
  "config": {
    "url": "/items",
    "query": { "limit": "{{ input.limit }}", "sort": "name" }
  }
}
```

Rules:

- Values are stringified. Arrays become repeated parameters (`tag=new&tag=sale`).
- A value that resolves to null **drops the parameter entirely**, so an optional
  parameter needs no conditional.
- Nested objects are rejected.
- If `url` already contains a query string, setting a non-empty `query` is an
  error rather than a merge — use one or the other.
- Parameters are emitted sorted by key, and spaces encode as `+`.
````

For `http.post.md` and `http.request.md`, change the `"type"` line in the snippet to `http.post` / `http.request`, and for `http.request.md` add `"method": "GET",` above `"url"` — otherwise the snippet fails the doc gate's compile check.

- [ ] **Step 2: Restore the proxy cookbook's §2**

Replace the body of `## 2. Forwarding query parameters` in `docs/04-guides/proxy-cookbook.md`. Keep the existing warning paragraph about `query.*` not being available in a node config — it is still true and is exactly why the route mapping is needed. Rewrite the section to:

````markdown
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
````

- [ ] **Step 3: Run the doc-snippet gate**

```bash
go test ./tools/docverify/... -count=1
```
Expected: PASS. If a snippet fails, it is being schema-validated and compiled — fix the snippet, not the gate. A workflow snippet needs a valid `id`, `nodes`, and `edges`, and every node's outputs must be wired.

- [ ] **Step 4: Add the CHANGELOG entry**

In `CHANGELOG.md`, add as the **first** bullet under `## [Unreleased]` → `### Added`:

```markdown
- `query` config field on `http.get`, `http.post`, and `http.request` — an object whose entries are URL-encoded and appended to `url`, so query parameters no longer have to be concatenated into the URL string by hand (#451). Hand-concatenation did no encoding, so any value containing `&`, `=`, or a space produced a malformed request. Values are stringified, arrays become repeated parameters, and a value resolving to null drops the parameter entirely — so an optional parameter such as a pagination cursor needs no conditional. Parameters are emitted sorted by key. Setting `query` when `url` already carries a query string is an error rather than a merge, so there is no precedence rule to remember. Note that `query.*` is still not available inside a node config: forward the caller's parameters by mapping `{{ query }}` into the workflow input on the route, then passing `{{ input.q }}` to the node — `docs/04-guides/proxy-cookbook.md` §2 shows the pattern.
```

- [ ] **Step 5: Commit**

```bash
git add docs/ CHANGELOG.md
git commit -m "docs: document the http.* query field and restore proxy cookbook 2 (#451)

Section 2 documented a query field that never existed; #445 rewrote it around
explicit per-parameter forwarding. It now shows wholesale forwarding again,
with the route-mapping step, and keeps the accurate warning that query.* is
not in scope inside a node config."
```

---

### Task 6: Full gate and pull request

**Files:** none modified.

- [ ] **Step 1: Run the complete local gate**

From the repo root, in order:

```bash
gofmt -l . | grep -v node_modules
go vet ./...
golangci-lint run --timeout=5m --build-tags=integration
go test ./... 
go test -tags integration ./...
govulncheck $(go list ./... | grep -v '/node_modules/')
```

All six must be clean. `golangci-lint` findings under a nonexistent `.worktrees/` path mean a stale cache — `golangci-lint cache clean`, then re-run. `govulncheck` printing nothing means the tool is broken, not that the code is clean.

- [ ] **Step 2: Push and open the PR**

```bash
git rev-parse --show-toplevel   # must be the worktree
git push -u origin feat/http-query-field
```

Open a PR titled `feat(http): query config field for http.* nodes (#451)`, with a body that:
- states `Closes #451`
- records the three decisions (config-field-only scope, reject-don't-merge, nulls omitted)
- notes the correction to the issue's premise: the workaround expression in #451 cannot work, because `query` is not in the node expression context
- explains the `oneOf` schema's editor rationale
- lists the six gate results

- [ ] **Step 3: Watch CI**

Five checks must pass: Go, Editor, Editor E2E, Wasm guests, Integration e2e. `benchmark` is not required and may remain pending, which shows as `mergeStateStatus: UNSTABLE` — that is not a blocker.

---

## Self-Review

**Spec coverage.** Every spec section maps to a task: config field and schema → Task 3; resolution/encoding table → Task 1; order of operations and merge rule → Task 2; code layout → Task 1; error handling → Tasks 1 and 2; testing (unit, integration, end-to-end) → Tasks 1, 2, 4; documentation → Task 5. The spec's three "out of scope" items are implemented nowhere, as intended.

**Placeholders.** None. Every code step carries the literal code; every test step carries the literal test.

**Type consistency.** `encodeQuery(raw any) (string, error)` and `queryScalar(key string, v any) (string, error)` are named identically in Task 1's implementation and Task 2's call site. `received_query` is spelled the same in the route trigger, the echo workflow body, the proxy workflow, and the verify assertions.

**One deviation from strict TDD, flagged where it occurs:** `TestDoRequest_NoQueryFieldLeavesURLAlone` (Task 2) passes before implementation. It is a regression guard on existing behavior, not a driver for new code, and Step 2 says so.
