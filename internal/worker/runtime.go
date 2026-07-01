package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// WorkerConfig holds the parsed configuration for a single worker.
type WorkerConfig struct {
	ID          string
	StreamSvc   string // service name for stream
	Topic       string
	Group       string
	Concurrency int
	Timeout     time.Duration // per-message processing timeout (default 5m)
	Middleware  []string
	WorkflowID  string
	InputMap    map[string]any
	DeadLetter  *DeadLetterConfig
	Retry       RetryConfig
}

// DeadLetterConfig holds dead letter queue configuration.
type DeadLetterConfig struct {
	Topic string
	After int
}

// RetryConfig controls pending-message reclaim and the poison-message cap.
type RetryConfig struct {
	MinIdle     time.Duration // pending entry must be idle this long before reclaim
	MaxAttempts int           // hard cap on delivery attempts when no dead_letter diverts
}

// Runtime manages worker consumers that process messages from Redis Streams.
type Runtime struct {
	workers        []WorkerConfig
	services       *registry.ServiceRegistry
	nodes          *registry.NodeRegistry
	workflows      map[string]map[string]any
	workflowCache  *engine.WorkflowCache
	compiler       *expr.Compiler
	tracer         oteltrace.Tracer
	logger         *slog.Logger
	middleware     []Middleware
	secretsContext map[string]any

	started bool
	cancel  context.CancelFunc
	opCtx   atomic.Pointer[context.Context] // operation ctx for handler + XAck
	wg      sync.WaitGroup
}

// NewRuntime creates a new worker runtime.
// If compiler is nil, a new one is created. Tracer is optional (nil uses noop).
func NewRuntime(
	workers []WorkerConfig,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
	workflows map[string]map[string]any,
	workflowCache *engine.WorkflowCache,
	middleware []Middleware,
	compiler *expr.Compiler,
	tracer oteltrace.Tracer,
	logger *slog.Logger,
	secretsContext map[string]any,
) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	if compiler == nil {
		compiler = expr.NewCompilerWithFunctions()
	}
	return &Runtime{
		workers:        workers,
		services:       services,
		nodes:          nodes,
		workflows:      workflows,
		workflowCache:  workflowCache,
		compiler:       compiler,
		tracer:         tracer,
		logger:         logger,
		middleware:     middleware,
		secretsContext: secretsContext,
	}
}

// Start begins consuming messages for all configured workers.
// It is safe to call multiple times; subsequent calls return nil.
func (r *Runtime) Start(ctx context.Context) error {
	if r.started {
		return nil
	}
	r.started = true
	parent := ctx
	r.opCtx.Store(&parent)
	ctx, r.cancel = context.WithCancel(ctx)

	for _, w := range r.workers {
		concurrency := w.Concurrency
		if concurrency < 1 {
			concurrency = 1
		}
		if concurrency > maxConcurrency {
			return fmt.Errorf("worker %q: concurrency %d exceeds maximum %d", w.ID, concurrency, maxConcurrency)
		}

		// Get the stream service
		svcInstance, ok := r.services.Get(w.StreamSvc)
		if !ok {
			return fmt.Errorf("worker %q: stream service %q not found", w.ID, w.StreamSvc)
		}

		provider, ok := svcInstance.(plugin.RedisClientProvider)
		if !ok {
			return fmt.Errorf("worker %q: service %q does not implement RedisClientProvider", w.ID, w.StreamSvc)
		}
		client := provider.Client()

		// Auto-create consumer group
		err := client.XGroupCreateMkStream(ctx, w.Topic, w.Group, "0").Err()
		if err != nil && !redis.HasErrorPrefix(err, "BUSYGROUP") {
			return fmt.Errorf("worker %q: create consumer group: %w", w.ID, err)
		}

		w.Retry = resolveRetry(w.Retry, w.Timeout, r.logger, w.ID)

		for i := 0; i < concurrency; i++ {
			consumerID := fmt.Sprintf("%s-%d", w.ID, i)
			r.wg.Add(1)
			go r.consume(ctx, w, client, consumerID)
		}

		r.wg.Add(1)
		go r.reap(ctx, w, client)

		r.logger.Info("worker started",
			"worker_id", w.ID,
			"topic", w.Topic,
			"group", w.Group,
			"concurrency", concurrency,
		)
	}

	return nil
}

