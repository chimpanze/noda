package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Dispatch edge case tests ---

func TestDispatchNode_UnknownType(t *testing.T) {
	nodes, _, services := setupDispatchTest(t)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "step1",
		Type:    "nonexistent.type",
		Outputs: []string{"success", "error"},
	}

	_, err := dispatchNode(context.Background(), node, execCtx, services, nodes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
	assert.Contains(t, err.Error(), "nonexistent.type")
}

func TestDispatchNode_ServiceNotFound(t *testing.T) {
	nodes, _, services := setupDispatchTest(t)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:   "step1",
		Type: "mock.pass",
		Services: map[string]string{
			"db": "missing-service",
		},
		Outputs: []string{"success", "error"},
	}

	_, err := dispatchNode(context.Background(), node, execCtx, services, nodes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing-service")
	assert.Contains(t, err.Error(), "not found")
}

// mockUndeclaredOutputExecutor returns an output name not in its declared outputs.
type mockUndeclaredOutputExecutor struct{}

func (e *mockUndeclaredOutputExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *mockUndeclaredOutputExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return "bogus_output", map[string]any{"data": true}, nil
}

func TestDispatchNode_UndeclaredOutput(t *testing.T) {
	plugins := registry.NewPluginRegistry()
	nodeReg := registry.NewNodeRegistry()

	p := &testPlugin{
		name:   "test",
		prefix: "test",
		nodes: []api.NodeRegistration{
			{
				Descriptor: &testDescriptor{name: "undeclared"},
				Factory:    func(map[string]any) api.NodeExecutor { return &mockUndeclaredOutputExecutor{} },
			},
		},
	}
	require.NoError(t, plugins.Register(p))
	require.NoError(t, nodeReg.RegisterFromPlugin(p))

	svcReg := registry.NewServiceRegistry()
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "step1",
		Type:    "test.undeclared",
		Outputs: []string{"success", "error"},
	}

	_, err := dispatchNode(context.Background(), node, execCtx, svcReg, nodeReg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "undeclared output")
	assert.Contains(t, err.Error(), "bogus_output")
}

// mockEmptyOutputExecutor returns an empty output name (should default to "success").
type mockEmptyOutputExecutor struct{}

func (e *mockEmptyOutputExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *mockEmptyOutputExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return "", map[string]any{"ok": true}, nil
}

func TestDispatchNode_EmptyOutputDefaultsToSuccess(t *testing.T) {
	plugins := registry.NewPluginRegistry()
	nodeReg := registry.NewNodeRegistry()

	p := &testPlugin{
		name:   "test",
		prefix: "test",
		nodes: []api.NodeRegistration{
			{
				Descriptor: &testDescriptor{name: "empty"},
				Factory:    func(map[string]any) api.NodeExecutor { return &mockEmptyOutputExecutor{} },
			},
		},
	}
	require.NoError(t, plugins.Register(p))
	require.NoError(t, nodeReg.RegisterFromPlugin(p))

	svcReg := registry.NewServiceRegistry()
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "step1",
		Type:    "test.empty",
		Outputs: []string{"success", "error"},
	}

	output, err := dispatchNode(context.Background(), node, execCtx, svcReg, nodeReg)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

// --- ExecuteGraph edge case tests ---

