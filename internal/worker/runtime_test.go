package worker

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/internal/registry"
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
			"middleware":   []any{"worker.log", "worker.timeout"},
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
		[]Middleware{trackingMW},
		nil,
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

	rt.Stop()
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
		[]Middleware{trackingMW},
		nil,
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

	rt.Stop()
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
		nil, nil,
	)

	err := rt.Start(ctx)
	require.NoError(t, err)

	// Stop should return quickly
	done := make(chan struct{})
	go func() {
		rt.Stop()
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
		nil, nil,
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
		[]Middleware{trackingMW},
		nil,
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

	rt.Stop()
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
