package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// noopExecutor returns immediately with no work.
type noopExecutor struct{}

func (e *noopExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *noopExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return "success", nil, nil
}

// benchOutputResolver returns standard outputs for any node type.
type benchOutputResolver struct{}

func (r *benchOutputResolver) OutputsForType(string) ([]string, bool) {
	return []string{"success", "error"}, true
}

func setupBenchRegistry(b *testing.B, nodeTypes []string) (*registry.NodeRegistry, *registry.ServiceRegistry) {
	b.Helper()
	plugins := registry.NewPluginRegistry()
	nodeReg := registry.NewNodeRegistry()

	var nodeRegs []api.NodeRegistration
	for _, name := range nodeTypes {
		nodeRegs = append(nodeRegs, api.NodeRegistration{
			Descriptor: &testDescriptor{name: name},
			Factory:    func(map[string]any) api.NodeExecutor { return &noopExecutor{} },
		})
	}

	p := &testPlugin{name: "test", prefix: "test", nodes: nodeRegs}
	if err := plugins.Register(p); err != nil {
		b.Fatal(err)
	}
	if err := nodeReg.RegisterFromPlugin(p); err != nil {
		b.Fatal(err)
	}
	return nodeReg, registry.NewServiceRegistry()
}

// --- Compilation benchmarks ---

func makeLinearWorkflow(n int) WorkflowConfig {
	nodes := make(map[string]NodeConfig, n)
	edges := make([]EdgeConfig, 0, n-1)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("n%d", i)
		nodes[id] = NodeConfig{Type: fmt.Sprintf("test.n%d", i)}
		if i > 0 {
			edges = append(edges, EdgeConfig{From: fmt.Sprintf("n%d", i-1), To: id})
		}
	}
	return WorkflowConfig{ID: "linear", Nodes: nodes, Edges: edges}
}

func makeParallelWorkflow(branches int) WorkflowConfig {
	nodes := map[string]NodeConfig{
		"start": {Type: "test.start"},
		"join":  {Type: "test.join"},
	}
	edges := make([]EdgeConfig, 0, branches*2)
	types := []string{"start", "join"}
	for i := 0; i < branches; i++ {
		id := fmt.Sprintf("b%d", i)
		nodes[id] = NodeConfig{Type: fmt.Sprintf("test.b%d", i)}
		types = append(types, fmt.Sprintf("b%d", i))
		edges = append(edges,
			EdgeConfig{From: "start", To: id},
			EdgeConfig{From: id, To: "join"},
		)
	}
	return WorkflowConfig{ID: "parallel", Nodes: nodes, Edges: edges}
}

func makeDiamondWorkflow() WorkflowConfig {
	return WorkflowConfig{
		ID: "diamond",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"},
			"b": {Type: "test.b"},
			"c": {Type: "test.c"},
			"d": {Type: "test.d"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "b"},
			{From: "a", To: "c"},
			{From: "b", To: "d"},
			{From: "c", To: "d"},
		},
	}
}

func BenchmarkCompile_Linear3(b *testing.B) {
	wf := makeLinearWorkflow(3)
	resolver := &benchOutputResolver{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Compile(wf, resolver)
	}
}

func BenchmarkCompile_Linear10(b *testing.B) {
	wf := makeLinearWorkflow(10)
	resolver := &benchOutputResolver{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Compile(wf, resolver)
	}
}

func BenchmarkCompile_Parallel4(b *testing.B) {
	wf := makeParallelWorkflow(4)
	resolver := &benchOutputResolver{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Compile(wf, resolver)
	}
}

func BenchmarkCompile_Diamond(b *testing.B) {
	wf := makeDiamondWorkflow()
	resolver := &benchOutputResolver{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Compile(wf, resolver)
	}
}

