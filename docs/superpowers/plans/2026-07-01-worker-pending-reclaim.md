# Worker Pending-Reclaim & Poison-Message Disposition Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an XAutoClaim-based pending-reclaim path to the worker runtime so failed/stranded messages are actually redelivered, and route pre-handler panics through the same retry → dead-letter/drop disposition instead of leaking them into the pending-entries list (PEL).

**Architecture:** One dedicated reaper goroutine per worker periodically runs `XAUTOCLAIM` for entries idle longer than `retry.min_idle` and dispatches them back through `processMessage`. `processMessage` is restructured so all processing (deserialize → input-map → build → handler) runs inside a single `recover()` that converts a panic into an error, and a single disposition step then decides ack / dead-letter / drop / leave-pending based on delivery attempts and config.

**Tech Stack:** Go, `github.com/redis/go-redis/v9` (`XAutoClaim`, `XPendingExt`, `XAck`, `XAdd`), `github.com/alicebob/miniredis/v2` for tests (supports `XAUTOCLAIM` + `FastForward` to simulate idle time), `log/slog`, `runtime/debug`.

## Global Constraints

- Language: Go; module `github.com/chimpanze/noda`.
- All new logic lives in `internal/worker/` (single package `worker`).
- Tests use `miniredis` via the existing `newTestSetup(t)` helper in `internal/worker/runtime_test.go` — no testcontainers, no build tags.
- No backward-compatibility constraints (pre-release project); config semantics may change freely.
- Follow existing code style: `slog` structured logging with `"worker_id"`, `"message_id"`, `"trace_id"` fields; errors via `fmt.Errorf`.
- `retry.min_idle` MUST be `>=` the effective handler timeout, or the reaper can steal a message a live consumer is still processing (double execution). Default effective timeout is `defaultMessageTimeout = 5m`.
- Defaults: `retry.max_attempts` = `10`; `retry.min_idle` = effective timeout, floored to `60s`.

---

### Task 1: Add `RetryConfig`, parse `retry` block, resolve/clamp defaults, update schema

**Files:**
- Modify: `internal/worker/runtime.go` (struct defs near `WorkerConfig`/`DeadLetterConfig` ~lines 24-41; `ParseWorkerConfigs` ~lines 445-510; add constants near `defaultMessageTimeout` ~line 219)
- Modify: `internal/config/schemas/worker.json`
- Test: `internal/worker/runtime_test.go`

**Interfaces:**
- Produces:
  - `type RetryConfig struct { MinIdle time.Duration; MaxAttempts int }`
  - `WorkerConfig.Retry RetryConfig` field
  - `const defaultMaxAttempts = 10`, `const minIdleFloor = 60 * time.Second`
  - `func resolveRetry(rc RetryConfig, timeout time.Duration, logger *slog.Logger, workerID string) RetryConfig`

- [ ] **Step 1: Write the failing test for parsing + defaults + clamp**

Add to `internal/worker/runtime_test.go`:

```go
func TestParseWorkerConfigs_RetryParsing(t *testing.T) {
	raw := map[string]map[string]any{
		"workers/w.json": {
			"id":        "w",
			"services":  map[string]any{"stream": "main-stream"},
			"subscribe": map[string]any{"topic": "t", "group": "g"},
			"trigger":   map[string]any{"workflow": "wf"},
			"retry":     map[string]any{"min_idle": "90s", "max_attempts": float64(7)},
		},
	}
	configs := ParseWorkerConfigs(raw)
	require.Len(t, configs, 1)
	assert.Equal(t, 90*time.Second, configs[0].Retry.MinIdle)
	assert.Equal(t, 7, configs[0].Retry.MaxAttempts)
}

func TestResolveRetry_Defaults(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// No timeout configured -> effective timeout is defaultMessageTimeout (5m).
	got := resolveRetry(RetryConfig{}, 0, logger, "w")
	assert.Equal(t, defaultMessageTimeout, got.MinIdle) // 5m > 60s floor
	assert.Equal(t, defaultMaxAttempts, got.MaxAttempts)
}

func TestResolveRetry_ClampsMinIdleUpToTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// min_idle 10s is below a 30s timeout -> clamp up to 60s floor (>= timeout).
	got := resolveRetry(RetryConfig{MinIdle: 10 * time.Second, MaxAttempts: 3}, 30*time.Second, logger, "w")
	assert.Equal(t, 60*time.Second, got.MinIdle) // clamped to timeout(30s) then floored to 60s
	assert.Equal(t, 3, got.MaxAttempts)
}
```

