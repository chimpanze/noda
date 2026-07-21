package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/util"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSetup(t *testing.T) (*redis.Client, *registry.ServiceRegistry, *registry.NodeRegistry, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// Create stream service
	streamSvc := &streamplugin.Service{}
	// We need to use the plugin's CreateService
	sp := &streamplugin.Plugin{}
	rawSvc, err := sp.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)
	_ = streamSvc

	svcReg := registry.NewServiceRegistry()
	err = svcReg.Register("main-stream", rawSvc, sp)
	require.NoError(t, err)

	nodeReg := registry.NewNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&transform.Plugin{})
	_ = nodeReg.RegisterFromPlugin(&util.Plugin{})

	return client, svcReg, nodeReg, mr
}

func TestParseWorkerConfigs(t *testing.T) {
	raw := map[string]map[string]any{
		"workers/notifications.json": {
			"id": "process-notifications",
			"services": map[string]any{
				"stream": "main-stream",
			},
			"subscribe": map[string]any{
				"topic": "user.created",
				"group": "notification-workers",
			},
			"concurrency": float64(5),
			"middleware":  []any{"worker.log", "worker.timeout"},
			"trigger": map[string]any{
				"workflow": "send-welcome-email",
				"input": map[string]any{
					"email": "{{ message.payload.email }}",
					"name":  "{{ message.payload.name }}",
				},
			},
			"dead_letter": map[string]any{
				"topic": "notifications.failed",
				"after": float64(3),
			},
		},
	}

	configs := ParseWorkerConfigs(raw)
	require.Len(t, configs, 1)

	wc := configs[0]
	assert.Equal(t, "process-notifications", wc.ID)
	assert.Equal(t, "main-stream", wc.StreamSvc)
	assert.Equal(t, "user.created", wc.Topic)
	assert.Equal(t, "notification-workers", wc.Group)
	assert.Equal(t, 5, wc.Concurrency)
	assert.Equal(t, []string{"worker.log", "worker.timeout"}, wc.Middleware)
	assert.Equal(t, "send-welcome-email", wc.WorkflowID)
	assert.Equal(t, "{{ message.payload.email }}", wc.InputMap["email"])
	require.NotNil(t, wc.DeadLetter)
	assert.Equal(t, "notifications.failed", wc.DeadLetter.Topic)
	assert.Equal(t, 3, wc.DeadLetter.After)
}

func TestRuntime_ConsumesAndExecutes(t *testing.T) {
	client, svcReg, nodeReg, _ := newTestSetup(t)
	ctx := context.Background()

	// Use a simple workflow with transform.set that stores a value we can check
	// The "side effect" is that the workflow runs without error
	workflows := map[string]map[string]any{
		"test-workflow": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "processing event",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	var mu sync.Mutex
	processed := 0

	wc := WorkerConfig{
		ID:          "test-worker",
		StreamSvc:   "main-stream",
		Topic:       "test-events",
		Group:       "test-group",
		Concurrency: 1,
		WorkflowID:  "test-workflow",
		InputMap:    map[string]any{"data": "{{ message.payload }}"},
	}

	// Create a custom middleware that tracks processing
	trackingMW := &trackingMiddleware{
		mu:        &mu,
		processed: &processed,
	}

	rt := NewRuntime(
		[]WorkerConfig{wc},
		svcReg, nodeReg, workflows,
		nil,
		[]Middleware{trackingMW},
		nil, nil, nil, nil,
	)

	err := rt.Start(ctx)
	require.NoError(t, err)

	// Publish a message
	data, _ := json.Marshal(map[string]any{"key": "value"})
	client.XAdd(ctx, &redis.XAddArgs{
		Stream: "test-events",
		Values: map[string]any{"payload": string(data)},
	})

	// Wait for processing
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return processed >= 1
	}, 5*time.Second, 50*time.Millisecond)

	_ = rt.Stop(context.Background())
}

func TestRuntime_ConcurrentProcessing(t *testing.T) {
	client, svcReg, nodeReg, _ := newTestSetup(t)
	ctx := context.Background()

	workflows := map[string]map[string]any{
		"test-workflow": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "concurrent processing",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	var mu sync.Mutex
	processed := 0

	wc := WorkerConfig{
		ID:          "concurrent-worker",
		StreamSvc:   "main-stream",
		Topic:       "concurrent-events",
		Group:       "concurrent-group",
		Concurrency: 3,
		WorkflowID:  "test-workflow",
		InputMap:    map[string]any{},
	}

	trackingMW := &trackingMiddleware{mu: &mu, processed: &processed}

	rt := NewRuntime(
		[]WorkerConfig{wc},
		svcReg, nodeReg, workflows,
		nil,
		[]Middleware{trackingMW},
		nil, nil, nil, nil,
	)

	err := rt.Start(ctx)
	require.NoError(t, err)

	// Publish multiple messages
	for i := range 5 {
		data, _ := json.Marshal(map[string]any{"i": i})
		client.XAdd(ctx, &redis.XAddArgs{
			Stream: "concurrent-events",
			Values: map[string]any{"payload": string(data)},
		})
	}

	// Wait for all to be processed
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return processed >= 5
	}, 10*time.Second, 50*time.Millisecond)

	_ = rt.Stop(context.Background())
}

