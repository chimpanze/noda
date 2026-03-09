package trace

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestNewProvider_Disabled(t *testing.T) {
	p, err := NewProvider(context.Background(), TracerConfig{Enabled: false}, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.NotNil(t, p.Tracer())
	assert.NoError(t, p.Shutdown(context.Background()))
}

func TestNewProvider_Enabled_NoExporter(t *testing.T) {
	p, err := NewProvider(context.Background(), TracerConfig{Enabled: true}, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.NotNil(t, p.Tracer())
	assert.NoError(t, p.Shutdown(context.Background()))
}

func TestNewProvider_OTLP(t *testing.T) {
	// This creates an OTLP exporter pointed at a non-existent endpoint.
	// It should still initialize successfully — export failures happen later.
	p, err := NewProvider(context.Background(), TracerConfig{
		Enabled:  true,
		Exporter: "otlp",
		Endpoint: "localhost:14318",
		Insecure: true,
	}, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.NoError(t, p.Shutdown(context.Background()))
}

func TestParseConfig(t *testing.T) {
	cfg := ParseConfig(map[string]any{
		"observability": map[string]any{
			"tracing": map[string]any{
				"enabled":  true,
				"exporter": "otlp",
				"endpoint": "collector:4318",
				"insecure": true,
			},
		},
	})
	assert.True(t, cfg.Enabled)
	assert.Equal(t, "otlp", cfg.Exporter)
	assert.Equal(t, "collector:4318", cfg.Endpoint)
	assert.True(t, cfg.Insecure)
}

func TestParseConfig_Empty(t *testing.T) {
	cfg := ParseConfig(map[string]any{})
	assert.False(t, cfg.Enabled)
	assert.Empty(t, cfg.Exporter)
}

func TestStartWorkflowSpan(t *testing.T) {
	tracer := noop.NewTracerProvider().Tracer("test")
	ctx, span := StartWorkflowSpan(context.Background(), tracer, "my-wf", "trace-123", "http")
	assert.NotNil(t, ctx)
	assert.NotNil(t, span)
	EndWorkflowSpan(span, nil)
}

func TestStartNodeSpan(t *testing.T) {
	tracer := noop.NewTracerProvider().Tracer("test")
	ctx, span := StartNodeSpan(context.Background(), tracer, "node-1", "transform.set")
	assert.NotNil(t, ctx)
	assert.NotNil(t, span)
	EndNodeSpan(span, "success", nil)
}

func TestStartNodeSpan_WithError(t *testing.T) {
	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := StartNodeSpan(context.Background(), tracer, "node-1", "db.query")
	assert.NotNil(t, span)
	EndNodeSpan(span, "", assert.AnError)
}

func TestEndWorkflowSpan_WithError(t *testing.T) {
	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := StartWorkflowSpan(context.Background(), tracer, "wf", "t", "http")
	EndWorkflowSpan(span, assert.AnError)
}

// --- EventHub tests ---

func TestEventHub_Subscribe_Emit(t *testing.T) {
	hub := NewEventHub()

	var received []Event
	var mu sync.Mutex
	unsub := hub.Subscribe(func(data []byte) {
		var e Event
		json.Unmarshal(data, &e)
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})
	defer unsub()

	hub.Emit(Event{
		Type:       EventWorkflowStarted,
		TraceID:    "t1",
		WorkflowID: "wf1",
	})
	hub.Emit(Event{
		Type:       EventNodeEntered,
		TraceID:    "t1",
		WorkflowID: "wf1",
		NodeID:     "n1",
		NodeType:   "transform.set",
	})

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, 2)
	assert.Equal(t, EventWorkflowStarted, received[0].Type)
	assert.Equal(t, "t1", received[0].TraceID)
	assert.Equal(t, EventNodeEntered, received[1].Type)
	assert.Equal(t, "n1", received[1].NodeID)
}

func TestEventHub_Unsubscribe(t *testing.T) {
	hub := NewEventHub()

	count := 0
	unsub := hub.Subscribe(func(data []byte) {
		count++
	})

	hub.Emit(Event{Type: EventWorkflowStarted})
	assert.Equal(t, 1, count)

	unsub()
	hub.Emit(Event{Type: EventWorkflowCompleted})
	assert.Equal(t, 1, count) // no more events after unsub
}

func TestEventHub_MultipleSubscribers(t *testing.T) {
	hub := NewEventHub()

	var count1, count2 int
	unsub1 := hub.Subscribe(func(data []byte) { count1++ })
	unsub2 := hub.Subscribe(func(data []byte) { count2++ })
	defer unsub1()
	defer unsub2()

	hub.Emit(Event{Type: EventNodeCompleted})
	assert.Equal(t, 1, count1)
	assert.Equal(t, 1, count2)
	assert.Equal(t, 2, hub.SubscriberCount())
}

func TestEventHub_EmitSetsTimestamp(t *testing.T) {
	hub := NewEventHub()

	var received Event
	unsub := hub.Subscribe(func(data []byte) {
		json.Unmarshal(data, &received)
	})
	defer unsub()

	hub.Emit(Event{Type: EventWorkflowStarted})
	assert.NotEmpty(t, received.Timestamp)
}

func TestEventHub_SubscriberCount(t *testing.T) {
	hub := NewEventHub()
	assert.Equal(t, 0, hub.SubscriberCount())

	unsub1 := hub.Subscribe(func(data []byte) {})
	assert.Equal(t, 1, hub.SubscriberCount())

	unsub2 := hub.Subscribe(func(data []byte) {})
	assert.Equal(t, 2, hub.SubscriberCount())

	unsub1()
	assert.Equal(t, 1, hub.SubscriberCount())

	unsub2()
	assert.Equal(t, 0, hub.SubscriberCount())
}