Add imports `io` and `log/slog` to the test file if not already present (they are not — add them).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/ -run 'TestParseWorkerConfigs_RetryParsing|TestResolveRetry' -v`
Expected: FAIL — `configs[0].Retry` undefined / `resolveRetry` undefined (compile error).

- [ ] **Step 3: Add the struct, field, constants, and resolver**

In `internal/worker/runtime.go`, add to imports (if missing): `"runtime/debug"` is NOT needed here (Task 3), but this task needs nothing new beyond existing imports.

Add the `Retry` field to `WorkerConfig` (after `DeadLetter *DeadLetterConfig`):

```go
	DeadLetter  *DeadLetterConfig
	Retry       RetryConfig
```

Add the struct after `DeadLetterConfig`:

```go
// RetryConfig controls pending-message reclaim and the poison-message cap.
type RetryConfig struct {
	MinIdle     time.Duration // pending entry must be idle this long before reclaim
	MaxAttempts int           // hard cap on delivery attempts when no dead_letter diverts
}
```

Add constants near `defaultMessageTimeout`:

```go
// defaultMaxAttempts bounds delivery attempts when no dead_letter is configured.
const defaultMaxAttempts = 10

// minIdleFloor is the lowest permitted reclaim idle threshold.
const minIdleFloor = 60 * time.Second
```

Add the resolver (place it near `ParseWorkerConfigs`):

```go
// resolveRetry fills in retry defaults and enforces min_idle >= handler timeout
// (with a 60s floor) so the reaper never steals a message a live consumer is
// still processing.
func resolveRetry(rc RetryConfig, timeout time.Duration, logger *slog.Logger, workerID string) RetryConfig {
	if timeout <= 0 {
		timeout = defaultMessageTimeout
	}
	if rc.MaxAttempts <= 0 {
		rc.MaxAttempts = defaultMaxAttempts
	}
	if rc.MinIdle <= 0 {
		rc.MinIdle = timeout
	} else if rc.MinIdle < timeout {
		logger.Warn("worker retry.min_idle below handler timeout; clamping up to timeout",
			"worker_id", workerID,
			"min_idle", rc.MinIdle.String(),
			"timeout", timeout.String(),
		)
		rc.MinIdle = timeout
	}
	if rc.MinIdle < minIdleFloor {
		rc.MinIdle = minIdleFloor
	}
	return rc
}
```

In `ParseWorkerConfigs`, after the `dead_letter` block and before `configs = append(configs, wc)`, add:

```go
		if retry, ok := raw["retry"].(map[string]any); ok {
			if s, ok := retry["min_idle"].(string); ok {
				if d, err := time.ParseDuration(s); err == nil {
					wc.Retry.MinIdle = d
				}
			}
			if m, ok := retry["max_attempts"].(float64); ok {
				wc.Retry.MaxAttempts = int(m)
			}
			if m, ok := retry["max_attempts"].(int); ok {
				wc.Retry.MaxAttempts = m
			}
		}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/worker/ -run 'TestParseWorkerConfigs_RetryParsing|TestResolveRetry' -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Update the JSON schema**

In `internal/config/schemas/worker.json`, add a `retry` property inside `"properties"` (after `dead_letter`):

```json
    "retry": {
      "type": "object",
      "properties": {
        "min_idle": { "type": "string" },
        "max_attempts": { "type": "integer" }
      }
    }
```

- [ ] **Step 6: Run the full worker suite to confirm nothing regressed**

Run: `go test ./internal/worker/ ./internal/config/...`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/worker/runtime.go internal/worker/runtime_test.go internal/config/schemas/worker.json
git commit -m "feat(worker): add retry config (min_idle, max_attempts) with timeout-aware clamp"
```

---

### Task 2: Pure failure-disposition decision function

**Files:**
- Modify: `internal/worker/runtime.go`
- Test: `internal/worker/runtime_test.go`

**Interfaces:**
- Produces:
  - `type failureAction int` with `const ( actionPending failureAction = iota; actionDeadLetter; actionDrop )`
  - `func decideFailureDisposition(attempts int64, dl *DeadLetterConfig, maxAttempts int) failureAction`
- Semantics: when `dl` is configured (`dl != nil && dl.After > 0`), it is the sole bound — dead-letter at `attempts >= dl.After`, otherwise leave pending; the hard `maxAttempts` cap applies ONLY when no dead-letter is configured.

- [ ] **Step 1: Write the failing table-driven test**

Add to `internal/worker/runtime_test.go`:

```go
func TestDecideFailureDisposition(t *testing.T) {
	dl := &DeadLetterConfig{Topic: "dlq", After: 3}
	tests := []struct {
		name        string
		attempts    int64
		dl          *DeadLetterConfig
		maxAttempts int
		want        failureAction
	}{
		{"no dl, under cap -> pending", 1, nil, 10, actionPending},
		{"no dl, at cap -> drop", 10, nil, 10, actionDrop},
		{"no dl, over cap -> drop", 12, nil, 10, actionDrop},
		{"dl set, under after -> pending", 2, dl, 10, actionPending},
		{"dl set, at after -> dead-letter", 3, dl, 10, actionDeadLetter},
		{"dl set never hard-drops before after", 9, dl, 5, actionPending},
		{"dl with After<=0 treated as no dl", 10, &DeadLetterConfig{Topic: "x"}, 10, actionDrop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, decideFailureDisposition(tt.attempts, tt.dl, tt.maxAttempts))
		})
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/ -run TestDecideFailureDisposition -v`
Expected: FAIL — `failureAction` / `decideFailureDisposition` undefined.

- [ ] **Step 3: Implement the decision function**

In `internal/worker/runtime.go`, add near the other disposition helpers:

```go
// failureAction is the disposition for a failed (errored or panicked) message.
type failureAction int