func TestRuntime_GracefulShutdown(t *testing.T) {
	_, svcReg, nodeReg, _ := newTestSetup(t)
	ctx := context.Background()

	workflows := map[string]map[string]any{
		"test-workflow": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type":   "util.log",
					"config": map[string]any{"message": "ok", "level": "info"},
				},
			},
			"edges": []any{},
		},
	}

	rt := NewRuntime(
		[]WorkerConfig{{
			ID:         "shutdown-worker",
			StreamSvc:  "main-stream",
			Topic:      "shutdown-topic",
			Group:      "shutdown-group",
			WorkflowID: "test-workflow",
		}},
		svcReg, nodeReg, workflows,
		nil, nil, nil, nil, nil, nil,
	)

	err := rt.Start(ctx)
	require.NoError(t, err)

	// Stop should return quickly
	done := make(chan struct{})
	go func() {
		_ = rt.Stop(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(5 * time.Second):
		t.Fatal("graceful shutdown took too long")
	}
}

func TestRuntime_MissingStreamService(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	nodeReg := registry.NewNodeRegistry()

	rt := NewRuntime(
		[]WorkerConfig{{
			ID:         "bad-worker",
			StreamSvc:  "nonexistent",
			Topic:      "t",
			Group:      "g",
			WorkflowID: "wf",
		}},
		svcReg, nodeReg, nil,
		nil, nil, nil, nil, nil, nil,
	)

	err := rt.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestRuntime_TriggerMapping(t *testing.T) {
	client, svcReg, nodeReg, _ := newTestSetup(t)
	ctx := context.Background()

	// Use a workflow that resolves input expressions
	workflows := map[string]map[string]any{
		"mapping-workflow": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "{{ input.email }}",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	var mu sync.Mutex
	processed := 0

	wc := WorkerConfig{
		ID:         "mapping-worker",
		StreamSvc:  "main-stream",
		Topic:      "mapping-events",
		Group:      "mapping-group",
		WorkflowID: "mapping-workflow",
		InputMap: map[string]any{
			"email": "{{ message.payload.email }}",
			"name":  "{{ message.payload.name }}",
		},
	}

	trackingMW := &trackingMiddleware{mu: &mu, processed: &processed}

	rt := NewRuntime(
		[]WorkerConfig{wc},
		svcReg, nodeReg, workflows,
		nil,
		[]Middleware{trackingMW},
		nil, nil, nil, nil,
	)

	err := rt.Start(ctx)
	require.NoError(t, err)

	// Publish with payload
	data, _ := json.Marshal(map[string]any{"email": "alice@example.com", "name": "Alice"})
	client.XAdd(ctx, &redis.XAddArgs{
		Stream: "mapping-events",
		Values: map[string]any{"payload": string(data)},
	})

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return processed >= 1
	}, 5*time.Second, 50*time.Millisecond)

	_ = rt.Stop(context.Background())
}

// panicOnWrapMiddleware panics during Wrap (chain construction), simulating a
// pre-handler panic inside processMessage. (panicMiddleware, further down,
// panics inside the handler invocation instead.)
type panicOnWrapMiddleware struct{}

func (panicOnWrapMiddleware) Name() string { return "panic" }

func (panicOnWrapMiddleware) Wrap(_ Handler, _ *MessageContext) Handler {
	panic("pre-handler boom")
}

// execution-4: a panic in pre-handler setup (here, middleware construction)
// must be recovered so the consumer goroutine survives, not crash the worker.
func TestProcessMessage_RecoversPreHandlerPanic(t *testing.T) {
	client, svcReg, nodeReg, _ := newTestSetup(t)
	workflows := map[string]map[string]any{
		"wf": {"nodes": map[string]any{}, "edges": []any{}},
	}
	wc := WorkerConfig{
		ID: "w", StreamSvc: "main-stream", Topic: "t", Group: "g", WorkflowID: "wf",
	}
	rt := NewRuntime(
		[]WorkerConfig{wc}, svcReg, nodeReg, workflows,
		nil, []Middleware{panicOnWrapMiddleware{}}, nil, nil, nil, nil,
	)
	ctx := context.Background()
	rt.opCtx.Store(&ctx)

	msg := redis.XMessage{ID: "1-0", Values: map[string]any{"payload": "{}"}}
	assert.NotPanics(t, func() {
		rt.processMessage(ctx, wc, client, "consumer-1", msg, -1)
	}, "processMessage must recover a pre-handler panic")
}

type trackingMiddleware struct {
	mu        *sync.Mutex
	processed *int
}

func (m *trackingMiddleware) Name() string { return "tracking" }

func (m *trackingMiddleware) Wrap(next Handler, _ *MessageContext) Handler {
	return func(ctx context.Context) error {
		err := next(ctx)
		m.mu.Lock()
		*m.processed++
		m.mu.Unlock()
		return err
	}
}

