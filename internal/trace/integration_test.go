package trace_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/trace"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOTelSpans_WorkflowExecution(t *testing.T) {
	// Set up in-memory span exporter
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	tracer := tp.Tracer("test")

	// Register a simple node
	nodeReg := registry.NewNodeRegistry()
	nodeReg.RegisterFactory("test.echo", newEchoExecutor)

	// Build a simple two-node workflow
	wfConfig := engine.WorkflowConfig{
		ID: "test-wf",
		Nodes: map[string]engine.NodeConfig{
			"step1": {Type: "test.echo", Config: map[string]any{"value": "hello"}},
			"step2": {Type: "test.echo", Config: map[string]any{"value": "world"}},
		},
		Edges: []engine.EdgeConfig{
			{From: "step1", To: "step2", Output: "success"},
		},
	}

	graph, err := engine.Compile(wfConfig, nodeReg)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(
		engine.WithTracer(tracer),
		engine.WithWorkflowID("test-wf"),
	)

	err = engine.ExecuteGraph(context.Background(), graph, execCtx, registry.NewServiceRegistry(), nodeReg)
	require.NoError(t, err)

	// Force flush
	tp.ForceFlush(context.Background())

	spans := exporter.GetSpans()
	require.GreaterOrEqual(t, len(spans), 3, "should have workflow + 2 node spans")

	// Check span names
	spanNames := make(map[string]bool)
	for _, s := range spans {
		spanNames[s.Name] = true
	}
	assert.True(t, spanNames["workflow:test-wf"], "should have workflow span")
	assert.True(t, spanNames["node:test.echo"], "should have node span")
}

func TestTraceCallback_WorkflowExecution(t *testing.T) {
	// Register a simple node
	nodeReg := registry.NewNodeRegistry()
	nodeReg.RegisterFactory("test.echo", newEchoExecutor)

	wfConfig := engine.WorkflowConfig{
		ID: "test-wf",
		Nodes: map[string]engine.NodeConfig{
			"step1": {Type: "test.echo", Config: map[string]any{"value": "hello"}},
		},
		Edges: []engine.EdgeConfig{},
	}

	graph, err := engine.Compile(wfConfig, nodeReg)
	require.NoError(t, err)

	var mu sync.Mutex
	var events []string
	execCtx := engine.NewExecutionContext(
		engine.WithWorkflowID("test-wf"),
		engine.WithTraceCallback(func(eventType, nodeID, nodeType, output, errMsg string, data any) {
			mu.Lock()
			events = append(events, eventType)
			mu.Unlock()
		}),
	)

	err = engine.ExecuteGraph(context.Background(), graph, execCtx, registry.NewServiceRegistry(), nodeReg)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, events, "workflow:started")
	assert.Contains(t, events, "node:entered")
	assert.Contains(t, events, "node:completed")
	assert.Contains(t, events, "workflow:completed")
}

func TestEventHub_WithTraceCallback(t *testing.T) {
	hub := trace.NewEventHub()

	var received []trace.Event
	var mu sync.Mutex
	unsub := hub.Subscribe(func(data []byte) {
		var e trace.Event
		json.Unmarshal(data, &e)
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})
	defer unsub()

	// Register a simple node
	nodeReg := registry.NewNodeRegistry()
	nodeReg.RegisterFactory("test.echo", newEchoExecutor)

	wfConfig := engine.WorkflowConfig{
		ID: "test-wf",
		Nodes: map[string]engine.NodeConfig{
			"step1": {Type: "test.echo", Config: map[string]any{"value": "hello"}},
		},
		Edges: []engine.EdgeConfig{},
	}

	graph, err := engine.Compile(wfConfig, nodeReg)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(
		engine.WithWorkflowID("test-wf"),
		engine.WithTraceCallback(func(eventType, nodeID, nodeType, output, errMsg string, data any) {
			hub.Emit(trace.Event{
				Type:       trace.EventType(eventType),
				WorkflowID: "test-wf",
				NodeID:     nodeID,
				NodeType:   nodeType,
				Output:     output,
				Error:      errMsg,
			})
		}),
	)

	err = engine.ExecuteGraph(context.Background(), graph, execCtx, registry.NewServiceRegistry(), nodeReg)
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.GreaterOrEqual(t, len(received), 3)

	// Check event types
	types := make([]trace.EventType, len(received))
	for i, e := range received {
		types[i] = e.Type
	}
	assert.Contains(t, types, trace.EventWorkflowStarted)
	assert.Contains(t, types, trace.EventNodeEntered)
	assert.Contains(t, types, trace.EventNodeCompleted)
	assert.Contains(t, types, trace.EventWorkflowCompleted)
}

// --- test node ---

type echoDescriptor struct{}

func (d *echoDescriptor) Name() string                          { return "echo" }
func (d *echoDescriptor) ServiceDeps() map[string]api.ServiceDep { return nil }
func (d *echoDescriptor) ConfigSchema() map[string]any           { return nil }

type echoExecutor struct{}

func newEchoExecutor(_ map[string]any) api.NodeExecutor { return &echoExecutor{} }

func (e *echoExecutor) Outputs() []string { return []string{"success"} }

func (e *echoExecutor) Execute(_ context.Context, _ api.ExecutionContext, config map[string]any, _ map[string]any) (string, any, error) {
	return "success", config, nil
}