const (
	actionPending    failureAction = iota // leave pending; reaper retries after min_idle
	actionDeadLetter                      // divert to the dead-letter topic and ack
	actionDrop                            // ack-drop and log an error (no DLQ configured)
)

// decideFailureDisposition chooses what to do with a failed message given how
// many times it has been delivered. When a dead-letter topic is configured it
// is the sole bound; the max-attempts cap only applies without one.
func decideFailureDisposition(attempts int64, dl *DeadLetterConfig, maxAttempts int) failureAction {
	if dl != nil && dl.After > 0 {
		if attempts >= int64(dl.After) {
			return actionDeadLetter
		}
		return actionPending
	}
	if attempts >= int64(maxAttempts) {
		return actionDrop
	}
	return actionPending
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/worker/ -run TestDecideFailureDisposition -v`
Expected: PASS (7 sub-tests).

- [ ] **Step 5: Commit**

```bash
git add internal/worker/runtime.go internal/worker/runtime_test.go
git commit -m "feat(worker): add failure-disposition decision (dead-letter vs drop vs pending)"
```

---

### Task 3: Restructure `processMessage` — single recover + unified disposition

**Files:**
- Modify: `internal/worker/runtime.go` (`processMessage` ~lines 223-355; add imports)
- Test: `internal/worker/runtime_test.go`

**Interfaces:**
- Produces:
  - `type msgResult struct { badInput bool; err error }`
  - `func (r *Runtime) runMessage(ctx context.Context, w WorkerConfig, msg redis.XMessage, traceID string) msgResult`
  - `func (r *Runtime) disposeFailure(ctx context.Context, client *redis.Client, w WorkerConfig, msg redis.XMessage, traceID string, wfErr error)`
- Consumes: `decideFailureDisposition`, `moveToDeadLetter`, `getDeliveryAttempts`, `deserializePayload` (all existing/Task 2).
- Behavior: a panic anywhere in deserialize/input-map/build/handler is recovered into `msgResult.err` (with stack) and treated as a retryable failure; an input-mapping *error* is a deterministic `badInput` (ack, or DLQ-copy if configured).

- [ ] **Step 1: Write the failing test — panic is recovered and message left pending**

Add to `internal/worker/runtime_test.go`. This uses a middleware that always panics; assert `processMessage` does not crash and the message stays pending (not acked) at attempt 1 with default retry.

```go
// panicMiddleware wraps handlers so invocation always panics.
type panicMiddleware struct{}

func (panicMiddleware) Name() string { return "test.panic" }
func (panicMiddleware) Wrap(next Handler, _ *MessageContext) Handler {
	return func(ctx context.Context) error { panic("boom in handler") }
}

func TestProcessMessage_PanicLeavesPending(t *testing.T) {
	client, svcReg, nodeReg, _ := newTestSetup(t)
	topic, group := "t-panic", "g-panic"
	require.NoError(t, client.XGroupCreateMkStream(context.Background(), topic, group, "0").Err())
	_, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: topic, Values: map[string]any{"payload": `{"x":1}`},
	}).Result()
	require.NoError(t, err)

	w := WorkerConfig{
		ID: "w", Topic: topic, Group: group, WorkflowID: "wf",
		Retry: RetryConfig{MinIdle: 60 * time.Second, MaxAttempts: 10},
	}
	r := NewRuntime([]WorkerConfig{w}, svcReg, nodeReg, map[string]map[string]any{
		"wf": {"nodes": map[string]any{}},
	}, nil, nil, nil, nil, nil, nil)
	r.middleware = []Middleware{panicMiddleware{}}
	parent := context.Background()
	r.opCtx.Store(&parent)

	// Read the message so it becomes pending, then process it.
	streams, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c", Streams: []string{topic, ">"}, Count: 1,
	}).Result()
	require.NoError(t, err)
	msg := streams[0].Messages[0]

	require.NotPanics(t, func() {
		r.processMessage(context.Background(), w, client, "c", msg)
	})

	// Not acked -> still pending (attempt 1 < maxAttempts, no dead_letter).
	pending, err := client.XPending(context.Background(), topic, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), pending.Count)
}
```

Note: `NewRuntime` takes 10 args in this order — `workers, services, nodes, workflows, workflowCache, middleware []Middleware, compiler, tracer, logger, secretsContext`. The test blocks pass `nil` for positions 5-10 and then set `r.middleware` directly (valid — the test is in `package worker`). `Handler` is `type Handler func(ctx context.Context) error` and `Middleware` is the interface `{ Name() string; Wrap(Handler, *MessageContext) Handler }` (confirm field names on `MessageContext` in `internal/worker/middleware.go`).

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/ -run TestProcessMessage_PanicLeavesPending -v`
Expected: FAIL — currently the panic is caught by the top-level bare recover, which just logs and returns WITHOUT the pending assertion holding? It DOES leave pending today, so this may pass against current code. If it passes now, that is fine — it becomes a regression guard. The behavioral change under test (stack capture + unified disposition) is covered by Task 5's end-to-end tests. Proceed regardless of red/green here; the goal of Step 3 is the refactor.