func TestDeserializePayload_JSONString(t *testing.T) {
	values := map[string]any{
		"payload": `{"key":"value","num":42}`,
	}
	result := deserializePayload(values)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "value", m["key"])
	assert.Equal(t, float64(42), m["num"])
}

func TestDeserializePayload_InvalidJSON(t *testing.T) {
	values := map[string]any{
		"payload": "not-json-at-all",
	}
	result := deserializePayload(values)
	// Falls back to the raw string
	assert.Equal(t, "not-json-at-all", result)
}

func TestDeserializePayload_NonStringPayload(t *testing.T) {
	values := map[string]any{
		"payload": 12345,
	}
	result := deserializePayload(values)
	// Returns the whole values map when payload is not a string
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 12345, m["payload"])
}

func TestDeserializePayload_NoPayloadKey(t *testing.T) {
	values := map[string]any{
		"other": "data",
	}
	result := deserializePayload(values)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "data", m["other"])
}

func TestResolveInput_NilMap(t *testing.T) {
	rt := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	result, err := engine.ResolveInput(rt.compiler, nil, map[string]any{})
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestResolveInput_NonStringValues(t *testing.T) {
	rt := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	inputMap := map[string]any{
		"count":  42,
		"active": true,
	}
	result, err := engine.ResolveInput(rt.compiler, inputMap, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, 42, result["count"])
	assert.Equal(t, true, result["active"])
}

func TestResolveInput_ExpressionResolution(t *testing.T) {
	rt := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	inputMap := map[string]any{
		"email": "{{ message.payload.email }}",
	}
	messageCtx := map[string]any{
		"message": map[string]any{
			"payload": map[string]any{
				"email": "test@example.com",
			},
		},
	}
	result, err := engine.ResolveInput(rt.compiler, inputMap, messageCtx)
	require.NoError(t, err)
	assert.Equal(t, "test@example.com", result["email"])
}

func TestParseWorkerConfigs_TimeoutParsing(t *testing.T) {
	raw := map[string]map[string]any{
		"w1": {
			"id":      "timeout-worker",
			"timeout": "30s",
			"trigger": map[string]any{
				"workflow": "wf1",
			},
		},
	}
	configs := ParseWorkerConfigs(raw)
	require.Len(t, configs, 1)
	assert.Equal(t, 30*time.Second, configs[0].Timeout)
}

func TestParseWorkerConfigs_IntConcurrency(t *testing.T) {
	raw := map[string]map[string]any{
		"w1": {
			"id":          "int-conc-worker",
			"concurrency": 4,
			"trigger": map[string]any{
				"workflow": "wf1",
			},
		},
	}
	configs := ParseWorkerConfigs(raw)
	require.Len(t, configs, 1)
	assert.Equal(t, 4, configs[0].Concurrency)
}

func TestParseWorkerConfigs_IntDeadLetterAfter(t *testing.T) {
	raw := map[string]map[string]any{
		"w1": {
			"id": "dl-int-worker",
			"dead_letter": map[string]any{
				"topic": "dl-topic",
				"after": 5,
			},
		},
	}
	configs := ParseWorkerConfigs(raw)
	require.Len(t, configs, 1)
	require.NotNil(t, configs[0].DeadLetter)
	assert.Equal(t, 5, configs[0].DeadLetter.After)
	assert.Equal(t, "dl-topic", configs[0].DeadLetter.Topic)
}

func TestParseWorkerConfigs_MinimalConfig(t *testing.T) {
	raw := map[string]map[string]any{
		"w1": {
			"id": "minimal-worker",
		},
	}
	configs := ParseWorkerConfigs(raw)
	require.Len(t, configs, 1)
	assert.Equal(t, "minimal-worker", configs[0].ID)
	assert.Empty(t, configs[0].StreamSvc)
	assert.Empty(t, configs[0].Topic)
	assert.Equal(t, 0, configs[0].Concurrency)
	assert.Nil(t, configs[0].DeadLetter)
	assert.Nil(t, configs[0].Middleware)
}

func TestParseWorkerConfigs_InvalidTimeout(t *testing.T) {
	raw := map[string]map[string]any{
		"w1": {
			"id":      "bad-timeout-worker",
			"timeout": "not-a-duration",
		},
	}
	configs := ParseWorkerConfigs(raw)
	require.Len(t, configs, 1)
	assert.Equal(t, time.Duration(0), configs[0].Timeout)
}

func TestRuntime_MaxConcurrencyExceeded(t *testing.T) {
	_, svcReg, nodeReg, _ := newTestSetup(t)

	rt := NewRuntime(
		[]WorkerConfig{{
			ID:          "max-conc-worker",
			StreamSvc:   "main-stream",
			Topic:       "max-conc-topic",
			Group:       "max-conc-group",
			Concurrency: maxConcurrency + 1,
			WorkflowID:  "wf",
		}},
		svcReg, nodeReg, nil,
		nil, nil, nil, nil, nil, nil,
	)

	err := rt.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

func TestRuntime_NonRedisClientProvider(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	nodeReg := registry.NewNodeRegistry()

	// Register a service that does NOT implement RedisClientProvider
	fakeSvc := &fakeService{}
	err := svcReg.Register("fake-svc", fakeSvc, &fakePlugin{})
	require.NoError(t, err)

	rt := NewRuntime(
		[]WorkerConfig{{
			ID:         "bad-provider-worker",
			StreamSvc:  "fake-svc",
			Topic:      "t",
			Group:      "g",
			WorkflowID: "wf",
		}},
		svcReg, nodeReg, nil,
		nil, nil, nil, nil, nil, nil,
	)

	err = rt.Start(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not implement RedisClientProvider")
}

func TestRuntime_StopWithoutStart(t *testing.T) {
	rt := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	err := rt.Stop(context.Background())
	assert.NoError(t, err)
}

func TestRuntime_StopWithContextTimeout(t *testing.T) {
	_, svcReg, nodeReg, _ := newTestSetup(t)

	workflows := map[string]map[string]any{
		"test-workflow": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type":   "util.log",
					"config": map[string]any{"message": "ok", "level": "info"},
				},
			},
			"edges": []any{},
		},
	}

	rt := NewRuntime(
		[]WorkerConfig{{
			ID:         "timeout-stop-worker",
			StreamSvc:  "main-stream",
			Topic:      "timeout-stop-topic",
			Group:      "timeout-stop-group",
			WorkflowID: "test-workflow",
		}},
		svcReg, nodeReg, workflows,
		nil, nil, nil, nil, nil, nil,
	)

	err := rt.Start(context.Background())
	require.NoError(t, err)

	// Stop with a generous timeout should succeed
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = rt.Stop(stopCtx)
	assert.NoError(t, err)
}