func TestExecuteGraph_NoEntryNodes(t *testing.T) {
	graph := &CompiledGraph{
		WorkflowID: "empty",
		EntryNodes: nil,
	}

	execCtx := NewExecutionContext()
	err := ExecuteGraph(context.Background(), graph, execCtx, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no entry nodes")
}

func TestExecuteGraph_ErrorOutputNoErrorEdge(t *testing.T) {
	// A node produces "error" output but the graph has no error edges for it.
	// The workflow should fail with a specific message.
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"fail": &mockFail{},
		"next": &mockPass{},
	})

	wf := WorkflowConfig{
		ID: "error-no-edge",
		Nodes: map[string]NodeConfig{
			"fail": {Type: "mock.fail"},
			"next": {Type: "mock.next"},
		},
		Edges: []EdgeConfig{
			{From: "fail", To: "next"}, // success edge only
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no error edge")
}

func TestExecuteGraph_NodeFailCancelsParallelBranch(t *testing.T) {
	// When one branch fails, parallel branches should be cancelled via context.
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"fast_fail": &mockFail{},
		"slow":      &mockSlow{delay: 5 * time.Second},
	})

	// Use resolver without error output so failure propagates
	resolver := &singleOutputResolver{outputs: []string{"success"}}
	wf := WorkflowConfig{
		ID: "fail-cancels",
		Nodes: map[string]NodeConfig{
			"fast_fail": {Type: "mock.fast_fail"},
			"slow":      {Type: "mock.slow"},
		},
		Edges: []EdgeConfig{},
	}

	graph, err := Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	start := time.Now()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	elapsed := time.Since(start)

	require.Error(t, err)
	// The slow node should be cancelled quickly, not wait 5 seconds
	assert.Less(t, elapsed, 2*time.Second)
}

func TestExecuteGraph_ORJoinDispatchesOnce(t *testing.T) {
	// In an OR-join, the merge node should only execute once even if
	// multiple branches complete.
	var count atomic.Int32
	counterExec := &countingExecutor{count: &count}

	resolver := &mapResolver{
		types: map[string][]string{
			"mock.check": {"then", "else", "error"},
		},
		fallback: []string{"success", "error"},
	}

	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"check":    &mockConditional{},
		"branch_t": &mockPass{},
		"branch_f": &mockPass{},
		"merge":    counterExec,
	})

	wf := WorkflowConfig{
		ID: "or-join",
		Nodes: map[string]NodeConfig{
			"check":    {Type: "mock.check", Config: map[string]any{"condition": true}},
			"branch_t": {Type: "mock.branch_t"},
			"branch_f": {Type: "mock.branch_f"},
			"merge":    {Type: "mock.merge"},
		},
		Edges: []EdgeConfig{
			{From: "check", Output: "then", To: "branch_t"},
			{From: "check", Output: "else", To: "branch_f"},
			{From: "branch_t", To: "merge"},
			{From: "branch_f", To: "merge"},
		},
	}

	graph, err := Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	assert.Equal(t, int32(1), count.Load(), "OR-join merge should execute exactly once")
}

func TestExecuteGraph_TraceCallbackReceivesEvents(t *testing.T) {
	var mu sync.Mutex
	var events []string

	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"step": &mockPass{},
	})

	wf := WorkflowConfig{
		ID: "trace",
		Nodes: map[string]NodeConfig{
			"step": {Type: "mock.step"},
		},
		Edges: []EdgeConfig{},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext(
		WithTraceCallback(func(eventType, nodeID, nodeType, output, errMsg string, data any) {
			mu.Lock()
			events = append(events, eventType)
			mu.Unlock()
		}),
	)

	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	assert.Contains(t, events, "workflow:started")
	assert.Contains(t, events, "workflow:completed")
	assert.Contains(t, events, "node:entered")
	assert.Contains(t, events, "node:completed")
}

func TestExecuteGraph_TraceCallbackOnFailure(t *testing.T) {
	var mu sync.Mutex
	var events []string

	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"fail": &mockFail{},
	})

	resolver := &singleOutputResolver{outputs: []string{"success"}}
	wf := WorkflowConfig{
		ID: "trace-fail",
		Nodes: map[string]NodeConfig{
			"fail": {Type: "mock.fail"},
		},
		Edges: []EdgeConfig{},
	}

	graph, err := Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := NewExecutionContext(
		WithTraceCallback(func(eventType, nodeID, nodeType, output, errMsg string, data any) {
			mu.Lock()
			events = append(events, eventType)
			mu.Unlock()
		}),
	)

	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err)

	assert.Contains(t, events, "workflow:started")
	assert.Contains(t, events, "workflow:failed")
	assert.Contains(t, events, "node:failed")
}

// --- Retry edge case tests ---