- [ ] **Step 3: Add imports and rewrite `processMessage`**

Add `"runtime/debug"` to the import block in `internal/worker/runtime.go`.

Replace the body of `processMessage` (everything from the `// Snapshot the operation ctx` comment through the final success `XAck`/log) with:

```go
	// Snapshot the operation ctx (survives r.cancel within the shutdown deadline).
	opCtxPtr := r.opCtx.Load()
	procCtx := *opCtxPtr

	timeout := w.Timeout
	if timeout == 0 {
		timeout = defaultMessageTimeout
	}
	procCtx, cancel := context.WithTimeout(procCtx, timeout)
	defer cancel()

	traceID := uuid.New().String()
	start := time.Now()

	r.logger.Info("worker processing message",
		"worker_id", w.ID, "consumer", consumerID, "message_id", msg.ID, "trace_id", traceID,
	)

	res := r.runMessage(procCtx, w, msg, traceID)
	duration := time.Since(start)

	if res.err == nil {
		if err := client.XAck(procCtx, w.Topic, w.Group, msg.ID).Err(); err != nil {
			r.logger.Error("worker ack failed",
				"worker_id", w.ID, "message_id", msg.ID, "trace_id", traceID, "error", err.Error())
		}
		r.logger.Info("worker message processed",
			"worker_id", w.ID, "message_id", msg.ID, "trace_id", traceID, "duration", duration.String())
		return
	}

	if res.badInput {
		r.logger.Error("worker input mapping failed",
			"worker_id", w.ID, "message_id", msg.ID, "trace_id", traceID, "error", res.err.Error())
		// Deterministic bad input: retrying can't help. Preserve for forensics
		// via the dead-letter topic if configured, otherwise ack-drop.
		if w.DeadLetter != nil {
			r.moveToDeadLetter(procCtx, client, w, msg, traceID, res.err)
		} else if err := client.XAck(procCtx, w.Topic, w.Group, msg.ID).Err(); err != nil {
			r.logger.Error("worker ack failed after bad mapping",
				"worker_id", w.ID, "message_id", msg.ID, "trace_id", traceID, "error", err.Error())
		}
		return
	}

	r.logger.Error("worker workflow failed",
		"worker_id", w.ID, "message_id", msg.ID, "trace_id", traceID,
		"duration", duration.String(), "error", res.err.Error())
	r.disposeFailure(procCtx, client, w, msg, traceID, res.err)
```

- [ ] **Step 4: Add `runMessage` and `disposeFailure`**

Add these methods below `processMessage`:

