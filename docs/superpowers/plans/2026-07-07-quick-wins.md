# Quick-Wins Tranche Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the six triaged quick-win issues (#264 #266 #269 #271 #280 #281) — five small runtime/security bugs plus two broken example modules — per `docs/superpowers/specs/2026-07-07-quick-wins-design.md`.

**Architecture:** Six independent point fixes in `internal/wasm`, `internal/server`, `internal/trace`, and two example guest modules. No new packages; one small extraction in `routes.go` (`awaitWorkflowResponse`) to make the select/drain behavior deterministically testable. TDD per task.

**Tech Stack:** Go, Fiber v3 (`fiber.Ctx` interface), testify, tinygo (example guest modules only).

## Global Constraints

- Branch `quick-wins` in worktree `.worktrees/quick-wins`, off `main` (`affbc9c`).
- Every commit gate: `gofmt -l .` clean on touched files, `go vet ./...`, tests green.
- Final gate additionally: `golangci-lint run`, `go test -race ./internal/wasm/... ./internal/trace/... ./internal/server/...`, tinygo builds of wasm-counter, discord-bot, wasm-helpers (control).
- Commit messages end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Test style: testify `require`/`assert` where files already use it; plain `t.Fatalf` where they don't. Match each file.

---

### Task 1: #264 — `parseReconnectConfig` codec coercion

**Files:**
- Modify: `internal/wasm/gateway.go:336-352`
- Test: `internal/wasm/gateway_test.go`

**Interfaces:**
- Consumes: `toInt64(v any) (int64, bool)`, `toFloat(v any) (float64, bool)` from `internal/wasm/coerce.go`.
- Produces: unchanged signature `parseReconnectConfig(m map[string]any) *ReconnectConfig`.

- [ ] **Step 1: Write the failing test** (append to `internal/wasm/gateway_test.go`):

```go
// parseReconnectConfig must accept msgpack-decoded numerics: msgpack picks
// the narrowest integer type (int8/int64), not float64 like JSON. With raw
// .(float64) assertions the values silently coerce to zero and reconnection
// is disabled (#264).
func TestParseReconnectConfig_MsgpackIntegerWidths(t *testing.T) {
	rc := parseReconnectConfig(map[string]any{
		"enabled":       true,
		"max_attempts":  int8(5),
		"backoff":       "exponential",
		"initial_delay": int64(250),
	})
	assert.True(t, rc.Enabled)
	assert.Equal(t, 5, rc.MaxAttempts)
	assert.Equal(t, "exponential", rc.Backoff)
	assert.Equal(t, 250*time.Millisecond, rc.InitialDelay)
}

func TestParseReconnectConfig_JSONFloats(t *testing.T) {
	rc := parseReconnectConfig(map[string]any{
		"enabled":       true,
		"max_attempts":  float64(3),
		"initial_delay": float64(100),
	})
	assert.Equal(t, 3, rc.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, rc.InitialDelay)
}
```

- [ ] **Step 2:** Run `go test ./internal/wasm/ -run TestParseReconnectConfig -v` — expect `TestParseReconnectConfig_MsgpackIntegerWidths` FAIL (`MaxAttempts` 0, `InitialDelay` 0), JSON case PASS.

- [ ] **Step 3: Fix** — in `parseReconnectConfig`, replace the two raw assertions:

```go
	if v, ok := toInt64(m["max_attempts"]); ok {
		rc.MaxAttempts = int(v)
	}
	if v, ok := m["backoff"].(string); ok {
		rc.Backoff = v
	}
	if v, ok := toFloat(m["initial_delay"]); ok {
		rc.InitialDelay = time.Duration(v) * time.Millisecond
	}
```

- [ ] **Step 4:** Re-run — both PASS. Also `go test ./internal/wasm/` full package.
- [ ] **Step 5:** Commit `fix(wasm): parseReconnectConfig accepts msgpack integer widths (#264)`.

### Task 2: #266 — `SendCommand` Add/Wait race

