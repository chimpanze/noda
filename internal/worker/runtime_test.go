package worker

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
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
	for i := 0; i < 5; i++ {
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

// trackingMiddleware tracks how many messages were processed.
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
		{"dl with After<=0 treated as no dl", 10, &DeadLetterConfig{Topic: "x"}, 10, actionDrop},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, decideFailureDisposition(tt.attempts, tt.dl, tt.maxAttempts))
		})
	}
}