```go
// msgResult is the outcome of processing one message.
type msgResult struct {
	badInput bool  // deterministic input-mapping failure
	err      error // nil on success
}

// runMessage deserializes, maps input, builds the execution context, and runs
// the workflow through the middleware chain. Any panic in that span is recovered
// into res.err (with a stack) so it flows through the failure disposition rather
// than killing the consumer/reaper goroutine.
func (r *Runtime) runMessage(ctx context.Context, w WorkerConfig, msg redis.XMessage, traceID string) (res msgResult) {
	defer func() {
		if rec := recover(); rec != nil {
			res = msgResult{err: fmt.Errorf("worker.recover: panic in message processing: %v\n%s", rec, debug.Stack())}
		}
	}()

	payload := deserializePayload(msg.Values)
	messageCtx := map[string]any{
		"message": map[string]any{"id": msg.ID, "payload": payload},
	}

	input, err := engine.ResolveInput(r.compiler, w.InputMap, messageCtx)
	if err != nil {
		return msgResult{badInput: true, err: err}
	}

	opts := []engine.ExecutionContextOption{
		engine.WithInput(input),
		engine.WithTrigger(api.TriggerData{Type: "event", Timestamp: time.Now(), TraceID: traceID}),
		engine.WithWorkflowID(w.WorkflowID),
		engine.WithLogger(r.logger),
		engine.WithCompiler(r.compiler),
		engine.WithSecrets(r.secretsContext),
	}
	if r.tracer != nil {
		opts = append(opts, engine.WithTracer(r.tracer))
	}
	execCtx := engine.NewExecutionContext(opts...)

	handler := func(ctx context.Context) error {
		return engine.RunWorkflow(ctx, w.WorkflowID, execCtx, r.workflowCache, r.workflows, r.services, r.nodes)
	}
	for i := len(r.middleware) - 1; i >= 0; i-- {
		handler = r.middleware[i].Wrap(handler, &MessageContext{
			WorkerID: w.ID, MessageID: msg.ID, TraceID: traceID,
			Topic: w.Topic, Group: w.Group, Logger: r.logger,
		})
	}
	return msgResult{err: handler(ctx)}
}

// disposeFailure applies the retry/dead-letter/drop decision to a failed message.
func (r *Runtime) disposeFailure(ctx context.Context, client *redis.Client, w WorkerConfig, msg redis.XMessage, traceID string, wfErr error) {
	attempts := r.getDeliveryAttempts(ctx, client, w.Topic, w.Group, msg.ID)
	switch decideFailureDisposition(attempts, w.DeadLetter, w.Retry.MaxAttempts) {
	case actionDeadLetter:
		r.moveToDeadLetter(ctx, client, w, msg, traceID, wfErr)
	case actionDrop:
		r.logger.Error("worker dropping message after max attempts; configure dead_letter to retain poison messages",
			"worker_id", w.ID, "message_id", msg.ID, "trace_id", traceID,
			"attempts", attempts, "max_attempts", w.Retry.MaxAttempts, "error", wfErr.Error())
		if err := client.XAck(ctx, w.Topic, w.Group, msg.ID).Err(); err != nil {
			r.logger.Error("worker ack failed after drop",
				"worker_id", w.ID, "message_id", msg.ID, "trace_id", traceID, "error", err.Error())
		}
	default: // actionPending — leave pending; reaper reclaims after min_idle
	}
}
```

Delete the now-obsolete inline body that the old `processMessage` contained (old input-mapping ack block, old execCtx/handler build, old `wfErr` dead-letter block, old bare top-level `recover()` deferred func). All of that is now inside `runMessage`/`disposeFailure`.

- [ ] **Step 5: Run the worker suite**

Run: `go test ./internal/worker/ -run 'TestProcessMessage|TestRuntime|TestDecide|TestParseWorkerConfigs|TestResolveRetry' -v`
Expected: PASS. Fix any signature mismatches (e.g., `Handler` type name — confirm it against `internal/worker/middleware.go`).

- [ ] **Step 6: Run the full package with the race detector**

Run: `go test -race ./internal/worker/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/worker/runtime.go internal/worker/runtime_test.go
git commit -m "feat(worker): unify panic + error disposition, capture panic stack"
```

---

### Task 4: Reaper goroutine — `reapOnce`, `reap`, and `Start` wiring

**Files:**
- Modify: `internal/worker/runtime.go` (`Start` ~lines 98-150; add `reap`/`reapOnce`)
- Test: `internal/worker/runtime_test.go`

**Interfaces:**
- Produces:
  - `func (r *Runtime) reapOnce(ctx context.Context, w WorkerConfig, client *redis.Client) error` — one full cursor-paged `XAUTOCLAIM` pass; dispatches each reclaimed message through `processMessage` with consumer id `w.ID + "-reaper"`.
  - `func (r *Runtime) reap(ctx context.Context, w WorkerConfig, client *redis.Client)` — ticker loop calling `reapOnce`; `defer r.wg.Done()`.
  - `func reapInterval(minIdle time.Duration) time.Duration`
- Consumes: `processMessage`, `resolveRetry` (Task 1/3).

- [ ] **Step 1: Write the failing test — an idle pending message is reclaimed and reprocessed**

Add to `internal/worker/runtime_test.go`. Uses a middleware whose first invocation fails and second succeeds, so the message is left pending, then reclaimed after `FastForward`, then acked.