**Files:**
- Modify: `internal/wasm/module.go` (`Stop` ~L196-251, `SendCommand` ~L296-339)
- Test: `internal/wasm/wasm_test.go`

**Interfaces:**
- Produces: no signature changes. New behavior: `SendCommand` after `Stop` has begun is a logged no-op.

- [ ] **Step 1: Write the failing deterministic test** (append to `wasm_test.go`; white-box — asserts the buffered-branch drop):

```go
// SendCommand after Stop must be a no-op: the outstandingCalls.Add(1) in the
// command-export branch would otherwise race Stop's Wait-at-zero (the
// WaitGroup misuse Go disallows), and the buffered branch would grow a dead
// buffer (#266). The buffered branch is the deterministic observable.
func TestModule_SendCommand_AfterStopIsDropped(t *testing.T) {
	plugin := newMockPlugin() // no "command" export → buffered branch
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 20}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))
	m.Start()
	require.NoError(t, m.Stop(context.Background()))

	m.SendCommand(map[string]any{"action": "late"})

	m.mu.Lock()
	buffered := len(m.commands)
	m.mu.Unlock()
	assert.Zero(t, buffered, "command sent after Stop must be dropped, not buffered")
}

// Best-effort race probe: hammers SendCommand (command-export branch, the
// branch that calls outstandingCalls.Add) concurrently with Stop. Against
// unfixed code -race / the WaitGroup misuse panic trips probabilistically;
// with the mu-serialized stopping check it never can. The deterministic
// regression pin is TestModule_SendCommand_AfterStopIsDropped above.
func TestModule_SendCommand_ConcurrentWithStop(t *testing.T) {
	plugin := newMockPlugin()
	plugin.exports["command"] = true
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())

	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 60}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))
	m.Start()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				m.SendCommand(map[string]any{"n": j})
			}
		}()
	}
	require.NoError(t, m.Stop(context.Background()))
	wg.Wait()
}
```

- [ ] **Step 2:** Run `go test ./internal/wasm/ -run 'TestModule_SendCommand_AfterStop|TestModule_SendCommand_Concurrent' -race -v` — expect `AfterStopIsDropped` FAIL (buffered == 1). Concurrent probe may pass (probabilistic).

- [ ] **Step 3: Fix** — two edits in `module.go`:

(a) In `Stop`, move the stopping-flag store inside the locked section so it is mu-ordered before any later `SendCommand`:

```go
func (m *Module) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return nil
	}
	m.running = false
	// Set under mu so SendCommand's mu-held check is race-free: any
	// SendCommand that acquires the lock after this point observes
	// stopping=true and won't outstandingCalls.Add — every Add strictly
	// happens-before the Wait below. AddAsyncResult also keys off this
	// to drop late writes.
	m.stopping.Store(true)
	close(m.stopCh)
	m.mu.Unlock()
```

(delete the old `m.stopping.Store(true)` + its comment after the unlock)

(b) In `SendCommand`, guard under the lock (right after `defer m.mu.Unlock()`):

```go
	m.mu.Lock()
	defer m.mu.Unlock()

	// Once Stop has begun (stopping set under this same mutex), adding to
	// outstandingCalls would race Stop's Wait-at-zero — WaitGroup misuse.
	// Buffering would be equally pointless: no tick will drain it.
	if m.stopping.Load() {
		m.Logger.Warn("dropping command on stopped module", "module", m.Name)
		return
	}
```

(c) At the `outstandingCalls.Wait()` in `Stop`, extend the existing comment with the invariant note:

```go
	// Wait for outstanding async-call goroutines BEFORE clearing the maps
	// they write into. With the Add(1)/Done() wrapping in CallAsync this
	// actually waits for the right things; without it the wait was a no-op.
	// Invariant: every outstandingCalls.Add happens-before this Wait —
	// SendCommand checks stopping under mu, and the hostapi.go Add sites
	// only run inside guest exports, which are all serialized before this
	// point (tick loop exited via tickDone; shutdown call above returned).
	// A new Add site must preserve this or the Wait panics.
```

