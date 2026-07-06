# Edge & Trace Hardening (Tranche D) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 8 mechanical security findings from `REVIEW-FINDINGS-2026-07-05.md` (7 Medium, 1 Low) — close information-disclosure (error bodies, trace stream, dev trace endpoint) and injection/DoS (open redirect, cross-user send, image bomb) surfaces.

**Architecture:** Defensive guards across `plugins/db`, `internal/server`, `internal/trace`, `plugins/core/{response,ws,sse}`, `plugins/image`. No public API break; no behavior change for valid inputs.

**Tech Stack:** Go (go1.25), Fiber v3, gofiber/contrib/v3/websocket, bimg, expr-lang.

## Global Constraints

- Go module floor: **go1.25**.
- Production error bodies must not leak driver/DB/schema detail; `devMode` gates the detail.
- Trace redaction must handle **any** map/slice value type (incl. `[]map[string]any`), not just `map[string]any`/`[]any`.
- Dev `/ws/trace` rejects cross-origin upgrades (same-host/localhost only; empty `Origin` allowed for non-browser clients).
- `response.redirect` rejects `/\`-authority (browsers normalize `\`→`/`).
- `ws.send`/`sse.send` channels must be literal (reject `*`).
- `image.resize`/`image.crop` enforce an output-dimension ceiling (default 10000 px/side, ~40 MP), overridable per node.
- All touched packages' tests run under `-race`.
- Pre-push gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/... ./plugins/...`.

**Worktree:** `.worktrees/edge-trace-hardening`, branch `feat/edge-trace-hardening` off `main`. Spec + this plan force-added.

## File map