```go
// flakyMiddleware fails the first N handler invocations, then succeeds.
type flakyMiddleware struct {
	mu       sync.Mutex
	failFor  int
	calls    int
}

func (m *flakyMiddleware) Name() string { return "test.flaky" }
func (m *flakyMiddleware) Wrap(next Handler, _ *MessageContext) Handler {
	return func(ctx context.Context) error {
		m.mu.Lock()
		m.calls++
		fail := m.calls <= m.failFor
		m.mu.Unlock()
		if fail {
			return fmt.Errorf("transient failure %d", m.calls)
		}
		return next(ctx)
	}
}

func TestReapOnce_ReclaimsIdlePendingMessage(t *testing.T) {
	client, svcReg, nodeReg, mr := newTestSetup(t)
	topic, group := "t-reap", "g-reap"
	require.NoError(t, client.XGroupCreateMkStream(context.Background(), topic, group, "0").Err())
	_, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: topic, Values: map[string]any{"payload": `{"x":1}`},
	}).Result()
	require.NoError(t, err)

	w := WorkerConfig{
		ID: "w", Topic: topic, Group: group, WorkflowID: "wf",
		Retry: RetryConfig{MinIdle: 60 * time.Second, MaxAttempts: 10},
	}
	r := NewRuntime([]WorkerConfig{w}, svcReg, nodeReg, map[string]map[string]any{
		"wf": {"nodes": map[string]any{}},
	}, nil, nil, nil, nil, nil, nil)
	flaky := &flakyMiddleware{failFor: 1}
	r.middleware = []Middleware{flaky}
	parent := context.Background()
	r.opCtx.Store(&parent)

	// Deliver + fail once -> message left pending.
	streams, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c", Streams: []string{topic, ">"}, Count: 1,
	}).Result()
	require.NoError(t, err)
	r.processMessage(context.Background(), w, client, "c", streams[0].Messages[0])

	pending, _ := client.XPending(context.Background(), topic, group).Result()
	require.Equal(t, int64(1), pending.Count)

	// Before min_idle elapses, reapOnce reclaims nothing.
	require.NoError(t, r.reapOnce(context.Background(), w, client))
	pending, _ = client.XPending(context.Background(), topic, group).Result()
	require.Equal(t, int64(1), pending.Count)

	// Advance past min_idle; now reapOnce reclaims and reprocesses -> succeeds -> acked.
	mr.FastForward(61 * time.Second)
	require.NoError(t, r.reapOnce(context.Background(), w, client))
	pending, _ = client.XPending(context.Background(), topic, group).Result()
	assert.Equal(t, int64(0), pending.Count)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/worker/ -run TestReapOnce_ReclaimsIdlePendingMessage -v`
Expected: FAIL — `r.reapOnce` undefined.

- [ ] **Step 3: Implement `reapInterval`, `reapOnce`, and `reap`**

In `internal/worker/runtime.go`, add:

```go
// reapInterval derives how often the reaper runs a claim pass from min_idle,
// bounded to a sane [5s, 30s] range so short idle windows still get serviced.
func reapInterval(minIdle time.Duration) time.Duration {
	iv := minIdle
	if iv > 30*time.Second {
		iv = 30 * time.Second
	}
	if iv < 5*time.Second {
		iv = 5 * time.Second
	}
	return iv
}

// reapOnce runs a single cursor-paged XAUTOCLAIM pass for entries idle longer
// than w.Retry.MinIdle, dispatching each reclaimed message through the normal
// processing/disposition path.
func (r *Runtime) reapOnce(ctx context.Context, w WorkerConfig, client *redis.Client) error {
	consumerID := w.ID + "-reaper"
	cursor := "0"
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msgs, next, err := client.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   w.Topic,
			Group:    w.Group,
			Consumer: consumerID,
			MinIdle:  w.Retry.MinIdle,
			Start:    cursor,
			Count:    16,
		}).Result()
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			r.processMessage(ctx, w, client, consumerID, msg)
		}
		if next == "0" || next == "0-0" {
			return nil
		}
		cursor = next
	}
}

// reap periodically reclaims idle pending messages for one worker.
func (r *Runtime) reap(ctx context.Context, w WorkerConfig, client *redis.Client) {
	defer r.wg.Done()
	ticker := time.NewTicker(reapInterval(w.Retry.MinIdle))
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.reapOnce(ctx, w, client); err != nil && ctx.Err() == nil {
				r.logger.Error("worker reaper claim failed",
					"worker_id", w.ID, "error", err.Error())
			}
		}
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/worker/ -run TestReapOnce_ReclaimsIdlePendingMessage -v`
Expected: PASS.

- [ ] **Step 5: Wire `resolveRetry` + reaper into `Start`**

In `internal/worker/runtime.go` `Start`, inside the `for _, w := range r.workers` loop, immediately after `concurrency` is validated and before spawning consumers, resolve retry defaults on the local `w`:

```go
		w.Retry = resolveRetry(w.Retry, w.Timeout, r.logger, w.ID)
```