func TestNewRuntime_NilLoggerAndCompiler(t *testing.T) {
	rt := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	assert.NotNil(t, rt)
	assert.NotNil(t, rt.compiler)
	assert.NotNil(t, rt.logger)
}

func TestParseWorkerConfigs_EmptyMap(t *testing.T) {
	configs := ParseWorkerConfigs(map[string]map[string]any{})
	assert.Empty(t, configs)
}

func TestDeserializePayload_JSONArray(t *testing.T) {
	values := map[string]any{
		"payload": `[1,2,3]`,
	}
	result := deserializePayload(values)
	arr, ok := result.([]any)
	require.True(t, ok)
	assert.Len(t, arr, 3)
}

func TestRuntime_XAckLandsDuringShutdown(t *testing.T) {
	client, svcReg, nodeReg, _ := newTestSetup(t)

	const topic, group = "ack-shutdown-topic", "ack-shutdown-group"

	// slow-workflow uses util.delay to introduce a deterministic ~200ms delay.
	// We publish a message, give the worker time to start processing, call Stop
	// with a 2s budget, and verify the message was acked (pending count == 0).
	workflows := map[string]map[string]any{
		"slow-workflow": {
			"nodes": map[string]any{
				"wait": map[string]any{
					"type":   "util.delay",
					"config": map[string]any{"timeout": "200ms"},
				},
			},
			"edges": []any{},
		},
	}

	rt := NewRuntime(
		[]WorkerConfig{{
			ID:         "ack-shutdown-worker",
			StreamSvc:  "main-stream",
			Topic:      topic,
			Group:      group,
			WorkflowID: "slow-workflow",
		}},
		svcReg, nodeReg, workflows,
		nil, nil, nil, nil, nil, nil,
	)

	// Auto-create the consumer group before publishing.
	require.NoError(t, client.XGroupCreateMkStream(context.Background(), topic, group, "0").Err())

	// Publish a message.
	id, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: topic,
		Values: map[string]any{"payload": `{"x":1}`},
	}).Result()
	require.NoError(t, err)

	require.NoError(t, rt.Start(context.Background()))

	// Give the worker a beat to pick up the message and enter the handler.
	time.Sleep(50 * time.Millisecond)

	// Begin Stop with a 2s budget — well over the 200ms handler delay.
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	require.NoError(t, rt.Stop(stopCtx))

	// The message should have been acked: pending count == 0.
	pending, err := client.XPending(context.Background(), topic, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending.Count,
		"message %s should be acked after graceful shutdown", id)
}

// fakeService is a service that does not implement RedisClientProvider.
type fakeService struct{}

func (f *fakeService) Close() error { return nil }

// fakePlugin satisfies the api.Plugin interface.
type fakePlugin struct{}

func (f *fakePlugin) Name() string                                     { return "fake" }
func (f *fakePlugin) Prefix() string                                   { return "fake" }
func (f *fakePlugin) Nodes() []api.NodeRegistration                    { return nil }
func (f *fakePlugin) HasServices() bool                                { return true }
func (f *fakePlugin) ServiceConfigSchema() map[string]any              { return nil }
func (f *fakePlugin) CreateService(config map[string]any) (any, error) { return &fakeService{}, nil }
func (f *fakePlugin) HealthCheck(service any) error                    { return nil }
func (f *fakePlugin) Shutdown(service any) error                       { return nil }

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
	// No timeout configured -> effective timeout is defaultMessageTimeout (5m),
	// plus the safety margin so the reaper can't claim during the ack window.
	got := resolveRetry(RetryConfig{}, 0, logger, "w")
	assert.Equal(t, defaultMessageTimeout+minIdleMargin, got.MinIdle)
	assert.Equal(t, defaultMaxAttempts, got.MaxAttempts)
}