func TestRetry_InvalidDelayDefaultsToOneSecond(t *testing.T) {
	executor := &failNTimesExecutor{failCount: 100}
	nodes, services := setupRetryTest(t, executor)
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "node",
		Type:    "rt.flaky",
		Outputs: []string{"success", "error"},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Invalid delay string should default to 1s, which means the context
	// will cancel before many retries complete
	output, err := retryNode(ctx, node, execCtx, services, nodes, &RetryConfig{
		Attempts: 100,
		Backoff:  "fixed",
		Delay:    "not-a-duration",
	})
	require.NoError(t, err)
	assert.Equal(t, "error", output)
}

// --- Context edge case tests ---

func TestExecutionContext_ResolveWithVars(t *testing.T) {
	ctx := NewExecutionContext(
		WithInput(map[string]any{"name": "Alice"}),
	)

	result, err := ctx.ResolveWithVars("{{ item }}", map[string]any{
		"item": "hello",
	})
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestExecutionContext_ResolveWithVarsOverridesContext(t *testing.T) {
	ctx := NewExecutionContext(
		WithInput(map[string]any{"name": "Alice"}),
	)

	// Extra vars should overlay on top of standard context
	result, err := ctx.ResolveWithVars("{{ input.name }}", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "Alice", result)
}

func TestExecutionContext_InterceptResponse_WithHTTPResponse(t *testing.T) {
	var intercepted *api.HTTPResponse
	ctx := NewExecutionContext()
	ctx.SetResponseInterceptor(func(resp *api.HTTPResponse) {
		intercepted = resp
	})

	resp := &api.HTTPResponse{
		Status: 200,
		Body:   "ok",
	}
	ctx.InterceptResponse(resp)

	require.NotNil(t, intercepted)
	assert.Equal(t, 200, intercepted.Status)
}

func TestExecutionContext_InterceptResponse_WithNonHTTPResponse(t *testing.T) {
	var intercepted *api.HTTPResponse
	ctx := NewExecutionContext()
	ctx.SetResponseInterceptor(func(resp *api.HTTPResponse) {
		intercepted = resp
	})

	// Non-HTTPResponse data should not trigger interceptor
	ctx.InterceptResponse(map[string]any{"data": "not a response"})
	assert.Nil(t, intercepted)
}

func TestExecutionContext_InterceptResponse_NoInterceptor(t *testing.T) {
	ctx := NewExecutionContext()
	// Should not panic when no interceptor is set
	ctx.InterceptResponse(&api.HTTPResponse{Status: 200})
}

func TestExecutionContext_BuildExprContext_WithAuth(t *testing.T) {
	ctx := NewExecutionContext(
		WithInput("test-input"),
		WithAuth(&api.AuthData{
			UserID: "user-42",
			Roles:  []string{"admin", "editor"},
			Claims: map[string]any{"org": "acme"},
		}),
	)

	result, err := ctx.Resolve("{{ auth.sub }}")
	require.NoError(t, err)
	assert.Equal(t, "user-42", result)
}

func TestExecutionContext_BuildExprContext_NoAuth(t *testing.T) {
	ctx := NewExecutionContext(
		WithInput("test-input"),
	)

	// Auth should be nil in context, resolving auth should not error but return nil
	result, err := ctx.Resolve("{{ auth }}")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestExecutionContext_GetOutput_NotFound(t *testing.T) {
	ctx := NewExecutionContext()
	_, ok := ctx.GetOutput("nonexistent")
	assert.False(t, ok)
}

func TestExecutionContext_SetCurrentNode_ClearsAfterEmpty(t *testing.T) {
	ctx := NewExecutionContext()
	ctx.SetCurrentNode("node-1")
	ctx.SetCurrentNode("")

	// After clearing, node context should be empty in logs
	// Just verify it doesn't panic
	ctx.Log("info", "test", nil)
}

// --- Compiler edge case tests ---

func TestExtractConfigRefs_WithExpressions(t *testing.T) {
	config := map[string]any{
		"value": "{{ nodes.query.result }}",
	}
	refs := extractConfigRefs(config)
	assert.True(t, refs["query"], "should extract 'query' from nodes.query.result")
}

func TestExtractConfigRefs_NoExpressions(t *testing.T) {
	config := map[string]any{
		"value": "plain string",
	}
	refs := extractConfigRefs(config)
	assert.Empty(t, refs)
}

func TestExtractConfigRefs_NilConfig(t *testing.T) {
	refs := extractConfigRefs(nil)
	assert.Empty(t, refs)
}

func TestExtractConfigRefs_NestedMap(t *testing.T) {
	config := map[string]any{
		"outer": map[string]any{
			"inner": "{{ nodes.step1.data }}",
		},
	}
	refs := extractConfigRefs(config)
	assert.True(t, refs["step1"])
}

func TestExtractConfigRefs_Array(t *testing.T) {
	config := map[string]any{
		"items": []any{
			"{{ nodes.a.val }}",
			"plain",
			map[string]any{
				"nested": "{{ nodes.b.val }}",
			},
		},
	}
	refs := extractConfigRefs(config)
	assert.True(t, refs["a"])
	assert.True(t, refs["b"])
}

func TestExtractIdentifiers_NodesPattern(t *testing.T) {
	idents := extractIdentifiers("nodes.query.result + nodes.other.value")
	assert.Contains(t, idents, "query")
	assert.Contains(t, idents, "other")
}

func TestExtractIdentifiers_TopLevelIdentifier(t *testing.T) {
	idents := extractIdentifiers("input.name")
	assert.Contains(t, idents, "input")
}

func TestExtractIdentifiers_EmptyExpression(t *testing.T) {
	idents := extractIdentifiers("")
	assert.Empty(t, idents)
}

func TestExtractIdentifiers_OperatorsOnly(t *testing.T) {
	idents := extractIdentifiers("+ * /")
	// No identifiers, only operators
	assert.Empty(t, idents)
}

func TestExtractIdentifiers_NumbersAreIdentChars(t *testing.T) {
	// Numbers are valid ident chars, so "123" is treated as an identifier
	idents := extractIdentifiers("123")
	assert.Len(t, idents, 1)
}

func TestWalkConfigStrings_AllTypes(t *testing.T) {
	var visited []string
	config := map[string]any{
		"str":    "hello",
		"num":    42,
		"nested": map[string]any{"key": "world"},
		"arr":    []any{"item1", 99, map[string]any{"k": "v"}},
	}
	walkConfigStrings(config, func(s string) {
		visited = append(visited, s)
	})
	assert.Contains(t, visited, "hello")
	assert.Contains(t, visited, "world")
	assert.Contains(t, visited, "item1")
	assert.Contains(t, visited, "v")
	assert.Len(t, visited, 4) // only strings
}

func TestContainsString(t *testing.T) {
	assert.True(t, containsString([]string{"a", "b", "c"}, "b"))
	assert.False(t, containsString([]string{"a", "b", "c"}, "d"))
	assert.False(t, containsString(nil, "a"))
	assert.False(t, containsString([]string{}, "a"))
}

func TestIsIdentChar(t *testing.T) {
	assert.True(t, isIdentChar('a'))
	assert.True(t, isIdentChar('Z'))
	assert.True(t, isIdentChar('0'))
	assert.True(t, isIdentChar('_'))
	assert.True(t, isIdentChar('-'))
	assert.False(t, isIdentChar(' '))
	assert.False(t, isIdentChar('.'))
	assert.False(t, isIdentChar('+'))
}

func TestCompile_ConfigAwareResolver(t *testing.T) {
	resolver := &configAwareTestResolver{}

	wf := WorkflowConfig{
		ID: "config-aware",
		Nodes: map[string]NodeConfig{
			"sw": {Type: "control.switch", Config: map[string]any{
				"cases": []any{"admin", "user"},
			}},
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "sw", Output: "admin", To: "a"},
			{From: "sw", Output: "user", To: "b"},
		},
	}

	graph, err := Compile(wf, resolver)
	require.NoError(t, err)
	assert.NotNil(t, graph.Nodes["sw"])
}

func TestCompile_GetEdge(t *testing.T) {
	wf := WorkflowConfig{
		ID: "get-edge",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	edge, ok := graph.GetEdge("a", "success", "b")
	assert.True(t, ok)
	assert.Equal(t, "a", edge.From)
	assert.Equal(t, "b", edge.To)

	_, ok = graph.GetEdge("a", "error", "b")
	assert.False(t, ok)
}

// --- Cache edge case tests ---

func TestWorkflowCache_GetMiss(t *testing.T) {
	cache, err := NewWorkflowCache(map[string]map[string]any{}, nil)
	require.NoError(t, err)

	_, ok := cache.Get("nonexistent")
	assert.False(t, ok)
}

func TestWorkflowCache_InvalidWorkflowFailsCreation(t *testing.T) {
	workflows := map[string]map[string]any{
		"bad": {
			"nodes": map[string]any{
				"a": map[string]any{"type": "mock.pass"},
			},
			"edges": []any{
				map[string]any{"from": "a", "to": "nonexistent"},
			},
		},
	}

	_, err := NewWorkflowCache(workflows, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compile")
}

func TestWorkflowCache_InvalidateError(t *testing.T) {
	cache, err := NewWorkflowCache(map[string]map[string]any{}, nil)
	require.NoError(t, err)

	err = cache.Invalidate(map[string]map[string]any{
		"bad": {
			"nodes": map[string]any{
				"a": map[string]any{"type": "mock.pass"},
			},
			"edges": []any{
				map[string]any{"from": "a", "to": "missing"},
			},
		},
	}, nil)
	require.Error(t, err)
}

func TestWorkflowCache_InvalidateSuccess(t *testing.T) {
	cache, err := NewWorkflowCache(map[string]map[string]any{
		"wf1": {
			"nodes": map[string]any{
				"a": map[string]any{"type": "mock.pass"},
			},
			"edges": []any{},
		},
	}, nil)
	require.NoError(t, err)

	_, ok := cache.Get("wf1")
	assert.True(t, ok)

	// Invalidate with a new set of workflows
	err = cache.Invalidate(map[string]map[string]any{
		"wf2": {
			"nodes": map[string]any{
				"b": map[string]any{"type": "mock.pass"},
			},
			"edges": []any{},
		},
	}, nil)
	require.NoError(t, err)

	_, ok = cache.Get("wf1")
	assert.False(t, ok, "wf1 should be gone after invalidation")

	_, ok = cache.Get("wf2")
	assert.True(t, ok, "wf2 should be available after invalidation")
}

func TestWorkflowCache_JSONIDIndex(t *testing.T) {
	cache, err := NewWorkflowCache(map[string]map[string]any{
		"file-key": {
			"id": "logical-id",
			"nodes": map[string]any{
				"a": map[string]any{"type": "mock.pass"},
			},
			"edges": []any{},
		},
	}, nil)
	require.NoError(t, err)

	// Should be accessible by both file key and logical ID
	_, ok := cache.Get("file-key")
	assert.True(t, ok)

	_, ok = cache.Get("logical-id")
	assert.True(t, ok)
}

// --- Parse edge case tests ---

func TestParseWorkflowFromMap_InvalidNodeFormat(t *testing.T) {
	raw := map[string]any{
		"nodes": map[string]any{
			"bad": "not-a-map",
		},
	}

	_, err := ParseWorkflowFromMap("test", raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

func TestParseWorkflowFromMap_InvalidRetryAttempts(t *testing.T) {
	raw := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "mock.pass"},
			"b": map[string]any{"type": "mock.pass"},
		},
		"edges": []any{
			map[string]any{
				"from":   "a",
				"to":     "b",
				"output": "error",
				"retry": map[string]any{
					"attempts": float64(0),
					"delay":    "1s",
				},
			},
		},
	}

	_, err := ParseWorkflowFromMap("test", raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attempts must be >= 1")
}

func TestParseWorkflowFromMap_MissingRetryDelay(t *testing.T) {
	raw := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "mock.pass"},
			"b": map[string]any{"type": "mock.pass"},
		},
		"edges": []any{
			map[string]any{
				"from":   "a",
				"to":     "b",
				"output": "error",
				"retry": map[string]any{
					"attempts": float64(3),
				},
			},
		},
	}

	_, err := ParseWorkflowFromMap("test", raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "retry delay is required")
}