Then, after the consumer-spawn `for i := 0; i < concurrency; i++` loop and before the `r.logger.Info("worker started", ...)` call, spawn the reaper:

```go
		r.wg.Add(1)
		go r.reap(ctx, w, client)
```

(Because `w` is a loop-local copy, the resolved `Retry` travels with the value passed to `consume`/`reap`. Note: Go 1.22+ per-iteration loop variables — confirm module Go version ≥ 1.22 in `go.mod`; the codebase targets Go 1.25, so this is safe.)

- [ ] **Step 6: Run the full worker suite with race detector**

Run: `go test -race ./internal/worker/`
Expected: PASS (reaper goroutine starts/stops cleanly under `Start`/`Stop`).

- [ ] **Step 7: Commit**

```bash
git add internal/worker/runtime.go internal/worker/runtime_test.go
git commit -m "feat(worker): add per-worker XAutoClaim reaper for pending-message redelivery"
```

---

### Task 5: End-to-end disposition tests + docs + CHANGELOG

**Files:**
- Test: `internal/worker/runtime_test.go`
- Modify: `docs/02-config/workers.md`
- Modify: `CHANGELOG.md`

**Interfaces:**
- Consumes: everything from Tasks 1-4. No new production code (if a test reveals a gap, fix it in the relevant file and note it).

- [ ] **Step 1: Write the poison-panic dead-letter test**

Add to `internal/worker/runtime_test.go`. A payload that always panics, with `dead_letter.after = 3`, should be dead-lettered after 3 delivery attempts.

```go
func TestReclaim_PoisonPanic_DeadLettered(t *testing.T) {
	client, svcReg, nodeReg, mr := newTestSetup(t)
	topic, group, dlq := "t-poison", "g-poison", "t-poison.dlq"
	require.NoError(t, client.XGroupCreateMkStream(context.Background(), topic, group, "0").Err())
	_, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: topic, Values: map[string]any{"payload": `{"x":1}`},
	}).Result()
	require.NoError(t, err)

	w := WorkerConfig{
		ID: "w", Topic: topic, Group: group, WorkflowID: "wf",
		Retry:      RetryConfig{MinIdle: 60 * time.Second, MaxAttempts: 10},
		DeadLetter: &DeadLetterConfig{Topic: dlq, After: 3},
	}
	r := NewRuntime([]WorkerConfig{w}, svcReg, nodeReg, map[string]map[string]any{
		"wf": {"nodes": map[string]any{}},
	}, nil, nil, nil, nil, nil, nil)
	r.middleware = []Middleware{panicMiddleware{}}
	parent := context.Background()
	r.opCtx.Store(&parent)

	// Attempt 1 (fresh delivery).
	streams, _ := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c", Streams: []string{topic, ">"}, Count: 1,
	}).Result()
	r.processMessage(context.Background(), w, client, "c", streams[0].Messages[0])

	// Attempts 2 and 3 via reclaim; each XAutoClaim bumps the delivery count.
	for i := 0; i < 3; i++ {
		mr.FastForward(61 * time.Second)
		require.NoError(t, r.reapOnce(context.Background(), w, client))
	}

	// Original acked (drained from PEL) and a message landed on the DLQ.
	pending, _ := client.XPending(context.Background(), topic, group).Result()
	assert.Equal(t, int64(0), pending.Count)
	dlLen, err := client.XLen(context.Background(), dlq).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), dlLen)
}
```

- [ ] **Step 2: Run it**

Run: `go test ./internal/worker/ -run TestReclaim_PoisonPanic_DeadLettered -v`
Expected: PASS. If attempts arithmetic is off by one (delivery count semantics), adjust the loop count so the total delivery count reaches `After=3`, and confirm against `getDeliveryAttempts`.

- [ ] **Step 3: Write the poison-panic no-DLQ drop test**

```go
func TestReclaim_PoisonPanic_NoDLQ_DroppedAfterMaxAttempts(t *testing.T) {
	client, svcReg, nodeReg, mr := newTestSetup(t)
	topic, group := "t-drop", "g-drop"
	require.NoError(t, client.XGroupCreateMkStream(context.Background(), topic, group, "0").Err())
	_, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: topic, Values: map[string]any{"payload": `{"x":1}`},
	}).Result()
	require.NoError(t, err)

	w := WorkerConfig{
		ID: "w", Topic: topic, Group: group, WorkflowID: "wf",
		Retry: RetryConfig{MinIdle: 60 * time.Second, MaxAttempts: 3}, // no DeadLetter
	}
	r := NewRuntime([]WorkerConfig{w}, svcReg, nodeReg, map[string]map[string]any{
		"wf": {"nodes": map[string]any{}},
	}, nil, nil, nil, nil, nil, nil)
	r.middleware = []Middleware{panicMiddleware{}}
	parent := context.Background()
	r.opCtx.Store(&parent)

	streams, _ := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c", Streams: []string{topic, ">"}, Count: 1,
	}).Result()
	r.processMessage(context.Background(), w, client, "c", streams[0].Messages[0])

	for i := 0; i < 3; i++ {
		mr.FastForward(61 * time.Second)
		require.NoError(t, r.reapOnce(context.Background(), w, client))
	}

	// Dropped (acked) after reaching max_attempts; PEL empty, nothing re-queued.
	pending, _ := client.XPending(context.Background(), topic, group).Result()
	assert.Equal(t, int64(0), pending.Count)
}
```