func BenchmarkCompile_RealWorkflow(b *testing.B) {
	// Load a real workflow from examples
	wfPath := filepath.Join("..", "..", "examples", "saas-backend", "workflows", "create-task.json")
	data, err := os.ReadFile(wfPath)
	if err != nil {
		b.Skipf("example workflow not found: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		b.Fatal(err)
	}
	wf, err := ParseWorkflowFromMap("create-task", raw)
	if err != nil {
		b.Fatal(err)
	}
	resolver := &benchOutputResolver{}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Compile(wf, resolver)
	}
}

// --- Execution benchmarks ---

func benchExec(b *testing.B, wf WorkflowConfig, nodeTypes []string) {
	resolver := &benchOutputResolver{}
	graph, err := Compile(wf, resolver)
	if err != nil {
		b.Fatal(err)
	}
	nodeReg, svcReg := setupBenchRegistry(b, nodeTypes)
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		execCtx := NewExecutionContext(WithWorkflowID(wf.ID), WithLogger(discardLogger))
		_ = ExecuteGraph(ctx, graph, execCtx, svcReg, nodeReg)
	}
}

func nodeTypesForLinear(n int) []string {
	types := make([]string, n)
	for i := 0; i < n; i++ {
		types[i] = fmt.Sprintf("n%d", i)
	}
	return types
}

func nodeTypesForParallel(branches int) []string {
	types := []string{"start", "join"}
	for i := 0; i < branches; i++ {
		types = append(types, fmt.Sprintf("b%d", i))
	}
	return types
}

func BenchmarkExecute_Linear3(b *testing.B) {
	benchExec(b, makeLinearWorkflow(3), nodeTypesForLinear(3))
}

func BenchmarkExecute_Linear10(b *testing.B) {
	benchExec(b, makeLinearWorkflow(10), nodeTypesForLinear(10))
}

func BenchmarkExecute_Parallel4(b *testing.B) {
	benchExec(b, makeParallelWorkflow(4), nodeTypesForParallel(4))
}

func BenchmarkExecute_Diamond(b *testing.B) {
	benchExec(b, makeDiamondWorkflow(), []string{"a", "b", "c", "d"})
}

func BenchmarkExecute_SingleNode(b *testing.B) {
	wf := WorkflowConfig{
		ID:    "single",
		Nodes: map[string]NodeConfig{"n0": {Type: "test.n0"}},
		Edges: []EdgeConfig{},
	}
	benchExec(b, wf, []string{"n0"})
}

func BenchmarkExecute_Parallel_Concurrent(b *testing.B) {
	wf := makeParallelWorkflow(4)
	resolver := &benchOutputResolver{}
	graph, err := Compile(wf, resolver)
	if err != nil {
		b.Fatal(err)
	}
	nodeReg, svcReg := setupBenchRegistry(b, nodeTypesForParallel(4))
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			execCtx := NewExecutionContext(WithWorkflowID("parallel"), WithLogger(discardLogger))
			_ = ExecuteGraph(ctx, graph, execCtx, svcReg, nodeReg)
		}
	})
}

// --- Eviction tracker benchmarks ---

func BenchmarkEvictionTracker_10Nodes(b *testing.B) {
	wf := makeLinearWorkflow(10)
	resolver := &benchOutputResolver{}
	graph, _ := Compile(wf, resolver)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		execCtx := NewExecutionContext(WithWorkflowID("linear"), WithLogger(discardLogger))
		_ = NewEvictionTracker(graph, execCtx)
	}
}

func BenchmarkEvictionTracker_50Nodes(b *testing.B) {
	wf := makeLinearWorkflow(50)
	resolver := &benchOutputResolver{}
	graph, _ := Compile(wf, resolver)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		execCtx := NewExecutionContext(WithWorkflowID("linear"), WithLogger(discardLogger))
		_ = NewEvictionTracker(graph, execCtx)
	}
}

// --- Workflow parsing benchmarks ---