func TestParseWorkflowFromMap_InvalidRetryDelay(t *testing.T) {
	raw := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "mock.pass"},
			"b": map[string]any{"type": "mock.pass"},
		},
		"edges": []any{
			map[string]any{
				"from":   "a",
				"to":     "b",
				"output": "error",
				"retry": map[string]any{
					"attempts": float64(3),
					"delay":    "not-a-duration",
				},
			},
		},
	}

	_, err := ParseWorkflowFromMap("test", raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid retry delay")
}

func TestParseWorkflowFromMap_InvalidRetryBackoff(t *testing.T) {
	raw := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "mock.pass"},
			"b": map[string]any{"type": "mock.pass"},
		},
		"edges": []any{
			map[string]any{
				"from":   "a",
				"to":     "b",
				"output": "error",
				"retry": map[string]any{
					"attempts": float64(3),
					"delay":    "1s",
					"backoff":  "random",
				},
			},
		},
	}

	_, err := ParseWorkflowFromMap("test", raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "backoff must be")
}

func TestParseWorkflowFromMap_ValidRetryDefaultBackoff(t *testing.T) {
	raw := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "mock.pass"},
			"b": map[string]any{"type": "mock.pass"},
		},
		"edges": []any{
			map[string]any{
				"from":   "a",
				"to":     "b",
				"output": "error",
				"retry": map[string]any{
					"attempts": float64(3),
					"delay":    "1s",
				},
			},
		},
	}

	wf, err := ParseWorkflowFromMap("test", raw)
	require.NoError(t, err)
	require.NotNil(t, wf.Edges[0].Retry)
	assert.Equal(t, "fixed", wf.Edges[0].Retry.Backoff)
}