- [ ] **Step 4: Run it**

Run: `go test ./internal/worker/ -run TestReclaim_PoisonPanic_NoDLQ_DroppedAfterMaxAttempts -v`
Expected: PASS (adjust loop count if needed so delivery count reaches `MaxAttempts=3`).

- [ ] **Step 5: Run the whole worker package with the race detector**

Run: `go test -race ./internal/worker/`
Expected: PASS (all new + existing tests).

- [ ] **Step 6: Document the feature**

In `docs/02-config/workers.md`, add a section documenting:
- the `retry` block (`min_idle`, `max_attempts`) with an example;
- the constraint that `min_idle` is clamped up to at least the handler `timeout` (and a 60s floor) so reclaim never steals in-flight work;
- that `dead_letter.after` now counts delivery attempts and, when set, is the bound (messages are retried via reclaim until `after`, then dead-lettered);
- that with no `dead_letter`, a message is dropped with an ERROR log after `max_attempts` (default 10) — configure `dead_letter` to retain poison messages.

Match the existing prose style and heading depth of the file (read it first).

- [ ] **Step 7: Update CHANGELOG**

In `CHANGELOG.md`, under `### Fixed` (or `### Added`/`### Changed` as fits the existing structure), add entries:

```markdown
- Worker now reclaims idle pending messages via XAutoClaim, so failed messages are actually redelivered and dead-letter (`dead_letter.after`) and retry limits are enforced (previously pending messages were never re-processed).
- Worker pre-handler panics are now retried and dead-lettered/dropped through the normal disposition instead of being stranded in the pending-entries list (#243); panic errors now include a stack trace.
- New worker `retry` config (`min_idle`, `max_attempts`); without a `dead_letter` topic a repeatedly-failing message is dropped with a loud error after `max_attempts`.
```

- [ ] **Step 8: Full build, vet, and the broader suites**

Run: `go build ./... && go vet ./internal/worker/ && go test ./internal/worker/ ./internal/config/...`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/worker/runtime_test.go docs/02-config/workers.md CHANGELOG.md
git commit -m "test(worker): e2e reclaim/dead-letter/drop coverage; docs + changelog (#243)"
```

---

## Self-Review

**Spec coverage:**
- Config surface (`retry.min_idle`, `retry.max_attempts`, functional `dead_letter.after`) → Task 1 (+ schema) and Task 5 docs. ✔
- `min_idle >= timeout` clamp with WARN → Task 1 `resolveRetry` + test. ✔
- Reaper goroutine (one per worker, XAutoClaim, opCtx, wg, error resilience) → Task 4 (`reap`/`reapOnce`, `Start` wiring). ✔
- Unified disposition (success / bad-input ack+DLQ-copy / wfErr+panic → dead-letter/drop/pending) → Tasks 2-3. ✔
- Panic captured with `debug.Stack()` → Task 3 `runMessage`. ✔
- No-DLQ hard-cap drop + ERROR → Task 2 decision + Task 3 `disposeFailure` + Task 5 test. ✔
- Test matrix (reclaim reprocess, transient retry→success, poison+DLQ→dead-letter, poison no-DLQ→drop, no-steal-before-min_idle, disposition unit table, config parse/clamp) → Tasks 1,2,4,5. ✔
- Out of scope (idempotency, backoff) → not implemented, as intended. ✔

**Placeholder scan:** No TBD/TODO; every code step shows complete code. Two steps ask the implementer to confirm an existing signature (`NewRuntime`, `Handler`, delivery-count arithmetic) against the codebase and adjust — these are verification instructions, not placeholders.

**Type consistency:** `RetryConfig{MinIdle, MaxAttempts}`, `failureAction`/`actionPending|actionDeadLetter|actionDrop`, `msgResult{badInput, err}`, `runMessage`, `disposeFailure`, `reapOnce`, `reap`, `reapInterval`, `resolveRetry`, `decideFailureDisposition` are named identically across all tasks. Reaper consumer id `w.ID+"-reaper"` is consistent. `getDeliveryAttempts`/`moveToDeadLetter`/`deserializePayload` reused from existing code with their current signatures.
