# Trigger Input Coercion (#331) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** HTTP trigger inputs are numerically coerced only when sourced from string-typed transports (path params, query, headers, form bodies), never from JSON bodies, computed expressions, or literals; a per-trigger `"coerce": false` disables coercion entirely. Fixes #331.

**Architecture:** All changes concentrate in `MapTrigger` (`internal/server/trigger.go`) — a regex classifies each input expression as a bare transport reference; `coerceNumeric` is applied only per the decision table in `docs/superpowers/specs/2026-07-16-trigger-input-coercion-design.md`. The route config schema gains a `trigger.coerce` boolean.

**Tech Stack:** Go, gofiber/fiber/v3, stdlib `regexp`. Tests use the existing `triggerTest` helper idiom in `internal/server/trigger_test.go` (fiber app + `httptest`).

## Global Constraints

- Branch: `feat/trigger-input-coercion`, worktree `.worktrees/trigger-input-coercion`, cut from `origin/main` **after `git fetch`** (never local main).
- Before every commit: `gofmt -l .` from the **repo root** must print nothing; `go vet ./...` clean.
- Commit messages end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- `coerceNumeric` itself is unchanged; only *when it is applied* changes.
- Do not touch worker/scheduler/ws trigger paths — they never coerced and must not start.

---

### Task 1: Source-based coercion in MapTrigger

**Files:**
- Modify: `internal/server/trigger.go` (loop body around line 79-84; new helper + regex at bottom near `coerceNumeric`)
- Test: `internal/server/trigger_test.go`

**Interfaces:**
- Consumes: existing `triggerTest(t, method, path, body, headers, triggerCfg)` helper (`trigger_test.go:17`) — it always sends JSON content type when `body != nil`.
- Produces: unexported `shouldCoerce(exprStr string, bodyStringTyped bool) bool` used only inside `MapTrigger`. Trigger config key `"coerce"` (bool, default true).

- [ ] **Step 1: Write the failing tests**

Add to `internal/server/trigger_test.go`. Also add a raw-body helper (the existing one hard-codes JSON):

```go
// triggerTestRaw is like triggerTest but sends a raw body with an explicit content type.
func triggerTestRaw(t *testing.T, method, path, rawBody, contentType string, triggerCfg map[string]any) *TriggerResult {
	t.Helper()

	app := fiber.New()
	compiler := expr.NewCompilerWithFunctions()

	var result *TriggerResult
	var triggerErr error

	app.All("/test/:id?", func(c fiber.Ctx) error {
		result, triggerErr = MapTrigger(c, triggerCfg, compiler)
		if triggerErr != nil {
			return c.Status(500).SendString(triggerErr.Error())
		}
		return c.SendString("ok")
	})

	req := httptest.NewRequest(method, path, strings.NewReader(rawBody))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.NoError(t, triggerErr)

	return result
}

func TestMapTrigger_JSONBodyStringsNotCoerced(t *testing.T) {
	// #331: JSON preserves types — numeric-looking strings must stay strings.
	result := triggerTest(t, "POST", "/test", map[string]any{"zip": "0042", "count": 7}, nil, map[string]any{
		"input": map[string]any{
			"zip":   "{{ body.zip }}",
			"count": "{{ body.count }}",
		},
	})
	assert.Equal(t, "0042", result.Input["zip"])
	assert.Equal(t, float64(7), result.Input["count"]) // JSON number passes through untouched
}

func TestMapTrigger_FormBodyStringsCoerced(t *testing.T) {
	// Form bodies are string-typed transport — coercion keeps working there.
	result := triggerTestRaw(t, "POST", "/test", "amount=42&note=0042x",
		"application/x-www-form-urlencoded", map[string]any{
			"input": map[string]any{
				"amount": "{{ body.amount }}",
				"note":   "{{ body.note }}",
			},
		})
	assert.Equal(t, 42, result.Input["amount"])
	assert.Equal(t, "0042x", result.Input["note"])
}

func TestMapTrigger_TransportRefsStillCoerced(t *testing.T) {
	result := triggerTest(t, "GET", "/test/0042?page=2", nil, map[string]string{
		"X-Page-Size": "50",
	}, map[string]any{
		"input": map[string]any{
			"id":    "{{ params.id }}",
			"page":  "{{ query.page }}",
			"size":  "{{ headers['X-Page-Size'] }}",
			"page2": "{{ request.query.page }}",
		},
	})
	assert.Equal(t, 42, result.Input["id"])
	assert.Equal(t, 2, result.Input["page"])
	assert.Equal(t, 50, result.Input["size"])
	assert.Equal(t, 2, result.Input["page2"])
}

func TestMapTrigger_ComputedAndLiteralNotCoerced(t *testing.T) {
	result := triggerTest(t, "GET", "/test?a=4", nil, nil, map[string]any{
		"input": map[string]any{
			"lit":  "9",                     // literal string: author's type wins
			"comp": "{{ query.a + \"1\" }}", // computed: expression's result type wins
		},
	})
	assert.Equal(t, "9", result.Input["lit"])
	assert.Equal(t, "41", result.Input["comp"])
}

func TestMapTrigger_CoerceOptOut(t *testing.T) {
	result := triggerTest(t, "GET", "/test/0042?page=2", nil, nil, map[string]any{
		"coerce": false,
		"input": map[string]any{
			"id":   "{{ params.id }}",
			"page": "{{ query.page }}",
		},
	})
	assert.Equal(t, "0042", result.Input["id"])
	assert.Equal(t, "2", result.Input["page"])
}
```