func TestResolveRetry_ClampsMinIdleUpToTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// min_idle 10s is below a 30s timeout -> clamp up to timeout+margin (60s),
	// which also satisfies the 60s floor.
	got := resolveRetry(RetryConfig{MinIdle: 10 * time.Second, MaxAttempts: 3}, 30*time.Second, logger, "w")
	assert.Equal(t, 60*time.Second, got.MinIdle)
	assert.Equal(t, 3, got.MaxAttempts)
}

func TestResolveRetry_MinIdleGetsMarginOverTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// The Redis idle clock starts at delivery, before the handler's timeout
	// clock, and the consumer still needs to XAck after the handler returns.
	// min_idle equal to (or barely above) the timeout leaves no headroom for
	// that, so it must be clamped up to timeout+margin, not just timeout.
	got := resolveRetry(RetryConfig{MinIdle: 100 * time.Second, MaxAttempts: 3}, 90*time.Second, logger, "w")
	assert.Equal(t, 90*time.Second+minIdleMargin, got.MinIdle)
}

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
		r.processMessage(context.Background(), w, client, "c", msg, -1)
	})

	// Not acked -> still pending (attempt 1 < maxAttempts, no dead_letter).
	pending, err := client.XPending(context.Background(), topic, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), pending.Count)
}

// flakyMiddleware fails the first N handler invocations, then succeeds.
type flakyMiddleware struct {
	mu      sync.Mutex
	failFor int
	calls   int
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
		"wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type":   "util.log",
					"config": map[string]any{"message": "ok", "level": "info"},
				},
			},
			"edges": []any{},
		},
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
	r.processMessage(context.Background(), w, client, "c", streams[0].Messages[0], -1)

	pending, _ := client.XPending(context.Background(), topic, group).Result()
	require.Equal(t, int64(1), pending.Count)

	// Before min_idle elapses, reapOnce reclaims nothing.
	require.NoError(t, r.reapOnce(context.Background(), w, client))
	pending, _ = client.XPending(context.Background(), topic, group).Result()
	require.Equal(t, int64(1), pending.Count)

	// Advance past min_idle; now reapOnce reclaims and reprocesses -> succeeds -> acked.
	mr.SetTime(time.Now().Add(61 * time.Second))
	require.NoError(t, r.reapOnce(context.Background(), w, client))
	pending, _ = client.XPending(context.Background(), topic, group).Result()
	assert.Equal(t, int64(0), pending.Count)
}