func TestParseWorkflowFromMap_SkipsInvalidEdgeFormat(t *testing.T) {
	raw := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "mock.pass"},
		},
		"edges": []any{
			"not-a-map", // invalid edge format, should be skipped
		},
	}

	wf, err := ParseWorkflowFromMap("test", raw)
	require.NoError(t, err)
	assert.Empty(t, wf.Edges)
}

func TestParseWorkflowFromMap_NoEdgesKey(t *testing.T) {
	raw := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{"type": "mock.pass"},
		},
	}

	wf, err := ParseWorkflowFromMap("test", raw)
	require.NoError(t, err)
	assert.Empty(t, wf.Edges)
	assert.Len(t, wf.Nodes, 1)
}

func TestParseWorkflowFromMap_NodeWithServices(t *testing.T) {
	raw := map[string]any{
		"nodes": map[string]any{
			"a": map[string]any{
				"type": "db.query",
				"services": map[string]any{
					"db": "postgres-main",
				},
			},
		},
		"edges": []any{},
	}

	wf, err := ParseWorkflowFromMap("test", raw)
	require.NoError(t, err)
	assert.Equal(t, "postgres-main", wf.Nodes["a"].Services["db"])
}

func TestMapStrVal(t *testing.T) {
	m := map[string]any{
		"key":    "value",
		"number": 42,
	}
	assert.Equal(t, "value", MapStrVal(m, "key"))
	assert.Equal(t, "", MapStrVal(m, "number"))
	assert.Equal(t, "", MapStrVal(m, "missing"))
}