- `plugins/db/create.go`, `plugins/db/upsert.go` — safe `ConflictError.Reason` (Task 1).
- `internal/server/errors.go` — gate Conflict/ServiceUnavailable detail on `devMode` (Task 1).
- `internal/trace/redact.go`, `internal/trace/events.go` — reflection redactor + `stream_key` (Task 2).
- `internal/trace/websocket.go` — `/ws/trace` origin guard (Task 3).
- `plugins/core/response/redirect.go` — `/\` guard (Task 4).
- `plugins/core/ws/send.go`, `plugins/core/sse/send.go` — wildcard-channel guard (Task 5).
- `plugins/image/limits.go` *(new)*, `resize.go`, `crop.go` — dimension ceiling (Task 6).

---

### Task 1: Gate error-detail leaks (data-1 + server-2)

**Files:**
- Modify: `plugins/db/create.go` (~78-82), `plugins/db/upsert.go` (~93-97), `internal/server/errors.go`
- Test: `plugins/db/*_test.go`, `internal/server/errors_test.go`

**Interfaces:** none; error `Message` content changes.

- [ ] **Step 1: Write the failing tests**

```go
// internal/server/errors_test.go
func TestMapErrorToHTTP_ConflictGatedOnDevMode(t *testing.T) {
	cf := &api.ConflictError{Resource: "users", Reason: `duplicate key value violates unique constraint "users_email_key" (email)=(a@b.com)`}
	// production: no raw driver detail
	status, resp := MapErrorToHTTP(cf, "trace-1", false)
	require.Equal(t, 409, status)
	require.NotContains(t, resp.Error.Message, "users_email_key")
	require.NotContains(t, resp.Error.Message, "a@b.com")
	require.Contains(t, resp.Error.Message, "users") // resource name is fine
	// dev: full detail
	_, devResp := MapErrorToHTTP(cf, "trace-1", true)
	require.Contains(t, devResp.Error.Message, "users_email_key")
}

func TestMapErrorToHTTP_ServiceUnavailableGatedOnDevMode(t *testing.T) {
	su := &api.ServiceUnavailableError{Service: "db", Cause: errors.New("dial tcp 10.0.0.5:5432: connection refused")}
	_, resp := MapErrorToHTTP(su, "t", false)
	require.NotContains(t, resp.Error.Message, "10.0.0.5")
	_, devResp := MapErrorToHTTP(su, "t", true)
	require.Contains(t, devResp.Error.Message, "10.0.0.5")
}
```

```go
// plugins/db/create_test.go (add) — the db plugin must not embed the raw driver string.
func TestConflictError_ReasonIsSafe(t *testing.T) {
	// Drive db.create against a duplicate-key error and assert the returned
	// *api.ConflictError.Reason == "unique constraint violation" (not the raw errMsg).
	// Use the existing db test harness / a fake gorm error containing "duplicate key".
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/server/ -run TestMapErrorToHTTP_Conflict -race`
Expected: FAIL — prod message currently contains `users_email_key` / `a@b.com` (returns `cfErr.Error()` verbatim).

- [ ] **Step 3: Safe reason in db plugin + gate in server**

In `plugins/db/create.go` (~79-82) and `upsert.go` (~94-97), change `Reason: errMsg` → `Reason: "unique constraint violation"`. Keep `Resource: table`.

In `internal/server/errors.go MapErrorToHTTP`, change the `ConflictError` and `ServiceUnavailableError` branches to build the message from safe fields in production:

```go
	case errors.As(err, &cfErr):
		status = 409
		msg := fmt.Sprintf("conflict on %s", cfErr.Resource)
		if devMode {
			msg = cfErr.Error()
		}
		resp = ErrorResponse{Error: api.ErrorData{Code: "CONFLICT", Message: msg, TraceID: traceID}}
	case errors.As(err, &suErr):
		status = 503
		msg := fmt.Sprintf("service unavailable: %s", suErr.Service)
		if devMode {
			msg = suErr.Error()
		}
		resp = ErrorResponse{Error: api.ErrorData{Code: "SERVICE_UNAVAILABLE", Message: msg, TraceID: traceID}}
```

Add `"fmt"` to `errors.go` imports if not present. (ValidationError/NotFound/Timeout branches unchanged.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test -race ./internal/server/ ./plugins/db/ -run 'TestMapErrorToHTTP|TestConflictError_ReasonIsSafe'`
Expected: PASS.

- [ ] **Step 5: Full suites**

Run: `go test -race ./internal/server/... ./plugins/db/...`
Expected: PASS (existing conflict tests that asserted the old message updated to the new prod/dev split).

- [ ] **Step 6: Commit**

```bash
git add plugins/db/create.go plugins/db/upsert.go internal/server/errors.go internal/server/errors_test.go plugins/db/create_test.go
git commit -m "fix(security): gate DB conflict/unavailable error detail behind dev mode (data-1, server-2)"
```

---

### Task 2: Reflection-based trace redactor + stream_key (realtime-2 + realtime-3)

**Files:**
- Modify: `internal/trace/redact.go`, `internal/trace/events.go`
- Test: `internal/trace/redact_test.go`

**Interfaces:**
- Produces: `redactValue(v any) any` (recursively redacts any map/slice/scalar).

- [ ] **Step 1: Write the failing tests**

```go
// redact_test.go
func TestRedactValue_TypedSliceOfMaps(t *testing.T) {
	in := []map[string]any{
		{"id": 1, "password": "hunter2"},
		{"id": 2, "api_key": "sk-abc"},
	}
	out := redactValue(in).([]any)
	require.Equal(t, "[REDACTED]", out[0].(map[string]any)["password"])
	require.Equal(t, "[REDACTED]", out[1].(map[string]any)["api_key"])
	require.Equal(t, 1, out[0].(map[string]any)["id"])
}

func TestRedactValue_StreamKey(t *testing.T) {
	out := redactValue(map[string]any{"stream_key": "live_xyz", "room": "r1"}).(map[string]any)
	require.Equal(t, "[REDACTED]", out["stream_key"])
	require.Equal(t, "r1", out["room"])
}

func TestEmit_RedactsSliceData(t *testing.T) {
	hub := NewEventHub()
	got := make(chan []byte, 1)
	unsub := hub.Subscribe(func(b []byte) { got <- b })
	defer unsub()
	hub.Emit(Event{Type: "node.completed", Data: []map[string]any{{"password": "p"}}})
	select {
	case b := <-got:
		require.NotContains(t, string(b), "\"p\"")
		require.Contains(t, string(b), "[REDACTED]")
	case <-time.After(time.Second):
		t.Fatal("no event")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/trace/ -run 'TestRedactValue_TypedSliceOfMaps|TestRedactValue_StreamKey|TestEmit_RedactsSliceData' -race`
Expected: FAIL — `redactValue` undefined; `stream_key` not redacted; Emit doesn't redact `[]map[string]any`.

- [ ] **Step 3: Add `stream_key` and the reflection redactor**

In `redact.go`, extend `sensitiveContains`:

```go
var sensitiveContains = []string{
	"password", "secret", "token", "authorization", "credential",
	"api_key", "apikey", "stream_key", "signing_key", "private_key",
}
```

Add a reflection-based redactor and rewrite `redactSecrets`/`redactSlice` value-recursion to go through it. Add `"reflect"` to imports.

```go
const maxRedactDepth = 32

// redactValue returns a deep, redacted copy of any value. Maps (string-keyed)
// have sensitive keys replaced and values recursed; slices/arrays of any
// element type are recursed element-wise; scalars pass through. This handles
// concretely-typed values like []map[string]any (db.query results) that the
// old type switch missed.
func redactValue(v any) any { return redactValueDepth(v, 0) }

func redactValueDepth(v any, depth int) any {
	if depth > maxRedactDepth {
		return v
	}
	switch val := v.(type) {
	case nil:
		return nil
	case map[string]any:
		return redactStringMap(val, depth)
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return v // non-string keys: can't classify; leave as-is
		}
		out := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			k := iter.Key().String()
			if IsSensitiveKey(k) {
				out[k] = "[REDACTED]"
			} else {
				out[k] = redactValueDepth(iter.Value().Interface(), depth+1)
			}
		}
		return out
	case reflect.Slice, reflect.Array:
		if rv.IsNil() {
			return v
		}
		n := rv.Len()
		out := make([]any, n)
		for i := 0; i < n; i++ {
			out[i] = redactValueDepth(rv.Index(i).Interface(), depth+1)
		}
		return out
	default:
		return v
	}
}

// redactStringMap preserves the existing map[string]any behavior, including the
// narrow cookie-container redaction, but recurses values through redactValueDepth.
func redactStringMap(m map[string]any, depth int) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if IsSensitiveKey(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = redactValueDepth(v, depth+1)
		if isCookieContainerKey(k) {
			if inner, ok := out[k].(map[string]any); ok {
				redactCookieValue(inner)
			}
		}
	}
	return out
}
```

Replace `redactSecrets`'s body with `return redactStringMap(m, 0)` (keep the name for existing callers such as `redactHTTPResponse`), and make `redactHTTPResponse`'s Body switch use `redactValue(body)` for the non-`map`/generic case. Delete the now-unused `redactSlice` if nothing else calls it (grep first; `redactHTTPResponse` used it for `[]any` Body — replace with `redactValue`).

In `events.go Emit`, replace the type switch:

```go
	switch data := event.Data.(type) {
	case *api.HTTPResponse:
		event.Data = redactHTTPResponse(data)
	default:
		event.Data = redactValue(data)
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/trace/ -run 'TestRedactValue|TestEmit_RedactsSliceData' -race`
Expected: PASS.

- [ ] **Step 5: Full trace suite**

Run: `go test ./internal/trace/... -race`
Expected: PASS (existing redaction/cookie tests still green — the map/cookie behavior is preserved).

- [ ] **Step 6: Commit**

```bash
git add internal/trace/redact.go internal/trace/events.go internal/trace/redact_test.go
git commit -m "fix(security): reflection-based trace redaction + stream_key (realtime-2, realtime-3)"
```

---

### Task 3: Dev /ws/trace Origin guard (realtime-4)

**Files:**
- Modify: `internal/trace/websocket.go`
- Test: `internal/trace/websocket_test.go` (create if absent)

- [ ] **Step 1: Write the failing test**

```go
// websocket_test.go
func TestTraceWebSocket_RejectsCrossOrigin(t *testing.T) {
	app := fiber.New()
	RegisterTraceWebSocket(app, NewEventHub(), slog.Default())

	// cross-origin upgrade attempt → 403 (never reaches the ws handler)
	req := httptest.NewRequest("GET", "/ws/trace", nil)
	req.Header.Set("Origin", "http://evil.com")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	resp, err := app.Test(req)
	require.NoError(t, err)
	require.Equal(t, 403, resp.StatusCode)

	// same-host origin → not 403 (426/101/etc. — upgrade proceeds past the guard)
	req2 := httptest.NewRequest("GET", "/ws/trace", nil)
	req2.Header.Set("Origin", "http://example.com")
	req2.Host = "example.com"
	resp2, _ := app.Test(req2)
	require.NotEqual(t, 403, resp2.StatusCode)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/trace/ -run TestTraceWebSocket_RejectsCrossOrigin`
Expected: FAIL — cross-origin returns an upgrade status, not 403.

- [ ] **Step 3: Add the origin guard before the websocket handler**

In `websocket.go`, register `/ws/trace` with a guard handler first:

```go
func RegisterTraceWebSocket(app *fiber.App, hub *EventHub, logger *slog.Logger) {
	app.Get("/ws/trace", traceOriginGuard, websocket.New(func(c *websocket.Conn) {
		// ... existing handler body unchanged ...
	}))
}

// traceOriginGuard rejects cross-origin WebSocket upgrades to the dev trace
// stream (which carries workflow inputs, DB rows, and secrets). An empty Origin
// (non-browser clients like the CLI) is allowed; browser origins must be
// same-host or localhost.
func traceOriginGuard(c fiber.Ctx) error {
	origin := c.Get("Origin")
	if origin == "" || originAllowed(origin, c.Hostname()) {
		return c.Next()
	}
	return c.SendStatus(fiber.StatusForbidden)
}

func originAllowed(origin, host string) bool {
	u, err := neturl.Parse(origin)
	if err != nil {
		return false
	}
	oh := u.Hostname()
	if oh == host {
		return true
	}
	return oh == "localhost" || oh == "127.0.0.1" || oh == "::1"
}
```

Add imports `neturl "net/url"` and ensure `fiber` is imported. Confirm the fiber v3 multi-handler route signature (`app.Get(path, handlers ...fiber.Handler)`) and that `c.Next()` proceeds to the websocket handler; adjust if the contrib websocket requires being the sole handler (in that case, wrap: register the guard as `app.Use("/ws/trace", traceOriginGuard)` before the `Get`). Add a short comment noting dev mode binds loopback.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/trace/ -run TestTraceWebSocket_RejectsCrossOrigin`
Expected: PASS.

- [ ] **Step 5: Full trace suite**

Run: `go test ./internal/trace/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/trace/websocket.go internal/trace/websocket_test.go
git commit -m "fix(security): reject cross-origin upgrades on dev /ws/trace (realtime-4)"
```

---

### Task 4: Open-redirect `/\` guard (nodes-1)

**Files:**
- Modify: `plugins/core/response/redirect.go`
- Test: `plugins/core/response/*_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestRedirect_RejectsBackslashAuthority(t *testing.T) {
	for _, bad := range []string{`/\evil.com`, `/\\evil.com`, `//evil.com`} {
		_, _, err := runRedirect(t, bad) // small helper: build execCtx, call redirectExecutor.Execute
		require.Error(t, err, "must reject %q", bad)
	}
	for _, ok := range []string{`/dashboard`, `https://example.com/x`, `http://example.com`} {
		_, _, err := runRedirect(t, ok)
		require.NoError(t, err, "must allow %q", ok)
	}
}
```

(If a redirect test helper doesn't exist, resolve a literal URL by passing `config["url"]` as a non-expression string and a minimal `NewExecutionContext()`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/core/response/ -run TestRedirect_RejectsBackslashAuthority`
Expected: FAIL — `/\evil.com` currently passes (starts with `/`).

- [ ] **Step 3: Reject backslash authority**

In `redirect.go`, replace the `strings.HasPrefix(urlStr, "//")` protocol-relative check with a check covering `/` and `\` as the second byte:

```go
	// Reject open redirect via protocol-relative URLs. Browsers normalize
	// backslashes to forward slashes, so "/\evil.com" is also protocol-relative.
	if len(urlStr) >= 2 && urlStr[0] == '/' && (urlStr[1] == '/' || urlStr[1] == '\\') {
		return "", nil, fmt.Errorf("response.redirect: url must start with /, http://, or https://")
	}
```

Keep the existing CRLF check and the `!HasPrefix("/") && !HasPrefix("http://") && !HasPrefix("https://")` scheme check.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugins/core/response/ -run TestRedirect_RejectsBackslashAuthority`
Expected: PASS.

- [ ] **Step 5: Full response suite**

Run: `go test ./plugins/core/response/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add plugins/core/response/redirect.go plugins/core/response/redirect_test.go
git commit -m "fix(security): reject backslash-authority open redirect (nodes-1)"
```

---

### Task 5: Wildcard-channel send guard (nodes-2)

**Files:**
- Modify: `plugins/core/ws/send.go`, `plugins/core/sse/send.go`
- Test: `plugins/core/ws/*_test.go`, `plugins/core/sse/*_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// ws/send_test.go (and mirror in sse/send_test.go with sse.send)
func TestWSSend_RejectsWildcardChannel(t *testing.T) {
	for _, bad := range []string{"*", "user.*", "a*b"} {
		_, _, err := runWSSend(t, bad) // helper: config{channel: bad, data: {...}} + fake ConnectionService
		require.Error(t, err, "must reject wildcard channel %q", bad)
	}
	_, _, err := runWSSend(t, "user.123")
	require.NoError(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./plugins/core/ws/ ./plugins/core/sse/ -run RejectsWildcardChannel`
Expected: FAIL — wildcard channel is sent through to the matcher.

- [ ] **Step 3: Reject `*` in the resolved channel**

In `plugins/core/ws/send.go`, after `channel, err := plugin.ResolveString(...)`:

```go
	if strings.Contains(channel, "*") {
		return "", nil, fmt.Errorf("ws.send: channel must be a literal name, not a pattern")
	}
```

Same in `plugins/core/sse/send.go` with `sse.send:` prefix. Add `"strings"` to imports where missing.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./plugins/core/ws/ ./plugins/core/sse/ -run RejectsWildcardChannel -race`
Expected: PASS.

- [ ] **Step 5: Full ws+sse suites**

Run: `go test -race ./plugins/core/ws/... ./plugins/core/sse/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add plugins/core/ws/send.go plugins/core/sse/send.go plugins/core/ws/send_test.go plugins/core/sse/send_test.go
git commit -m "fix(security): ws.send/sse.send reject wildcard channels (nodes-2)"
```

---

### Task 6: Image output-dimension ceiling (edge-io-1)

**Files:**
- Create: `plugins/image/limits.go`
- Modify: `plugins/image/resize.go`, `plugins/image/crop.go`
- Test: `plugins/image/*_test.go`

**Interfaces:**
- Produces: `enforceDimensionLimit(nCtx api.ExecutionContext, config map[string]any, width, height int) error`.

- [ ] **Step 1: Write the failing test**

```go
func TestEnforceDimensionLimit(t *testing.T) {
	c := makeImageCtx(t) // minimal execution context
	require.NoError(t, enforceDimensionLimit(c, map[string]any{}, 800, 600))
	require.Error(t, enforceDimensionLimit(c, map[string]any{}, 100000, 600))     // > maxWidth
	require.Error(t, enforceDimensionLimit(c, map[string]any{}, 9000, 9000))      // > maxPixels (81MP)
	// per-node override raises the cap
	require.NoError(t, enforceDimensionLimit(c, map[string]any{"max_width": float64(200000), "max_pixels": float64(1 << 40)}, 100000, 600))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./plugins/image/ -run TestEnforceDimensionLimit`
Expected: FAIL — `enforceDimensionLimit` undefined.

- [ ] **Step 3: Add the limit helper and call it**

`plugins/image/limits.go`:

```go
package image

import (
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
)

const (
	defaultMaxDimension = 10000
	defaultMaxPixels    = 40_000_000 // ~40 megapixels
)

// enforceDimensionLimit rejects output dimensions that exceed the per-side or
// total-pixel ceiling, preventing decompression/allocation bombs. Caps default
// to 10000 px/side and 40 MP, overridable via max_width/max_height/max_pixels.
func enforceDimensionLimit(nCtx api.ExecutionContext, config map[string]any, width, height int) error {
	maxW := defaultMaxDimension
	if v, ok, _ := plugin.ResolveOptionalInt(nCtx, config, "max_width"); ok {
		maxW = v
	}
	maxH := defaultMaxDimension
	if v, ok, _ := plugin.ResolveOptionalInt(nCtx, config, "max_height"); ok {
		maxH = v
	}
	maxPx := int64(defaultMaxPixels)
	if v, ok, _ := plugin.ResolveOptionalInt(nCtx, config, "max_pixels"); ok {
		maxPx = int64(v)
	}
	if width > maxW || height > maxH {
		return fmt.Errorf("output dimensions %dx%d exceed limit (%dx%d)", width, height, maxW, maxH)
	}
	if int64(width)*int64(height) > maxPx {
		return fmt.Errorf("output area %d px exceeds limit (%d px)", int64(width)*int64(height), maxPx)
	}
	return nil
}
```

In `resize.go`, after resolving `width`/`height` and before building `bimg.Options`:

```go
	if err := enforceDimensionLimit(nCtx, config, width, height); err != nil {
		return "", nil, fmt.Errorf("image.resize: %w", err)
	}
```

Same in `crop.go` with `image.crop:` prefix (after its width/height resolution ~53).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./plugins/image/ -run TestEnforceDimensionLimit`
Expected: PASS.

- [ ] **Step 5: Full image suite**

Run: `go test ./plugins/image/... -race`
Expected: PASS. (Note: bimg tests may need libvips; if the environment lacks it and existing image tests are skipped/gated, keep `enforceDimensionLimit` and its unit test independent of bimg so they run regardless.)

- [ ] **Step 6: Commit**

```bash
git add plugins/image/limits.go plugins/image/resize.go plugins/image/crop.go plugins/image/limits_test.go
git commit -m "fix(security): cap image.resize/crop output dimensions (edge-io-1)"
```

---

### Task 7: CHANGELOG + full gate

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: CHANGELOG entry**

Add under a `### Security` heading: "Edge & trace hardening: DB conflict/unavailable error bodies no longer leak driver/constraint detail in production; trace redaction now covers slice-typed node data (e.g. `db.query` rows) and `stream_key`; the dev `/ws/trace` endpoint rejects cross-origin connections; `response.redirect` rejects `/\`-authority open redirects; `ws.send`/`sse.send` reject wildcard channels; `image.resize`/`image.crop` cap output dimensions."

- [ ] **Step 2: Full gate**

Run: `gofmt -l . && go vet ./... && golangci-lint run && go test -race ./internal/... ./plugins/...`
Expected: clean, all pass. Fix any lint issues introduced by this branch; leave pre-existing/unrelated ones (note them). If the image package requires libvips and isn't buildable in the environment, note that `enforceDimensionLimit`'s unit test still runs (pure Go).

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(security): changelog for edge & trace hardening"
```

---

## Self-review notes

- **Spec coverage:** data-1+server-2 → Task 1; realtime-2+realtime-3 → Task 2; realtime-4 → Task 3; nodes-1 → Task 4; nodes-2 → Task 5; edge-io-1 → Task 6; changelog/gate → Task 7. All 8 covered.
- **Type consistency:** `redactValue(any) any` (Task 2) used by `Emit` and `redactHTTPResponse`. `enforceDimensionLimit(nCtx, config, w, h) error` (Task 6) used by resize + crop. `MapErrorToHTTP(err, traceID, devMode)` signature unchanged (Task 1 only changes branch bodies).
- **Risk notes:** Task 2 preserves the map/cookie-container behavior (existing tests guard it). Task 3 must confirm fiber v3's multi-handler route wiring for the contrib websocket (fallback: `app.Use` the guard). Task 6 keeps the limit check pure-Go so it tests without libvips.
- **Deferred (out of scope):** auth-1/auth-2 (separate auth-anti-enumeration tranche); realtime-1/5/6, edge-io-2/3/4, and other findings not in this set.