Note: if `{{ headers['X-Page-Size'] }}` single-quote syntax fails to compile in expr-lang, use `{{ headers[\"X-Page-Size\"] }}` — verify against `internal/expr` compiler tests for the bracket idiom.

- [ ] **Step 2: Run tests to verify the new ones fail**

Run: `go test ./internal/server/ -run 'TestMapTrigger_(JSONBody|FormBody|TransportRefs|ComputedAndLiteral|CoerceOptOut)' -v`
Expected: `TestMapTrigger_JSONBodyStringsNotCoerced`, `TestMapTrigger_ComputedAndLiteralNotCoerced`, `TestMapTrigger_CoerceOptOut` FAIL (values coerced to numbers today); the form/transport tests PASS (that behavior already exists — they are regression pins).

- [ ] **Step 3: Implement source-based coercion**

In `internal/server/trigger.go`, add `"regexp"` to imports. Replace the loop tail (currently `result.Input[key] = coerceNumeric(resolved)` at line 83) and add the helper. The `coerceEnabled`/`bodyStringTyped` reads go right before the `for key, exprVal := range inputMap` loop:

```go
		// Coercion policy (#331): only bare references into string-typed
		// transports are numerically coerced. "coerce": false disables it.
		coerceEnabled := true
		if v, ok := triggerConfig["coerce"].(bool); ok {
			coerceEnabled = v
		}
		bodyStringTyped := strings.Contains(c.Get("Content-Type"), "form")
```

Loop tail replacement:

```go
			resolved, err := resolver.Resolve(exprStr)
			if err != nil {
				return nil, fmt.Errorf("trigger mapping: field %q: %w", key, err)
			}
			if coerceEnabled && shouldCoerce(exprStr, bodyStringTyped) {
				resolved = coerceNumeric(resolved)
			}
			result.Input[key] = resolved
```

Helper + regex (place directly above `coerceNumeric`):

```go
// transportRef matches input expressions that are a single bare member-access
// reference into a transport namespace: {{ params.x }}, {{ query.x }},
// {{ headers["X-Y"] }}, {{ body.x }}, and their request.* aliases. Computed
// expressions and literals never match — their result type is authoritative.
var transportRef = regexp.MustCompile(`^\{\{\s*(?:request\.)?(params|query|headers|body)(?:\.[A-Za-z_][A-Za-z0-9_]*|\[[^\]{}]+\])+\s*\}\}$`)

// shouldCoerce reports whether a trigger-input expression's resolved value
// should go through coerceNumeric. params/query/headers always arrive as
// strings; body values are string-typed only for form-encoded requests (#331).
func shouldCoerce(exprStr string, bodyStringTyped bool) bool {
	m := transportRef.FindStringSubmatch(strings.TrimSpace(exprStr))
	if m == nil {
		return false
	}
	if m[1] == "body" {
		return bodyStringTyped
	}
	return true
}
```

Update the `coerceNumeric` doc comment's second sentence to: `Applied only to bare transport references — see shouldCoerce.`

- [ ] **Step 4: Run the package tests**

Run: `go test ./internal/server/ -count=1`
Expected: PASS. If pre-existing tests assert coerced JSON-body or literal values (check `TestMapTrigger_BodyMapping`, `TestMapTrigger_StaticValues`, `TestMapTrigger_DefaultValues`), update those assertions to the new (correct) semantics and note each in the commit message.

- [ ] **Step 5: Commit**

```bash
gofmt -l .   # must be empty, run from repo root
go vet ./internal/server/
git add internal/server/trigger.go internal/server/trigger_test.go
git commit -m "fix(server): coerce trigger inputs by transport source, add coerce opt-out (#331)"
```

---

### Task 2: Config schema + docs + CHANGELOG