func BenchmarkParseWorkflowFromMap(b *testing.B) {
	raw := map[string]any{
		"nodes": map[string]any{
			"validate": map[string]any{"type": "transform.set", "config": map[string]any{"fields": map[string]any{"name": "{{ input.name }}"}}},
			"save":     map[string]any{"type": "db.insert", "config": map[string]any{"table": "tasks"}},
			"respond":  map[string]any{"type": "response.json", "config": map[string]any{"status": "200", "body": "{{ nodes.save }}"}},
		},
		"edges": []any{
			map[string]any{"from": "validate", "to": "save"},
			map[string]any{"from": "save", "to": "respond"},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = ParseWorkflowFromMap("test-wf", raw)
	}
}

// --- Workflow cache benchmarks ---

func BenchmarkWorkflowCache_Get(b *testing.B) {
	workflows := map[string]map[string]any{
		"wf1": {
			"nodes": map[string]any{
				"a": map[string]any{"type": "transform.set"},
				"b": map[string]any{"type": "response.json"},
			},
			"edges": []any{
				map[string]any{"from": "a", "to": "b"},
			},
		},
	}
	cache, err := NewWorkflowCache(workflows, &benchOutputResolver{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		cache.Get("wf1")
	}
}

func BenchmarkWorkflowCache_Get_Parallel(b *testing.B) {
	workflows := map[string]map[string]any{
		"wf1": {
			"nodes": map[string]any{
				"a": map[string]any{"type": "transform.set"},
				"b": map[string]any{"type": "response.json"},
			},
			"edges": []any{
				map[string]any{"from": "a", "to": "b"},
			},
		},
	}
	cache, err := NewWorkflowCache(workflows, &benchOutputResolver{})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cache.Get("wf1")
		}
	})
}

// --- Expression resolution in execution context ---

func BenchmarkExecContext_Resolve(b *testing.B) {
	compiler := expr.NewCompiler()
	execCtx := NewExecutionContext(
		WithCompiler(compiler),
		WithLogger(discardLogger),
		WithInput(map[string]any{"name": "Alice", "age": 30}),
	)
	execCtx.SetOutput("validate", map[string]any{"valid": true})

	// Warm cache
	_, _ = execCtx.Resolve("{{ input.name }}")

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = execCtx.Resolve("{{ input.name }}")
	}
}

func BenchmarkExecContext_SetOutput_Concurrent(b *testing.B) {
	compiler := expr.NewCompiler()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		execCtx := NewExecutionContext(WithCompiler(compiler), WithLogger(discardLogger))
		i := 0
		for pb.Next() {
			execCtx.SetOutput(fmt.Sprintf("node_%d", i%100), map[string]any{"val": i})
			i++
		}
	})
}

// --- testPlugin and testDescriptor for benchmarks (reuse from executor_test.go) ---
// These are already defined in executor_test.go and dispatch_test.go within
// the same package, so they are available here.

// Ensure noopExecutor satisfies the interface.
var _ api.NodeExecutor = &noopExecutor{}

func BenchmarkExecute_LargePayload(b *testing.B) {
	// Build a ~10MB input payload with many keys.
	const numKeys = 10000
	payload := make(map[string]any, numKeys)
	// Each value is ~1KB string to reach ~10MB total.
	value := string(make([]byte, 1024))
	for i := 0; i < numKeys; i++ {
		payload[fmt.Sprintf("field_%d", i)] = value
	}

	wf := makeLinearWorkflow(3)
	resolver := &benchOutputResolver{}
	graph, err := Compile(wf, resolver)
	if err != nil {
		b.Fatal(err)
	}
	nodeReg, svcReg := setupBenchRegistry(b, nodeTypesForLinear(3))
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		execCtx := NewExecutionContext(
			WithWorkflowID("linear"),
			WithLogger(discardLogger),
			WithInput(payload),
		)
		_ = ExecuteGraph(ctx, graph, execCtx, svcReg, nodeReg)
	}
}

func BenchmarkResolve(b *testing.B) {
	compiler := expr.NewCompilerWithFunctions()
	ctx := NewExecutionContext(
		WithInput(map[string]any{"name": "test", "count": 42}),
		WithCompiler(compiler),
		WithLogger(discardLogger),
	)
	ctx.SetOutput("step1", map[string]any{"result": "ok"})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ctx.Resolve("{{ input.name }}")
	}
}