// Stop gracefully shuts down all workers and waits for in-flight processing.
// If ctx is cancelled before all workers finish, Stop returns ctx.Err().
func (r *Runtime) Stop(ctx context.Context) error {
	// Swap opCtx to the shutdown ctx BEFORE cancelling the read loop so that
	// any in-flight handler + XAck picks up the shutdown deadline budget rather
	// than the already-cancelled read ctx.
	shutdown := ctx
	r.opCtx.Store(&shutdown)
	if r.cancel != nil {
		r.cancel()
	}
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		r.logger.Warn("worker shutdown deadline exceeded, some goroutines still running")
		return ctx.Err()
	}
}

// consume is the main loop for a single consumer goroutine.
func (r *Runtime) consume(ctx context.Context, w WorkerConfig, client *redis.Client, consumerID string) {
	defer r.wg.Done()

	for {
		if ctx.Err() != nil {
			return
		}

		streams, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    w.Group,
			Consumer: consumerID,
			Streams:  []string{w.Topic, ">"},
			Count:    1,
			Block:    2 * time.Second,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			r.logger.Error("worker read error",
				"worker_id", w.ID,
				"consumer", consumerID,
				"error", err.Error(),
			)
			time.Sleep(time.Second)
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				r.processMessage(ctx, w, client, consumerID, msg)
			}
		}
	}
}

// maxConcurrency is the upper bound for per-worker concurrency.
const maxConcurrency = 1000

// defaultMessageTimeout is used when no per-worker timeout is configured.
const defaultMessageTimeout = 5 * time.Minute

// defaultMaxAttempts bounds delivery attempts when no dead_letter is configured.
const defaultMaxAttempts = 10

// minIdleFloor is the lowest permitted reclaim idle threshold.
const minIdleFloor = 60 * time.Second

// processMessage handles a single message: maps input, runs the workflow, and
// acks / dead-letters / drops / leaves-pending based on the outcome.
func (r *Runtime) processMessage(ctx context.Context, w WorkerConfig, client *redis.Client, consumerID string, msg redis.XMessage) {
	// Last-resort outer recover: catches panics in the disposition/ack code that
	// follows runMessage (e.g. XAck, XPendingExt, XAdd). runMessage has its own
	// inner recover for the deserialize→handler span; this one only handles what
	// escapes after runMessage returns.  Without it, a panic here would permanently
	// kill the consumer or reaper goroutine.
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("worker.recover: panic escaped message processing",
				"worker_id", w.ID,
				"consumer", consumerID,
				"message_id", msg.ID,
				"panic", fmt.Sprintf("%v", rec),
				"stack", string(debug.Stack()),
			)
		}
	}()

	// ctx is intentionally not used for processing; all ops use the opCtx snapshot
	// below, which outlives r.cancel to honour the graceful-shutdown budget.
	_ = ctx

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
}

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

// getDeliveryAttempts returns how many times a message has been delivered.
func (r *Runtime) getDeliveryAttempts(ctx context.Context, client *redis.Client, topic, group, messageID string) int64 {
	msgs, err := client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: topic,
		Group:  group,
		Start:  messageID,
		End:    messageID,
		Count:  1,
	}).Result()
	if err != nil || len(msgs) == 0 {
		return 0
	}
	return msgs[0].RetryCount
}