// --- Eviction edge case tests ---

func TestEviction_TerminalNodesNeverEvicted(t *testing.T) {
	wf := WorkflowConfig{
		ID: "terminal-evict",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{},
	}
	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	execCtx.SetOutput("a", "data-a")

	tracker := NewEvictionTracker(graph, execCtx)
	tracker.NodeCompleted("a")

	// Terminal node outputs should never be evicted
	_, ok := execCtx.GetOutput("a")
	assert.True(t, ok, "terminal node output should not be evicted")
}

func TestEviction_ExpressionRefTracking(t *testing.T) {
	// Node C references node A's output via expression, even though
	// there's no direct edge A→C. A should only be evicted after C completes.
	wf := WorkflowConfig{
		ID: "expr-ref-evict",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
			"b": {Type: "mock.pass", Config: map[string]any{
				"val": "{{ nodes.a.result }}",
			}},
			"c": {Type: "mock.pass", Config: map[string]any{
				"val": "{{ nodes.a.other }}",
			}},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"},
			{From: "b", To: "c"},
		},
	}
	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	execCtx.SetOutput("a", "data-a")

	tracker := NewEvictionTracker(graph, execCtx)

	// After B completes, A is still referenced by C
	tracker.NodeCompleted("b")
	_, ok := execCtx.GetOutput("a")
	assert.True(t, ok, "a should not be evicted — c still references it via expression")

	// After C completes, A can be evicted
	tracker.NodeCompleted("c")
	_, ok = execCtx.GetOutput("a")
	assert.False(t, ok, "a should be evicted after all consumers complete")
}

