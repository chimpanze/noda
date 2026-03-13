package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
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

// WorkflowRunner executes a compiled workflow graph.
type WorkflowRunner interface {
	RunWorkflow(ctx context.Context, workflowID string, execCtx *engine.ExecutionContextImpl) error
}

// Runtime manages worker consumers that process messages from Redis Streams.
type Runtime struct {
	workers       []WorkerConfig
	services      *registry.ServiceRegistry
	nodes         *registry.NodeRegistry
	workflows     map[string]map[string]any
	workflowCache *engine.WorkflowCache
	compiler      *expr.Compiler
	logger        *slog.Logger
	middleware    []Middleware

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewRuntime creates a new worker runtime.
// If compiler is nil, a new one is created.
func NewRuntime(
	workers []WorkerConfig,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
	workflows map[string]map[string]any,
	workflowCache *engine.WorkflowCache,
	middleware []Middleware,
	compiler *expr.Compiler,
	logger *slog.Logger,
) *Runtime {
	if logger == nil {
		logger = slog.Default()
	}
	if compiler == nil {
		compiler = expr.NewCompilerWithFunctions()
	}
	return &Runtime{
		workers:       workers,
		services:      services,
		nodes:         nodes,
		workflows:     workflows,
		workflowCache: workflowCache,
		compiler:      compiler,
		logger:        logger,
		middleware:    middleware,
	}
}

// Start begins consuming messages for all configured workers.
func (r *Runtime) Start(ctx context.Context) error {
	ctx, r.cancel = context.WithCancel(ctx)

	for _, w := range r.workers {
		w := w
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
	input, err := r.resolveInput(w.InputMap, messageCtx)
	if err != nil {
		r.logger.Error("worker input mapping failed",
			"worker_id", w.ID,
			"message_id", msg.ID,
			"trace_id", traceID,
			"error", err.Error(),
		)
		// Ack the message to prevent infinite retry of bad mappings
		client.XAck(ctx, w.Topic, w.Group, msg.ID)
		return
	}

	// Build execution context
	execCtx := engine.NewExecutionContext(
		engine.WithInput(input),
		engine.WithTrigger(api.TriggerData{
			Type:      "event",
			Timestamp: time.Now(),
			TraceID:   traceID,
		}),
		engine.WithWorkflowID(w.WorkflowID),
		engine.WithLogger(r.logger),
		engine.WithCompiler(r.compiler),
	)

	// Build the handler chain (workflow execution wrapped in middleware)
	handler := func(ctx context.Context) error {
		return r.executeWorkflow(ctx, w.WorkflowID, execCtx)
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

// executeWorkflow runs a workflow, using the cache if available or compiling on the fly.
func (r *Runtime) executeWorkflow(ctx context.Context, workflowID string, execCtx *engine.ExecutionContextImpl) error {
	if r.workflowCache != nil {
		graph, ok := r.workflowCache.Get(workflowID)
		if !ok {
			return fmt.Errorf("workflow %q not found", workflowID)
		}
		return engine.ExecuteGraph(ctx, graph, execCtx, r.services, r.nodes)
	}

	// Fallback: compile on the fly (used in tests without a cache)
	wfData, ok := r.workflows[workflowID]
	if !ok {
		return fmt.Errorf("workflow %q not found", workflowID)
	}
	wfConfig, err := engine.ParseWorkflowFromMap(workflowID, wfData)
	if err != nil {
		return fmt.Errorf("parse workflow %q: %w", workflowID, err)
	}
	graph, err := engine.Compile(wfConfig, r.nodes)
	if err != nil {
		return fmt.Errorf("compile workflow %q: %w", workflowID, err)
	}
	return engine.ExecuteGraph(ctx, graph, execCtx, r.services, r.nodes)
}

// resolveInput evaluates trigger input mapping expressions against the message context.
func (r *Runtime) resolveInput(inputMap map[string]any, messageCtx map[string]any) (map[string]any, error) {
	if inputMap == nil {
		return map[string]any{}, nil
	}

	resolver := expr.NewResolver(r.compiler, messageCtx)
	result := make(map[string]any)

	for key, exprVal := range inputMap {
		exprStr, ok := exprVal.(string)
		if !ok {
			result[key] = exprVal
			continue
		}

		resolved, err := resolver.Resolve(exprStr)
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", key, err)
		}
		result[key] = resolved
	}

	return result, nil
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

	data, _ := json.Marshal(dlPayload)
	err := client.XAdd(ctx, &redis.XAddArgs{
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
	client.XAck(ctx, w.Topic, w.Group, msg.ID)

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
func ParseWorkerConfigs(workers map[string]map[string]any) []WorkerConfig {
	var configs []WorkerConfig
	for _, raw := range workers {
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
