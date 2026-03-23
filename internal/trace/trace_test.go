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
		_ = json.Unmarshal(data, &e)
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
	assert.Equal(t, 2, hub.subscriberCount())
}

func TestEventHub_EmitSetsTimestamp(t *testing.T) {
	hub := NewEventHub()

	var received Event
	unsub := hub.Subscribe(func(data []byte) {
		_ = json.Unmarshal(data, &received)
	})
	defer unsub()

	hub.Emit(Event{Type: EventWorkflowStarted})
	assert.NotEmpty(t, received.Timestamp)
}

func TestEventHub_SubscriberCount(t *testing.T) {
	hub := NewEventHub()
	assert.Equal(t, 0, hub.subscriberCount())

	unsub1 := hub.Subscribe(func(data []byte) {})
	assert.Equal(t, 1, hub.subscriberCount())

	unsub2 := hub.Subscribe(func(data []byte) {})
	assert.Equal(t, 2, hub.subscriberCount())

	unsub1()
	assert.Equal(t, 1, hub.subscriberCount())

	unsub2()
	assert.Equal(t, 0, hub.subscriberCount())
}

func TestParseConfig_NoTracing(t *testing.T) {
	// observability key present but no tracing sub-key
	cfg := ParseConfig(map[string]any{
		"observability": map[string]any{
			"metrics": map[string]any{"enabled": true},
		},
	})
	assert.False(t, cfg.Enabled)
	assert.Empty(t, cfg.Exporter)
}

func TestParseConfig_PartialFields(t *testing.T) {
	// Only some fields set
	cfg := ParseConfig(map[string]any{
		"observability": map[string]any{
			"tracing": map[string]any{
				"enabled": true,
			},
		},
	})
	assert.True(t, cfg.Enabled)
	assert.Empty(t, cfg.Exporter)
	assert.Empty(t, cfg.Endpoint)
	assert.False(t, cfg.Insecure)
}

func TestParseConfig_WrongTypes(t *testing.T) {
	// Wrong types for fields — should be silently ignored
	cfg := ParseConfig(map[string]any{
		"observability": map[string]any{
			"tracing": map[string]any{
				"enabled":  "yes",  // string instead of bool
				"exporter": 42,     // int instead of string
				"endpoint": true,   // bool instead of string
				"insecure": "true", // string instead of bool
			},
		},
	})
	assert.False(t, cfg.Enabled)
	assert.Empty(t, cfg.Exporter)
	assert.Empty(t, cfg.Endpoint)
	assert.False(t, cfg.Insecure)
}

func TestEventHub_EmitPreservesExistingTimestamp(t *testing.T) {
	hub := NewEventHub()

	var received Event
	unsub := hub.Subscribe(func(data []byte) {
		_ = json.Unmarshal(data, &received)
	})
	defer unsub()

	hub.Emit(Event{
		Type:      EventWorkflowStarted,
		Timestamp: "2025-01-01T00:00:00Z",
	})
	assert.Equal(t, "2025-01-01T00:00:00Z", received.Timestamp)
}

func TestEventHub_EmitNoSubscribers(t *testing.T) {
	hub := NewEventHub()
	// Should not panic
	hub.Emit(Event{Type: EventWorkflowStarted, TraceID: "t1"})
}

func TestEventHub_EmitAllFields(t *testing.T) {
	hub := NewEventHub()

	var received Event
	unsub := hub.Subscribe(func(data []byte) {
		_ = json.Unmarshal(data, &received)
	})
	defer unsub()

	hub.Emit(Event{
		Type:       EventNodeCompleted,
		TraceID:    "t1",
		WorkflowID: "wf1",
		NodeID:     "n1",
		NodeType:   "transform.set",
		Output:     "success",
		Duration:   "15ms",
		FromNode:   "n0",
		ToNode:     "n1",
		Data:       map[string]any{"key": "val"},
	})

	assert.Equal(t, EventNodeCompleted, received.Type)
	assert.Equal(t, "t1", received.TraceID)
	assert.Equal(t, "wf1", received.WorkflowID)
	assert.Equal(t, "n1", received.NodeID)
	assert.Equal(t, "transform.set", received.NodeType)
	assert.Equal(t, "success", received.Output)
	assert.Equal(t, "15ms", received.Duration)
	assert.Equal(t, "n0", received.FromNode)
	assert.Equal(t, "n1", received.ToNode)
	assert.NotEmpty(t, received.Timestamp)
}

func TestEventHub_ConcurrentEmitAndSubscribe(t *testing.T) {
	hub := NewEventHub()

	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0

	unsub := hub.Subscribe(func(data []byte) {
		mu.Lock()
		count++
		mu.Unlock()
	})
	defer unsub()

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.Emit(Event{Type: EventNodeEntered})
		}()
	}
	wg.Wait()

	mu.Lock()
	assert.Equal(t, 50, count)
	mu.Unlock()
}

func TestNewProvider_OTLP_WithEndpoint(t *testing.T) {
	// Test OTLP with endpoint but not insecure
	p, err := NewProvider(context.Background(), TracerConfig{
		Enabled:  true,
		Exporter: "otlp",
		Endpoint: "localhost:14318",
	}, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.NotNil(t, p.Tracer())
	assert.NoError(t, p.Shutdown(context.Background()))
}

func TestParseConfig_SamplingRate(t *testing.T) {
	cfg := ParseConfig(map[string]any{
		"observability": map[string]any{
			"tracing": map[string]any{
				"enabled":       true,
				"exporter":      "otlp",
				"sampling_rate": 0.5,
			},
		},
	})
	assert.True(t, cfg.Enabled)
	require.NotNil(t, cfg.SamplingRate)
	assert.Equal(t, 0.5, *cfg.SamplingRate)
}

func TestParseConfig_SamplingRate_NotSet(t *testing.T) {
	cfg := ParseConfig(map[string]any{
		"observability": map[string]any{
			"tracing": map[string]any{
				"enabled": true,
			},
		},
	})
	assert.Nil(t, cfg.SamplingRate)
}

func TestParseConfig_SamplingRate_WrongType(t *testing.T) {
	cfg := ParseConfig(map[string]any{
		"observability": map[string]any{
			"tracing": map[string]any{
				"enabled":       true,
				"sampling_rate": "half",
			},
		},
	})
	assert.Nil(t, cfg.SamplingRate)
}

func TestNewProvider_WithSamplingRate(t *testing.T) {
	rate := 0.25
	p, err := NewProvider(context.Background(), TracerConfig{
		Enabled:      true,
		SamplingRate: &rate,
	}, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.NotNil(t, p.Tracer())
	assert.NoError(t, p.Shutdown(context.Background()))
}

func TestNewProvider_OTLP_NoEndpoint(t *testing.T) {
	// Test OTLP without endpoint (uses default)
	p, err := NewProvider(context.Background(), TracerConfig{
		Enabled:  true,
		Exporter: "otlp",
	}, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.NoError(t, p.Shutdown(context.Background()))
}