// moveToDeadLetter publishes the failed message to the dead letter topic and acks the original.
func (r *Runtime) moveToDeadLetter(ctx context.Context, client *redis.Client, w WorkerConfig, msg redis.XMessage, traceID string, wfErr error) {
	dlPayload := map[string]any{
		"original_topic":      w.Topic,
		"original_group":      w.Group,
		"original_message_id": msg.ID,
		"original_payload":    msg.Values,
		"error":               wfErr.Error(),
		"trace_id":            traceID,
		"dead_lettered_at":    time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(dlPayload)
	if err != nil {
		r.logger.Error("worker dead letter marshal failed",
			"worker_id", w.ID,
			"message_id", msg.ID,
			"error", err.Error(),
		)
		return
	}
	err = client.XAdd(ctx, &redis.XAddArgs{
		Stream: w.DeadLetter.Topic,
		Values: map[string]any{"payload": string(data)},
	}).Err()

	if err != nil {
		r.logger.Error("worker dead letter publish failed",
			"worker_id", w.ID,
			"message_id", msg.ID,
			"trace_id", traceID,
			"error", err.Error(),
		)
		return
	}

	// Ack original message
	if err := client.XAck(ctx, w.Topic, w.Group, msg.ID).Err(); err != nil {
		r.logger.Error("worker ack failed after dead letter",
			"worker_id", w.ID,
			"message_id", msg.ID,
			"trace_id", traceID,
			"error", err.Error(),
		)
	}

	r.logger.Warn("worker message dead lettered",
		"worker_id", w.ID,
		"message_id", msg.ID,
		"trace_id", traceID,
		"dead_letter_topic", w.DeadLetter.Topic,
	)
}

// deserializePayload extracts the payload from a Redis Stream message.
func deserializePayload(values map[string]any) any {
	payloadStr, ok := values["payload"].(string)
	if !ok {
		return values
	}
	var payload any
	if err := json.Unmarshal([]byte(payloadStr), &payload); err != nil {
		return payloadStr
	}
	return payload
}

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
	// Guard so the function is safe even if a caller forgets resolveRetry.
	if maxAttempts <= 0 {
		maxAttempts = defaultMaxAttempts
	}
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

// ParseWorkerConfigs extracts WorkerConfig from raw config maps.
// Keys are sorted for deterministic ordering across runs.
func ParseWorkerConfigs(workers map[string]map[string]any) []WorkerConfig {
	keys := make([]string, 0, len(workers))
	for k := range workers {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var configs []WorkerConfig
	for _, k := range keys {
		raw := workers[k]
		wc := WorkerConfig{
			ID: engine.MapStrVal(raw, "id"),
		}

		if svc, ok := raw["services"].(map[string]any); ok {
			wc.StreamSvc = engine.MapStrVal(svc, "stream")
		}

		if sub, ok := raw["subscribe"].(map[string]any); ok {
			wc.Topic = engine.MapStrVal(sub, "topic")
			wc.Group = engine.MapStrVal(sub, "group")
		}

		if c, ok := raw["concurrency"].(float64); ok {
			wc.Concurrency = int(c)
		}
		if c, ok := raw["concurrency"].(int); ok {
			wc.Concurrency = c
		}

		if timeoutStr, ok := raw["timeout"].(string); ok {
			if d, err := time.ParseDuration(timeoutStr); err == nil {
				wc.Timeout = d
			}
		}

		if mw, ok := raw["middleware"].([]any); ok {
			for _, m := range mw {
				if s, ok := m.(string); ok {
					wc.Middleware = append(wc.Middleware, s)
				}
			}
		}

		if trigger, ok := raw["trigger"].(map[string]any); ok {
			wc.WorkflowID = engine.MapStrVal(trigger, "workflow")
			if input, ok := trigger["input"].(map[string]any); ok {
				wc.InputMap = input
			}
		}

		if dl, ok := raw["dead_letter"].(map[string]any); ok {
			wc.DeadLetter = &DeadLetterConfig{
				Topic: engine.MapStrVal(dl, "topic"),
			}
			if after, ok := dl["after"].(float64); ok {
				wc.DeadLetter.After = int(after)
			}
			if after, ok := dl["after"].(int); ok {
				wc.DeadLetter.After = after
			}
		}

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

		configs = append(configs, wc)
	}
	return configs
}