**Files:**
- Modify: `internal/config/schemas/route.json` (trigger.properties)
- Modify: `docs/02-config/routes.md` (trigger field table, ~line 24-29)
- Modify: `docs/01-getting-started/realtime.md` (pitfall callout at ~line 132 referencing #331)
- Modify: `CHANGELOG.md` (`[Unreleased]`)
- Test: `internal/config/validator_test.go`

**Interfaces:**
- Consumes: Task 1's `"coerce"` trigger key semantics.
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the failing schema test**

In `internal/config/validator_test.go`, add (follow the file's existing table/validate idiom — construct a minimal route map and run it through the same schema-validation path as neighboring tests):

```go
func TestRouteSchema_TriggerCoerceFlag(t *testing.T) {
	route := map[string]any{
		"id":     "r1",
		"method": "GET",
		"path":   "/things/:id",
		"trigger": map[string]any{
			"workflow": "wf1",
			"coerce":   false,
		},
	}
	errs := validateAgainstSchema("route.json", "routes/r1.json", route)
	assert.Empty(t, errs)

	route["trigger"].(map[string]any)["coerce"] = "no"
	errs = validateAgainstSchema("route.json", "routes/r1.json", route)
	assert.NotEmpty(t, errs, "non-boolean coerce must be rejected")
}
```

- [ ] **Step 2: Run it — the negative half fails**

Run: `go test ./internal/config/ -run TestRouteSchema_TriggerCoerceFlag -v`
Expected: FAIL on the `"no"` assertion (trigger has no `additionalProperties: false`, so today the string is accepted silently as an unknown key; declaring the property adds the type check).

- [ ] **Step 3: Add the property to route.json**

In `internal/config/schemas/route.json`, inside `trigger.properties` (next to `raw_body`):

```json
"coerce": {
  "type": "boolean",
  "description": "Numeric coercion of trigger inputs sourced from string-typed transports (params/query/headers/form body). Default true."
}
```

- [ ] **Step 4: Run the test — passes**

Run: `go test ./internal/config/ -count=1`
Expected: PASS.

- [ ] **Step 5: Update docs**

`docs/02-config/routes.md`: in the trigger field table after the `trigger.files` row, add:

```markdown
| `trigger.coerce` | boolean | no | Numeric coercion of string-typed trigger inputs (default `true`). Set `false` to keep numeric-looking path/query/header/form values as strings. |
```

Below the "Trigger input sources" line, add:

```markdown
**Numeric coercion:** inputs that are a single bare reference to a string-typed transport — `params.*`, `query.*`, `headers.*`, or `body.*` for form-encoded requests (plus `request.*` aliases) — are converted to numbers when they parse as one (`{{ query.limit }}` → `10`). JSON body values keep their JSON types, and computed expressions and literal values are never coerced. Set `"coerce": false` on the trigger when IDs like `"0042"` must stay strings.
```

`docs/01-getting-started/realtime.md` ~line 132: replace the `> **Numeric-string coercion pitfall:** …` blockquote (it references the now-fixed #331) with:

```markdown
> **Numeric-string coercion:** path params are string-typed transport, so `/api/board/42/messages` delivers `input.room_id` as the number `42` by default. If `room_id` must stay a string (TEXT column, leading zeros), set `"coerce": false` on the route trigger, or convert explicitly with `{{ string(input.room_id) }}`. JSON body values always keep their JSON types.
```

`CHANGELOG.md` under `[Unreleased]` (create `### Fixed` / `### Changed` subsections if absent, merge into existing ones if present):

```markdown
### Fixed
- Trigger inputs sourced from JSON bodies keep their JSON types; numeric coercion now applies only to bare references into string-typed transports (path params, query, headers, form bodies) (#331).

### Changed
- New `trigger.coerce` route option (default `true`) disables trigger-input numeric coercion per route. Literal and computed trigger-input values are no longer coerced.
```

- [ ] **Step 6: Commit**

```bash
gofmt -l .   # empty
git add internal/config/schemas/route.json internal/config/validator_test.go docs/02-config/routes.md docs/01-getting-started/realtime.md CHANGELOG.md
git commit -m "docs+config: trigger.coerce flag, source-based coercion docs (#331)"
```

---

### Task 3: Full-suite verification and PR

**Files:**
- No new files; fixes only where the suite reveals fallout (most likely: e2e or example-driven tests asserting coerced JSON-body values).

- [ ] **Step 1: Run the full suite**

Run: `go build ./... && go test ./... -count=1`
Expected: PASS. Any failure that asserts a JSON-body/literal input arrived as a number is the old bug encoded in a test — update the assertion to the new semantics and record it. Any other failure: stop and investigate (systematic-debugging), do not paper over.

- [ ] **Step 2: Lint gate**

Run: `gofmt -l .` (empty) and `golangci-lint run ./...` if installed locally (CI runs it).
Expected: clean.

- [ ] **Step 3: Commit spec+plan records and open PR**

```bash
git add -f docs/superpowers/specs/2026-07-16-trigger-input-coercion-design.md docs/superpowers/plans/2026-07-16-trigger-input-coercion.md
git commit -m "docs(superpowers): trigger-input-coercion spec + plan records"
git push -u origin feat/trigger-input-coercion
gh pr create --title "fix(server): source-based trigger input coercion + coerce opt-out" --body "Fixes #331. ..."
```

PR body must include: the decision table from the spec, the behavior-changes list, and end with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