- [ ] **Step 4:** Re-run both tests with `-race` — PASS. Run existing `go test ./internal/wasm/ -race` full package (notably `TestModule_SendCommand_Buffered`, which sends **before Start** — must still pass since `stopping` is false pre-start).
- [ ] **Step 5:** Commit `fix(wasm): SendCommand cannot race Stop's outstandingCalls.Wait (#266)`.

### Task 3: #269 — example guest modules tinygo build

**Files:**
- Modify: `examples/wasm-counter/wasm/counter/main.go` (~L40)
- Modify: `examples/discord-bot/wasm/bot/main.go` (~L93)

**Interfaces:**
- Consumes: PDK `Command.Data any`, `IncomingWSMsg.Data any` (host delivers codec-decoded values, not bytes).

- [ ] **Step 1: Verify current breakage:** in each of the two module dirs run
  `tinygo build -o "$SCRATCHPAD/out.wasm" -target wasi -buildmode=c-shared .`
  Expected: `cannot use cmd.Data (variable of type any) as []byte value` (counter) / same for `msg.Data` (bot). Control: same command in `examples/wasm-helpers/wasm/helpers` (or its module dir) succeeds.

- [ ] **Step 2: Fix wasm-counter** — add helper and use it:

```go
// decodeInto converts a host-decoded value (the PDK delivers Data fields
// already unmarshalled, as any) into a typed struct via a JSON round-trip.
func decodeInto(v any, dst any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
```

and in `tick()` replace `if err := json.Unmarshal(cmd.Data, &op); err != nil {` with `if err := decodeInto(cmd.Data, &op); err != nil {`.

- [ ] **Step 3: Fix discord-bot** — same helper in `bot/main.go`; replace `if err := json.Unmarshal(msg.Data, &payload); err != nil {` with `if err := decodeInto(msg.Data, &payload); err != nil {`. The nested `json.RawMessage` fields (`gatewayPayload.Data`) work because decodeInto re-marshals to real JSON first. Check the rest of the file for further direct `.Data` byte uses (`handleHello`/`handleDispatch` take the already-raw `payload.Data json.RawMessage` — those stay `json.Unmarshal`).

- [ ] **Step 4:** Re-run the tinygo builds for both modules — both succeed.
- [ ] **Step 5:** Commit `fix(examples): wasm-counter & discord-bot guest modules compile under tinygo (#269)`.

### Task 4: #271 — drain `responseCh` on workflow-error and timeout paths

**Files:**
- Modify: `internal/server/routes.go:414-462` (extract handler tail)
- Test: `internal/server/routes_test.go`

**Interfaces:**
- Produces: `func (s *Server) awaitWorkflowResponse(c fiber.Ctx, responseCh chan *api.HTTPResponse, workflowDone chan error, responseTimeout time.Duration, routeID, traceID string, respValidator *responseValidator) error`

- [ ] **Step 1: Write the failing tests** (append to `routes_test.go`):

