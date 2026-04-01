package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"sync"
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
}

// DeadLetterConfig holds dead letter queue configuration.
type DeadLetterConfig struct {
	Topic string
	After int
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

		for i := 0; i < concurrency; i++ {
			consumerID := fmt.Sprintf("%s-%d", w.ID, i)
			r.wg.Add(1)
			go r.consume(ctx, w, client, consumerID)
		}

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

// processMessage handles a single message: maps input, runs workflow, acks/nacks.
func (r *Runtime) processMessage(ctx context.Context, w WorkerConfig, client *redis.Client, consumerID string, msg redis.XMessage) {
	timeout := w.Timeout
	if timeout == 0 {
		timeout = defaultMessageTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	traceID := uuid.New().String()
	start := time.Now()

	r.logger.Info("worker processing message",
		"worker_id", w.ID,
		"consumer", consumerID,
		"message_id", msg.ID,
		"trace_id", traceID,
	)

	// Deserialize the payload
	payload := deserializePayload(msg.Values)

	// Build the message context for trigger mapping
	messageCtx := map[string]any{
		"message": map[string]any{
			"id":      msg.ID,
			"payload": payload,
		},
	}

	// Resolve trigger input mapping
	input, err := engine.ResolveInput(r.compiler, w.InputMap, messageCtx)
	if err != nil {
		r.logger.Error("worker input mapping failed",
			"worker_id", w.ID,
			"message_id", msg.ID,
			"trace_id", traceID,
			"error", err.Error(),
		)
		// Ack the message to prevent infinite retry of bad mappings
		if err := client.XAck(ctx, w.Topic, w.Group, msg.ID).Err(); err != nil {
			r.logger.Error("worker ack failed after bad mapping",
				"worker_id", w.ID,
				"message_id", msg.ID,
				"trace_id", traceID,
				"error", err.Error(),
			)
		}
		return
	}

	// Build execution context
	opts := []engine.ExecutionContextOption{
		engine.WithInput(input),
		engine.WithTrigger(api.TriggerData{
			Type:      "event",
			Timestamp: time.Now(),
			TraceID:   traceID,
		}),
		engine.WithWorkflowID(w.WorkflowID),
		engine.WithLogger(r.logger),
		engine.WithCompiler(r.compiler),
		engine.WithSecrets(r.secretsContext),
	}
	if r.tracer != nil {
		opts = append(opts, engine.WithTracer(r.tracer))
	}
	execCtx := engine.NewExecutionContext(opts...)

	// Build the handler chain (workflow execution wrapped in middleware)
	handler := func(ctx context.Context) error {
		return engine.RunWorkflow(ctx, w.WorkflowID, execCtx, r.workflowCache, r.workflows, r.services, r.nodes)
	}

	// Apply middleware in reverse order
	for i := len(r.middleware) - 1; i >= 0; i-- {
		handler = r.middleware[i].Wrap(handler, &MessageContext{
			WorkerID:  w.ID,
			MessageID: msg.ID,
			TraceID:   traceID,
			Topic:     w.Topic,
			Group:     w.Group,
			Logger:    r.logger,
		})
	}

	// Execute
	wfErr := handler(ctx)

	duration := time.Since(start)

	if wfErr != nil {
		r.logger.Error("worker workflow failed",
			"worker_id", w.ID,
			"message_id", msg.ID,
			"trace_id", traceID,
			"duration", duration.String(),
			"error", wfErr.Error(),
		)

		// Check dead letter
		if w.DeadLetter != nil && w.DeadLetter.After > 0 {
			attempts := r.getDeliveryAttempts(ctx, client, w.Topic, w.Group, msg.ID)
			if attempts >= int64(w.DeadLetter.After) {
				r.moveToDeadLetter(ctx, client, w, msg, traceID, wfErr)
				return
			}
		}
		// Leave message as pending for redelivery (don't ack)
		return
	}

	// Success — ack the message
	if err := client.XAck(ctx, w.Topic, w.Group, msg.ID).Err(); err != nil {
		r.logger.Error("worker ack failed",
			"worker_id", w.ID,
			"message_id", msg.ID,
			"trace_id", traceID,
			"error", err.Error(),
		)
	}

	r.logger.Info("worker message processed",
		"worker_id", w.ID,
		"message_id", msg.ID,
		"trace_id", traceID,
		"duration", duration.String(),
	)
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

		configs = append(configs, wc)
	}
	return configs
}
