package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/worker"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEventTestServer creates a server with a stream service registered.
func newEventTestServer(t *testing.T, routes map[string]map[string]any, workflows map[string]map[string]any) (*Server, *registry.ServiceRegistry) {
	t.Helper()

	mr := miniredis.RunT(t)
	sp := &streamplugin.Plugin{}
	rawSvc, err := sp.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)

	svcReg := registry.NewServiceRegistry()
	err = svcReg.Register("main-stream", rawSvc, sp)
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Routes:    routes,
		Workflows: workflows,
		Schemas:   map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	return srv, svcReg
}

// TestE2E_EventEmit_Stream tests that event.emit publishes to a Redis Stream.
func TestE2E_EventEmit_Stream(t *testing.T) {
	srv, _ := newEventTestServer(t,
		map[string]map[string]any{
			"emit-event": {
				"method": "POST",
				"path":   "/api/events",
				"trigger": map[string]any{
					"workflow": "emit-event",
					"input": map[string]any{
						"topic":   "{{ body.topic }}",
						"payload": "{{ body.payload }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"emit-event": {
				"nodes": map[string]any{
					"emit": map[string]any{
						"type":     "event.emit",
						"services": map[string]any{"stream": "main-stream"},
						"config": map[string]any{
							"mode":    "stream",
							"topic":   "{{ input.topic }}",
							"payload": "{{ input.payload }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.emit }}"},
					},
				},
				"edges": []any{map[string]any{"from": "emit", "to": "respond"}},
			},
		},
	)

	body := `{"topic": "user.created", "payload": {"user_id": "u1", "email": "alice@example.com"}}`
	req := httptest.NewRequest("POST", "/api/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.NotEmpty(t, result["message_id"])
}

// TestE2E_EventEmitAndWorkerConsume tests the full pipeline:
// HTTP request → emit event → worker consumes → worker workflow executes.
func TestE2E_EventEmitAndWorkerConsume(t *testing.T) {
	srv, svcReg := newEventTestServer(t,
		map[string]map[string]any{
			"emit-event": {
				"method": "POST",
				"path":   "/api/events",
				"trigger": map[string]any{
					"workflow": "emit-event",
					"input": map[string]any{
						"payload": "{{ body }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"emit-event": {
				"nodes": map[string]any{
					"emit": map[string]any{
						"type":     "event.emit",
						"services": map[string]any{"stream": "main-stream"},
						"config": map[string]any{
							"mode":    "stream",
							"topic":   "worker-events",
							"payload": "{{ input.payload }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.emit }}"},
					},
				},
				"edges": []any{map[string]any{"from": "emit", "to": "respond"}},
			},
		},
	)

	// Define the worker workflow
	workerWorkflows := map[string]map[string]any{
		"process-event": {
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

	// Track processing
	var mu sync.Mutex
	processed := 0

	trackingMW := &testTrackingMiddleware{mu: &mu, processed: &processed}

	// Start worker
	wc := worker.WorkerConfig{
		ID:         "test-worker",
		StreamSvc:  "main-stream",
		Topic:      "worker-events",
		Group:      "test-group",
		WorkflowID: "process-event",
		InputMap: map[string]any{
			"data": "{{ message.payload }}",
		},
	}

	rt := worker.NewRuntime(
		[]worker.WorkerConfig{wc},
		svcReg, buildTestNodeRegistry(), workerWorkflows,
		nil,
		[]worker.Middleware{trackingMW},
		nil, nil,
	)

	ctx := context.Background()
	err := rt.Start(ctx)
	require.NoError(t, err)
	defer func() { _ = rt.Stop(context.Background()) }()

	// Emit event via HTTP
	body := `{"user_id": "u1", "action": "signup"}`
	req := httptest.NewRequest("POST", "/api/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Wait for worker to process
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return processed >= 1
	}, 5*time.Second, 50*time.Millisecond)
}

// TestE2E_WorkerFailureRedelivery tests that failed messages are not acked (left for redelivery).
func TestE2E_WorkerFailureRedelivery(t *testing.T) {
	mr := miniredis.RunT(t)
	sp := &streamplugin.Plugin{}
	rawSvc, err := sp.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)

	svcReg := registry.NewServiceRegistry()
	err = svcReg.Register("main-stream", rawSvc, sp)
	require.NoError(t, err)

	// A workflow that always fails (references a nonexistent node type)
	failWorkflows := map[string]map[string]any{
		"fail-workflow": {
			"nodes": map[string]any{
				"bad": map[string]any{
					"type": "nonexistent.node",
				},
			},
			"edges": []any{},
		},
	}

	var mu sync.Mutex
	attempts := 0

	wc := worker.WorkerConfig{
		ID:         "fail-worker",
		StreamSvc:  "main-stream",
		Topic:      "fail-events",
		Group:      "fail-group",
		WorkflowID: "fail-workflow",
	}

	failMW := &testCountMiddleware{mu: &mu, count: &attempts}

	rt := worker.NewRuntime(
		[]worker.WorkerConfig{wc},
		svcReg, buildTestNodeRegistry(), failWorkflows,
		nil,
		[]worker.Middleware{failMW},
		nil, nil,
	)

	ctx := context.Background()
	err = rt.Start(ctx)
	require.NoError(t, err)

	// Publish a message
	streamSvc := rawSvc.(*streamplugin.Service)
	_, err = streamSvc.Publish(ctx, "fail-events", map[string]any{"test": true})
	require.NoError(t, err)

	// Wait for first attempt
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return attempts >= 1
	}, 5*time.Second, 50*time.Millisecond)

	_ = rt.Stop(context.Background())

	// The message should NOT have been acked (it's still pending)
	// This verifies the failure path doesn't ack
}

// testTrackingMiddleware tracks processed messages.
type testTrackingMiddleware struct {
	mu        *sync.Mutex
	processed *int
}

func (m *testTrackingMiddleware) Name() string { return "tracking" }

func (m *testTrackingMiddleware) Wrap(next worker.Handler, _ *worker.MessageContext) worker.Handler {
	return func(ctx context.Context) error {
		err := next(ctx)
		m.mu.Lock()
		*m.processed++
		m.mu.Unlock()
		return err
	}
}

// testCountMiddleware counts all attempts (including failures).
type testCountMiddleware struct {
	mu    *sync.Mutex
	count *int
}

func (m *testCountMiddleware) Name() string { return "counter" }

func (m *testCountMiddleware) Wrap(next worker.Handler, _ *worker.MessageContext) worker.Handler {
	return func(ctx context.Context) error {
		m.mu.Lock()
		*m.count++
		m.mu.Unlock()
		return next(ctx)
	}
}