```go
// A response the workflow already produced must win deterministically over a
// synthesized workflow error / 504: Go's select picks randomly among ready
// cases, so without the drain an identical run could 500/504 or succeed
// depending on scheduling (#271, reachable since tranche B made truncation
// fail loudly).
func TestAwaitWorkflowResponse_ErrorAfterResponse_ResponseWins(t *testing.T) {
	srv := newTestServer(t, map[string]map[string]any{}, map[string]map[string]any{}, nil)

	responseCh := make(chan *api.HTTPResponse, 1)
	responseCh <- &api.HTTPResponse{Status: 201, Body: map[string]any{"ok": true}}
	workflowDone := make(chan error, 1)
	workflowDone <- fmt.Errorf("workflow timed out after response fired")

	srv.App().Get("/await-error", func(c fiber.Ctx) error {
		return srv.awaitWorkflowResponse(c, responseCh, workflowDone, time.Second, "r", "tid", nil)
	})
	resp, err := srv.App().Test(httptest.NewRequest("GET", "/await-error", nil))
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
}

func TestAwaitWorkflowResponse_TimeoutAfterResponse_ResponseWins(t *testing.T) {
	srv := newTestServer(t, map[string]map[string]any{}, map[string]map[string]any{}, nil)

	responseCh := make(chan *api.HTTPResponse, 1)
	responseCh <- &api.HTTPResponse{Status: 200, Body: map[string]any{"ok": true}}
	workflowDone := make(chan error, 1) // never closes: workflow still running

	srv.App().Get("/await-timeout", func(c fiber.Ctx) error {
		return srv.awaitWorkflowResponse(c, responseCh, workflowDone, time.Nanosecond, "r", "tid", nil)
	})
	resp, err := srv.App().Test(httptest.NewRequest("GET", "/await-timeout", nil))
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

// Polarity controls: with no produced response the existing behavior stands.
func TestAwaitWorkflowResponse_ErrorWithoutResponse_MapsError(t *testing.T) {
	srv := newTestServer(t, map[string]map[string]any{}, map[string]map[string]any{}, nil)

	responseCh := make(chan *api.HTTPResponse, 1)
	workflowDone := make(chan error, 1)
	workflowDone <- fmt.Errorf("boom")

	srv.App().Get("/await-plain-error", func(c fiber.Ctx) error {
		return srv.awaitWorkflowResponse(c, responseCh, workflowDone, time.Second, "r", "tid", nil)
	})
	resp, err := srv.App().Test(httptest.NewRequest("GET", "/await-plain-error", nil))
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)
}

func TestAwaitWorkflowResponse_TimeoutWithoutResponse_504(t *testing.T) {
	srv := newTestServer(t, map[string]map[string]any{}, map[string]map[string]any{}, nil)

	responseCh := make(chan *api.HTTPResponse, 1)
	workflowDone := make(chan error, 1)

	srv.App().Get("/await-plain-timeout", func(c fiber.Ctx) error {
		return srv.awaitWorkflowResponse(c, responseCh, workflowDone, 10*time.Millisecond, "r", "tid", nil)
	})
	resp, err := srv.App().Test(httptest.NewRequest("GET", "/await-plain-timeout", nil))
	require.NoError(t, err)
	assert.Equal(t, 504, resp.StatusCode)
}
```

(Note: in the first two tests both select cases are ready — whichever the select picks, the outcome must be the response. That invariance IS the fix; against unfixed code the tests flake ~50%, run with `-count=20` in step 2 to confirm failure.)

- [ ] **Step 2:** `go test ./internal/server/ -run TestAwaitWorkflowResponse -count=20` — expect compile FAIL (method missing). After extraction-without-drain (if done mechanically first), the two `_ResponseWins` tests must FAIL within 20 runs.