// TestReapOnce_DrainsBacklogAtConcurrency guards against a regression where
// reducing XAutoClaim's Count to the worker's concurrency (instead of a fixed
// page size) would cause the reaper to leave part of a multi-message backlog
// unclaimed. The cursor-paged loop in reapOnce must still walk the whole
// pending set and reclaim+process every idle message, even when each page is
// only as large as the configured concurrency.
func TestReapOnce_DrainsBacklogAtConcurrency(t *testing.T) {
	client, svcReg, nodeReg, mr := newTestSetup(t)
	topic, group := "t-reap-backlog", "g-reap-backlog"
	require.NoError(t, client.XGroupCreateMkStream(context.Background(), topic, group, "0").Err())

	const n = 4
	for i := range n {
		_, err := client.XAdd(context.Background(), &redis.XAddArgs{
			Stream: topic, Values: map[string]any{"payload": fmt.Sprintf(`{"x":%d}`, i)},
		}).Result()
		require.NoError(t, err)
	}

	w := WorkerConfig{
		ID: "w", Topic: topic, Group: group, WorkflowID: "wf", Concurrency: 2,
		Retry: RetryConfig{MinIdle: 60 * time.Second, MaxAttempts: 10},
	}
	r := NewRuntime([]WorkerConfig{w}, svcReg, nodeReg, map[string]map[string]any{
		"wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type":   "util.log",
					"config": map[string]any{"message": "ok", "level": "info"},
				},
			},
			"edges": []any{},
		},
	}, nil, nil, nil, nil, nil, nil)
	parent := context.Background()
	r.opCtx.Store(&parent)

	// Deliver all n messages to a dead consumer so they sit pending.
	_, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "dead", Streams: []string{topic, ">"}, Count: n,
	}).Result()
	require.NoError(t, err)

	pending, _ := client.XPending(context.Background(), topic, group).Result()
	require.Equal(t, int64(n), pending.Count)

	// Advance past min_idle so every message is eligible for reclaim.
	mr.SetTime(time.Now().Add(61 * time.Second))

	// A single reapOnce call, paging with Count == concurrency (2), must still
	// drain and process the entire backlog of n=4 messages.
	require.NoError(t, r.reapOnce(context.Background(), w, client))

	pending, _ = client.XPending(context.Background(), topic, group).Result()
	assert.Equal(t, int64(0), pending.Count, "reapOnce must drain the full backlog even with Count==concurrency")
}

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

	// Attempt 1 (fresh delivery via XReadGroup, delivery count = 1).
	streams, _ := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c", Streams: []string{topic, ">"}, Count: 1,
	}).Result()
	r.processMessage(context.Background(), w, client, "c", streams[0].Messages[0], -1)

	// Attempts 2 and 3 via reclaim: each XAutoClaim bumps delivery count by 1.
	// After 2 reapOnce calls the count reaches 3 (>= after=3) and the message is dead-lettered.
	// Clock must advance cumulatively so each claim sees the message idle > min_idle.
	base := time.Now()
	for range 2 {
		base = base.Add(61 * time.Second)
		mr.SetTime(base)
		require.NoError(t, r.reapOnce(context.Background(), w, client))
	}

	// Original acked (drained from PEL) and a message landed on the DLQ.
	pending, _ := client.XPending(context.Background(), topic, group).Result()
	assert.Equal(t, int64(0), pending.Count)
	dlLen, err := client.XLen(context.Background(), dlq).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), dlLen)
}

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

	// Attempt 1 (fresh delivery, count = 1).
	streams, _ := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c", Streams: []string{topic, ">"}, Count: 1,
	}).Result()
	r.processMessage(context.Background(), w, client, "c", streams[0].Messages[0], -1)

	// Attempts 2 and 3 via reclaim; after 2 reapOnce calls count reaches 3 (>= max_attempts=3) → drop.
	base := time.Now()
	for range 2 {
		base = base.Add(61 * time.Second)
		mr.SetTime(base)
		require.NoError(t, r.reapOnce(context.Background(), w, client))
	}

	// Dropped (acked) after reaching max_attempts; PEL empty, nothing re-queued.
	pending, _ := client.XPending(context.Background(), topic, group).Result()
	assert.Equal(t, int64(0), pending.Count)
}

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
		{"dl set never hard-drops before after", 9, dl, 5, actionDeadLetter},
		// Raw-function guard only: resolveDeadLetter defaults After to
		// max_attempts at startup, so a topic-only DLQ config never reaches
		// this function with After == 0 in practice.
		{"dl with After<=0 treated as no dl", 10, &DeadLetterConfig{Topic: "x"}, 10, actionDrop},
		// maxAttempts=0 must default to defaultMaxAttempts (10), not drop on attempt 1.
		{"maxAttempts=0 defaults, low attempt -> pending", 1, nil, 0, actionPending},
		{"maxAttempts=0 defaults, at default cap -> drop", defaultMaxAttempts, nil, 0, actionDrop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, decideFailureDisposition(tt.attempts, tt.dl, tt.maxAttempts))
		})
	}
}

// TestProcessMessage_OuterRecover_Survives verifies that the last-resort outer
// recover in processMessage catches panics that occur after runMessage returns
// (i.e. in the disposition/ack code path). We trigger this deterministically by
// passing a nil *redis.Client: runMessage itself succeeds (empty workflow, no
// client needed), but the subsequent client.XAck call panics on a nil receiver.
// The outer defer must absorb that panic so the caller does not crash.
func TestProcessMessage_OuterRecover_Survives(t *testing.T) {
	r := NewRuntime(nil, nil, nil, map[string]map[string]any{
		"wf": {
			"nodes": map[string]any{},
			"edges": []any{},
		},
	}, nil, nil, nil, nil, nil, nil)
	parent := context.Background()
	r.opCtx.Store(&parent)

	w := WorkerConfig{
		ID:         "outer-recover-worker",
		Topic:      "t-outer",
		Group:      "g-outer",
		WorkflowID: "wf",
		Retry:      RetryConfig{MinIdle: 60 * time.Second, MaxAttempts: 10},
	}

	msg := redis.XMessage{
		ID:     "1-0",
		Values: map[string]any{"payload": `{"x":1}`},
	}

	// A nil client causes client.XAck to panic with a nil pointer dereference
	// after runMessage returns successfully. The outer recover must absorb it.
	require.NotPanics(t, func() {
		r.processMessage(context.Background(), w, nil, "c", msg, -1)
	})
}

// hangMiddleware simulates a workflow that blocks until the handler context
// expires (e.g. a stuck external call), returning the context error.
type hangMiddleware struct{}

func (hangMiddleware) Name() string { return "test.hang" }
func (hangMiddleware) Wrap(next Handler, _ *MessageContext) Handler {
	return func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
}