// --- Exclusivity edge case tests ---

func TestExclusivity_NoOutputNodes(t *testing.T) {
	wf := WorkflowConfig{
		ID: "no-outputs",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{},
	}
	graph := compileForExclusivity(t, wf)
	err := ValidateOutputExclusivity(graph)
	assert.NoError(t, err)
}

func TestExclusivity_OutputNodeWithEmptyName(t *testing.T) {
	wf := WorkflowConfig{
		ID: "empty-name",
		Nodes: map[string]NodeConfig{
			"out": {Type: "workflow.output", Config: map[string]any{}},
		},
		Edges: []EdgeConfig{},
	}
	graph := compileForExclusivity(t, wf)
	err := ValidateOutputExclusivity(graph)
	assert.NoError(t, err) // empty name nodes are skipped
}

// --- Helper types ---

type countingExecutor struct {
	count *atomic.Int32
}

func (e *countingExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *countingExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	e.count.Add(1)
	return "success", map[string]any{"counted": true}, nil
}

type configAwareTestResolver struct{}

func (r *configAwareTestResolver) OutputsForType(nodeType string) ([]string, bool) {
	return []string{"success", "error"}, true
}

func (r *configAwareTestResolver) OutputsForTypeWithConfig(nodeType string, config map[string]any) ([]string, bool) {
	if nodeType == "control.switch" {
		outputs := []string{"error"}
		if cases, ok := config["cases"].([]any); ok {
			for _, c := range cases {
				if s, ok := c.(string); ok {
					outputs = append(outputs, s)
				}
			}
		}
		return outputs, true
	}
	return []string{"success", "error"}, true
}

// --- Retry with dispatchNode error propagation ---

func TestRetry_DispatchErrorReturnsImmediately(t *testing.T) {
	// When dispatchNode returns a structural error (no error output edge),
	// retry should return immediately instead of retrying — these errors
	// (panics, unknown types, missing services) will never succeed on retry.
	plugins := registry.NewPluginRegistry()
	nodeReg := registry.NewNodeRegistry()

	callCount := &atomic.Int32{}
	p := &testPlugin{
		name:   "retry-err",
		prefix: "re",
		nodes: []api.NodeRegistration{
			{
				Descriptor: &testDescriptor{name: "hard-fail"},
				Factory: func(map[string]any) api.NodeExecutor {
					return &hardFailThenSucceed{callCount: callCount, failUntil: 2}
				},
			},
		},
	}
	require.NoError(t, plugins.Register(p))
	require.NoError(t, nodeReg.RegisterFromPlugin(p))

	svcReg := registry.NewServiceRegistry()
	execCtx := NewExecutionContext()

	node := &CompiledNode{
		ID:      "hard",
		Type:    "re.hard-fail",
		Outputs: []string{"success"}, // no error output, so dispatchNode returns error
	}

	_, err := retryNode(context.Background(), node, execCtx, svcReg, nodeReg, &RetryConfig{
		Attempts: 3,
		Backoff:  "fixed",
		Delay:    "1ms",
	})
	require.Error(t, err)
	assert.Equal(t, int32(1), callCount.Load(), "should only attempt once before returning the dispatch error")
}