- [ ] **Step 3: Implement** — in `buildRouteHandler`, replace lines 426-462 (from `timer := time.NewTimer(...)` through the select's closing brace) with:

```go
		return s.awaitWorkflowResponse(c, responseCh, workflowDone, responseTimeout, routeID, traceID, respValidator)
```

and add the method below the handler builder:

```go
// awaitWorkflowResponse waits for the first of: a produced response, workflow
// completion, or the response timeout — and writes the HTTP result. On the
// error and timeout arms it first drains responseCh: a response the workflow
// already produced wins deterministically over a synthesized error, because
// select chooses randomly among ready cases and the client would have
// received that response under a marginally different interleaving. The
// suppressed workflow error stays visible in logs and the trace.
func (s *Server) awaitWorkflowResponse(c fiber.Ctx, responseCh chan *api.HTTPResponse, workflowDone chan error, responseTimeout time.Duration, routeID, traceID string, respValidator *responseValidator) error {
	timer := time.NewTimer(responseTimeout)
	defer timer.Stop()

	select {
	case resp := <-responseCh:
		// Response node fired — send response immediately
		return s.validateAndWriteResponse(c, resp, routeID, traceID, respValidator)

	case wfErr := <-workflowDone:
		if wfErr != nil {
			select {
			case resp := <-responseCh:
				s.logger.Warn("workflow failed after producing a response; returning the response",
					"route", routeID, "error", wfErr, "trace_id", traceID)
				return s.validateAndWriteResponse(c, resp, routeID, traceID, respValidator)
			default:
			}
			status, errResp := MapErrorToHTTP(wfErr, traceID, s.devMode)
			return writeErrorResponse(c, status, errResp)
		}
		// Drain responseCh: the response node may have fired just before
		// the workflow finished, and select picked this case randomly.
		select {
		case resp := <-responseCh:
			return s.validateAndWriteResponse(c, resp, routeID, traceID, respValidator)
		default:
		}
		// No response node → 202 Accepted
		return c.Status(fiber.StatusAccepted).JSON(map[string]any{
			"status":   "accepted",
			"trace_id": traceID,
		})

	case <-timer.C:
		select {
		case resp := <-responseCh:
			return s.validateAndWriteResponse(c, resp, routeID, traceID, respValidator)
		default:
		}
		// Response timeout — the deferred cancel stops the workflow goroutine
		return writeErrorResponse(c, 504, ErrorResponse{
			Error: api.ErrorData{
				Code:    "TIMEOUT",
				Message: "Response timeout exceeded",
				TraceID: traceID,
			},
		})
	}
}
```

- [ ] **Step 4:** `go test ./internal/server/ -run TestAwaitWorkflowResponse -count=20 -race` PASS; then full `go test ./internal/server/` (existing route tests, esp. `TestRoute_WorkflowError_MappedResponse` and `TestRoute_NoResponseNode_202Accepted`, must stay green).
- [ ] **Step 5:** Commit `fix(server): produced response wins over workflow error/timeout on the route handler select (#271)`.

### Task 5: #280 — redactor fails closed past depth cap

**Files:**
- Modify: `internal/trace/redact.go:54-57`
- Test: `internal/trace/redact_test.go`

- [ ] **Step 1: Write the failing test:**

```go
// Past the recursion cap the redactor must fail CLOSED: returning the raw
// value would leak a deeply nested secret — over-redaction is the safe
// direction for degenerate (>32-deep) payloads (#280).
func TestRedactValue_PastDepthCapScrubbed(t *testing.T) {
	leaf := "raw-secret-material"
	v := any(map[string]any{"leaf": leaf})
	for i := 0; i < maxRedactDepth+2; i++ {
		v = map[string]any{"nest": v}
	}
	out := redactValue(v)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), leaf) {
		t.Fatalf("leaf value survived past the depth cap: %s", b)
	}
	if !strings.Contains(string(b), "[REDACTED: max depth]") {
		t.Fatalf("expected max-depth sentinel in output: %s", b)
	}
}

func TestRedactValue_UnderDepthCapPassesThrough(t *testing.T) {
	out := redactValue(map[string]any{"a": map[string]any{"b": "plain"}})
	b, _ := json.Marshal(out)
	if !strings.Contains(string(b), "plain") {
		t.Fatalf("shallow non-sensitive value must pass through: %s", b)
	}
}
```

- [ ] **Step 2:** `go test ./internal/trace/ -run TestRedactValue_ -v` — `PastDepthCapScrubbed` FAIL (raw leaf present).
- [ ] **Step 3: Fix** in `redactValueDepth`:

```go
func redactValueDepth(v any, depth int) any {
	if depth > maxRedactDepth {
		// Fail closed: past the cap we can no longer classify keys, and
		// returning the raw value would leak anything sensitive below it.
		return "[REDACTED: max depth]"
	}
```

- [ ] **Step 4:** Re-run + full `go test ./internal/trace/`.
- [ ] **Step 5:** Commit `fix(trace): redactor fails closed past the depth cap (#280)`.

### Task 6: #281 — case-insensitive origin compare

**Files:**
- Modify: `internal/trace/websocket.go:79-89`
- Test: `internal/trace/websocket_test.go`

- [ ] **Step 1: Write the failing test:**

```go
// Hostnames are case-insensitive (RFC 4343); originAllowed must not reject
// same-origin dev traffic over casing (#281). It still fails closed for
// genuinely foreign origins.
func TestOriginAllowed_CaseInsensitive(t *testing.T) {
	cases := []struct {
		origin, host string
		want         bool
	}{
		{"http://Example.com", "example.com", true},
		{"http://EXAMPLE.COM:3000", "example.com", true},
		{"http://LocalHost:5173", "example.com", true},
		{"http://evil.com", "example.com", false},
		{"://bad-url", "example.com", false},
	}
	for _, tc := range cases {
		if got := originAllowed(tc.origin, tc.host); got != tc.want {
			t.Errorf("originAllowed(%q, %q) = %v, want %v", tc.origin, tc.host, got, tc.want)
		}
	}
}
```

- [ ] **Step 2:** `go test ./internal/trace/ -run TestOriginAllowed -v` — FAIL on the two mixed-case rows.
- [ ] **Step 3: Fix:**

```go
func originAllowed(origin, host string) bool {
	u, err := neturl.Parse(origin)
	if err != nil {
		return false
	}
	oh := u.Hostname()
	// Hostnames are case-insensitive (RFC 4343).
	if strings.EqualFold(oh, host) {
		return true
	}
	return strings.EqualFold(oh, "localhost") || oh == "127.0.0.1" || oh == "::1"
}
```

(add `"strings"` to imports)

- [ ] **Step 4:** Re-run + full `go test ./internal/trace/` (incl. `TestTraceWebSocket_RejectsCrossOrigin`).
- [ ] **Step 5:** Commit `fix(trace): dev /ws/trace origin compare is case-insensitive (#281)`.

### Task 7: CHANGELOG + gates + PR

**Files:**
- Modify: `CHANGELOG.md` (`[Unreleased]`)

- [ ] **Step 1: CHANGELOG** — under `### Fixed` add:

```markdown
- Quick-wins batch: wasm gateway reconnection settings are honored under msgpack encoding (`max_attempts`/`initial_delay` no longer silently coerce to zero); `wasm.send` during module shutdown can no longer trip the Go WaitGroup Add/Wait misuse panic (commands to a stopping module are dropped with a warning); a response the workflow already produced now deterministically wins over a synthesized workflow error or response timeout (previously a scheduling race could return 500/504 despite a produced response); the wasm-counter and discord-bot example guest modules compile under tinygo again; the dev `/ws/trace` origin check compares hostnames case-insensitively.
```

under `### Security`:

```markdown
- The trace redactor now fails closed past its recursion depth cap (returns `[REDACTED: max depth]` instead of the raw value), so a secret nested deeper than 32 levels can no longer bypass redaction.
```

- [ ] **Step 2: Gates** (from the worktree root):
  - `gofmt -l cmd internal plugins pkg examples` → no output
  - `go vet ./...`
  - `golangci-lint run`
  - `go test -race ./internal/wasm/... ./internal/trace/... ./internal/server/...`
  - `go build ./...`
  - tinygo builds (Task 3 commands) for both fixed examples + wasm-helpers control
- [ ] **Step 3:** `git add -f docs/superpowers/specs/2026-07-07-quick-wins-design.md docs/superpowers/plans/2026-07-07-quick-wins.md` + commit CHANGELOG + docs.
- [ ] **Step 4:** Whole-branch review via the code-review skill; fix Critical/Important findings, file follow-up issues for Minors.
- [ ] **Step 5:** Push, open PR titled `fix: quick-wins batch — reconnect msgpack, SendCommand race, example builds, response drain, redact depth, origin case`, body summarizing each fix with `Fixes #264`, `Fixes #266`, `Fixes #269`, `Fixes #271`, `Fixes #280`, `Fixes #281`. Wait for the 4 required CI checks.