// A message that fails by exhausting the handler timeout must still be
// dispositioned. The disposition ops (XPendingExt/XAdd/XAck) cannot run on the
// same context whose budget the handler just consumed, or they all fail and
// the message retries forever regardless of dead_letter.after / max_attempts.
func TestProcessMessage_TimeoutFailure_StillDeadLettered(t *testing.T) {
	client, svcReg, nodeReg, _ := newTestSetup(t)
	topic, group, dlq := "t-timeout", "g-timeout", "t-timeout.dlq"
	require.NoError(t, client.XGroupCreateMkStream(context.Background(), topic, group, "0").Err())
	_, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: topic, Values: map[string]any{"payload": `{"x":1}`},
	}).Result()
	require.NoError(t, err)

	w := WorkerConfig{
		ID: "w", Topic: topic, Group: group, WorkflowID: "wf",
		Timeout:    50 * time.Millisecond,
		Retry:      RetryConfig{MinIdle: 60 * time.Second, MaxAttempts: 10},
		DeadLetter: &DeadLetterConfig{Topic: dlq, After: 1},
	}
	r := NewRuntime([]WorkerConfig{w}, svcReg, nodeReg, map[string]map[string]any{
		"wf": {"nodes": map[string]any{}},
	}, nil, nil, nil, nil, nil, nil)
	r.middleware = []Middleware{hangMiddleware{}}
	parent := context.Background()
	r.opCtx.Store(&parent)

	streams, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c", Streams: []string{topic, ">"}, Count: 1,
	}).Result()
	require.NoError(t, err)
	r.processMessage(context.Background(), w, client, "c", streams[0].Messages[0], -1)

	// Delivery count 1 >= after=1: dead-lettered despite the exhausted budget.
	pending, err := client.XPending(context.Background(), topic, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending.Count)
	dlLen, err := client.XLen(context.Background(), dlq).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), dlLen)
}

func TestResolveDeadLetter(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// nil passes through.
	assert.Nil(t, resolveDeadLetter(nil, 10, logger, "w"))

	// Empty topic is invalid: disable the DLQ loudly rather than publishing
	// into a stream literally named "".
	assert.Nil(t, resolveDeadLetter(&DeadLetterConfig{After: 3}, 10, logger, "w"))

	// Topic without after: default after to maxAttempts so poison messages are
	// dead-lettered instead of silently ack-dropped by the max-attempts cap.
	got := resolveDeadLetter(&DeadLetterConfig{Topic: "dlq"}, 7, logger, "w")
	require.NotNil(t, got)
	assert.Equal(t, 7, got.After)

	// Fully specified passes through unchanged.
	got = resolveDeadLetter(&DeadLetterConfig{Topic: "dlq", After: 3}, 10, logger, "w")
	require.NotNil(t, got)
	assert.Equal(t, "dlq", got.Topic)
	assert.Equal(t, 3, got.After)
}

// The pre-reclaim docs described dead-lettering as retry.dlq; honor that shape
// instead of silently ignoring it (which would ack-drop after max_attempts).
func TestParseWorkerConfigs_LegacyRetryDLQ(t *testing.T) {
	raw := map[string]map[string]any{
		"workers/w.json": {
			"id":        "w",
			"services":  map[string]any{"stream": "main-stream"},
			"subscribe": map[string]any{"topic": "t", "group": "g"},
			"trigger":   map[string]any{"workflow": "wf"},
			"retry":     map[string]any{"max_attempts": float64(3), "dlq": "orders.failed"},
		},
	}
	configs := ParseWorkerConfigs(raw)
	require.Len(t, configs, 1)
	require.NotNil(t, configs[0].DeadLetter)
	assert.Equal(t, "orders.failed", configs[0].DeadLetter.Topic)
	assert.Equal(t, 3, configs[0].Retry.MaxAttempts)
}

func TestParseWorkerConfigs_DeadLetterBlockWinsOverLegacyDLQ(t *testing.T) {
	raw := map[string]map[string]any{
		"workers/w.json": {
			"id":          "w",
			"services":    map[string]any{"stream": "main-stream"},
			"subscribe":   map[string]any{"topic": "t", "group": "g"},
			"trigger":     map[string]any{"workflow": "wf"},
			"dead_letter": map[string]any{"topic": "explicit.dlq", "after": float64(2)},
			"retry":       map[string]any{"dlq": "legacy.dlq"},
		},
	}
	configs := ParseWorkerConfigs(raw)
	require.Len(t, configs, 1)
	require.NotNil(t, configs[0].DeadLetter)
	assert.Equal(t, "explicit.dlq", configs[0].DeadLetter.Topic)
	assert.Equal(t, 2, configs[0].DeadLetter.After)
}

// barrierMiddleware blocks each invocation until the test releases it, and
// signals entry so the test can prove multiple handlers run concurrently.
type barrierMiddleware struct {
	entered chan struct{}
	release chan struct{}
}

func (m *barrierMiddleware) Name() string { return "test.barrier" }
func (m *barrierMiddleware) Wrap(next Handler, _ *MessageContext) Handler {
	return func(ctx context.Context) error {
		m.entered <- struct{}{}
		select {
		case <-m.release:
			return next(ctx)
		case <-time.After(5 * time.Second):
			return fmt.Errorf("barrier timeout: reclaimed messages are not processed concurrently")
		}
	}
}