type hardFailThenSucceed struct {
	callCount *atomic.Int32
	failUntil int
}

func (e *hardFailThenSucceed) Outputs() []string { return []string{"success", "error"} }
func (e *hardFailThenSucceed) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	n := int(e.callCount.Add(1))
	if n <= e.failUntil {
		return "", nil, fmt.Errorf("hard failure %d", n)
	}
	return "success", map[string]any{"ok": true}, nil
}

// --- Workflow execution with retry on error edges ---

func TestExecuteGraph_RetryExhaustedFollowsErrorEdge(t *testing.T) {
	mu := &sync.Mutex{}
	var order []string
	alwaysFail := &failNTimesExecutor{failCount: 1000}
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"flaky":   alwaysFail,
		"handler": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "handler"},
	})

	wf := WorkflowConfig{
		ID: "retry-exhaust",
		Nodes: map[string]NodeConfig{
			"flaky":   {Type: "mock.flaky"},
			"handler": {Type: "mock.handler"},
		},
		Edges: []EdgeConfig{
			{From: "flaky", Output: "error", To: "handler", Retry: &RetryConfig{
				Attempts: 2,
				Backoff:  "fixed",
				Delay:    "1ms",
			}},
		},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	// After all retries fail, error edge should be followed to handler
	assert.Contains(t, order, "handler")
}

func TestExecuteGraph_MultipleEntryNodes(t *testing.T) {
	mu := &sync.Mutex{}
	var order []string
	nodeReg, svcReg := setupIntegrationTest(t, map[string]api.NodeExecutor{
		"a": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "a"},
		"b": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "b"},
		"c": &orderTrackingExecutor{mu: mu, order: &order, nodeID: "c"},
	})

	wf := WorkflowConfig{
		ID: "multi-entry",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.a"},
			"b": {Type: "mock.b"},
			"c": {Type: "mock.c"},
		},
		Edges: []EdgeConfig{}, // all are entry nodes, no edges
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	execCtx := NewExecutionContext()
	err = ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	assert.Len(t, order, 3)
	assert.Contains(t, order, "a")
	assert.Contains(t, order, "b")
	assert.Contains(t, order, "c")
}

// --- Cache Invalidate with JSON ID ---

func TestWorkflowCache_InvalidateWithJSONID(t *testing.T) {
	cache, err := NewWorkflowCache(map[string]map[string]any{}, nil)
	require.NoError(t, err)

	err = cache.Invalidate(map[string]map[string]any{
		"file-key": {
			"id": "logical-id",
			"nodes": map[string]any{
				"a": map[string]any{"type": "mock.pass"},
			},
			"edges": []any{},
		},
	}, nil)
	require.NoError(t, err)

	_, ok := cache.Get("logical-id")
	assert.True(t, ok)
}

func TestWorkflowCache_InvalidParseFailsCreation(t *testing.T) {
	workflows := map[string]map[string]any{
		"bad": {
			"nodes": map[string]any{
				"a": "not-a-map", // invalid node format
			},
		},
	}

	_, err := NewWorkflowCache(workflows, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestWorkflowCache_InvalidateParseError(t *testing.T) {
	cache, err := NewWorkflowCache(map[string]map[string]any{}, nil)
	require.NoError(t, err)

	err = cache.Invalidate(map[string]map[string]any{
		"bad": {
			"nodes": map[string]any{
				"a": "not-a-map",
			},
		},
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}

func TestCompile_NilResolver(t *testing.T) {
	wf := WorkflowConfig{
		ID: "nil-resolver",
		Nodes: map[string]NodeConfig{
			"a": {Type: "mock.pass"},
		},
		Edges: []EdgeConfig{},
	}

	graph, err := Compile(wf, nil)
	require.NoError(t, err)
	// DefaultOutputResolver should be used
	assert.Contains(t, graph.Nodes["a"].Outputs, "success")
	assert.Contains(t, graph.Nodes["a"].Outputs, "error")
}