// Reclaimed messages must be processed with the worker's configured
// concurrency, not serially in the reaper goroutine — otherwise one slow
// poison message head-of-line-blocks redelivery of everything else.
func TestReapOnce_ProcessesPageConcurrently(t *testing.T) {
	client, svcReg, nodeReg, mr := newTestSetup(t)
	topic, group := "t-parreap", "g-parreap"
	require.NoError(t, client.XGroupCreateMkStream(context.Background(), topic, group, "0").Err())
	for i := range 3 {
		_, err := client.XAdd(context.Background(), &redis.XAddArgs{
			Stream: topic, Values: map[string]any{"payload": fmt.Sprintf(`{"i":%d}`, i)},
		}).Result()
		require.NoError(t, err)
	}

	w := WorkerConfig{
		ID: "w", Topic: topic, Group: group, WorkflowID: "wf", Concurrency: 3,
		Retry: RetryConfig{MinIdle: 60 * time.Second, MaxAttempts: 10},
	}
	r := NewRuntime([]WorkerConfig{w}, svcReg, nodeReg, map[string]map[string]any{
		"wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type":   "util.log",
					"config": map[string]any{"message": "ok", "level": "info"},
				},
			},
			"edges": []any{},
		},
	}, nil, nil, nil, nil, nil, nil)
	mw := &barrierMiddleware{entered: make(chan struct{}, 3), release: make(chan struct{})}
	r.middleware = []Middleware{mw}
	parent := context.Background()
	r.opCtx.Store(&parent)

	// Make all three messages pending without processing them.
	streams, err := client.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group: group, Consumer: "c", Streams: []string{topic, ">"}, Count: 10,
	}).Result()
	require.NoError(t, err)
	require.Len(t, streams[0].Messages, 3)

	mr.SetTime(time.Now().Add(61 * time.Second))

	done := make(chan error, 1)
	go func() { done <- r.reapOnce(context.Background(), w, client) }()

	// All three reclaimed messages must be in-flight simultaneously.
	for i := range 3 {
		select {
		case <-mw.entered:
		case <-time.After(2 * time.Second):
			t.Fatalf("only %d of 3 reclaimed messages processing concurrently", i)
		}
	}
	close(mw.release)
	require.NoError(t, <-done)

	pending, err := client.XPending(context.Background(), topic, group).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending.Count)
}

// Per CLAUDE.md ("Real config files. Test against actual JSON config files in
// testdata/"), the retry and dead_letter fields must round-trip from a real
// worker config file, not just hand-built maps.
func TestParseWorkerConfigs_FromTestdataFixture(t *testing.T) {
	data, err := os.ReadFile("../../testdata/valid-project/workers/notifications.json")
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	configs := ParseWorkerConfigs(map[string]map[string]any{"workers/notifications.json": raw})
	require.Len(t, configs, 1)
	wc := configs[0]
	assert.Equal(t, "process-notifications", wc.ID)
	assert.Equal(t, 10*time.Minute, wc.Retry.MinIdle)
	assert.Equal(t, 5, wc.Retry.MaxAttempts)
	require.NotNil(t, wc.DeadLetter)
	assert.Equal(t, "task.created.dlq", wc.DeadLetter.Topic)
	assert.Equal(t, 3, wc.DeadLetter.After)
}

func TestReapInterval_ScalesWithMinIdle(t *testing.T) {
	// Nothing becomes claimable sooner than min_idle after delivery, so the
	// reaper polls at min_idle/2 (floor 30s) instead of a fixed 30s — a fixed
	// cap scanned ~10x more often than useful at the default min_idle (5m30s).
	assert.Equal(t, 30*time.Second, reapInterval(45*time.Second))
	assert.Equal(t, 30*time.Second, reapInterval(60*time.Second))
	assert.Equal(t, 165*time.Second, reapInterval(330*time.Second)) // default timeout+margin
	assert.Equal(t, 30*time.Minute, reapInterval(time.Hour))
}

func TestPrefetchAttempts_BatchesDeliveryCounts(t *testing.T) {
	client, _, _, _ := newTestSetup(t)
	ctx := context.Background()
	topic, group, consumer := "t-prefetch", "g-prefetch", "w-reaper"
	require.NoError(t, client.XGroupCreateMkStream(ctx, topic, group, "0").Err())

	ids := make([]string, 0, 3)
	for range 3 {
		id, err := client.XAdd(ctx, &redis.XAddArgs{
			Stream: topic, Values: map[string]any{"payload": `{}`},
		}).Result()
		require.NoError(t, err)
		ids = append(ids, id)
	}

	// Deliver all three to `consumer` so they are pending with delivery count 1.
	streams, err := client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group: group, Consumer: consumer, Streams: []string{topic, ">"}, Count: 10,
	}).Result()
	require.NoError(t, err)
	msgs := streams[0].Messages
	require.Len(t, msgs, 3)

	got := prefetchAttempts(ctx, client, topic, group, consumer, msgs)
	require.Len(t, got, 3)
	for _, id := range ids {
		assert.Equal(t, int64(1), got[id])
	}

	// Empty page: no Redis call needed, nil map.
	assert.Nil(t, prefetchAttempts(ctx, client, topic, group, consumer, nil))
}
