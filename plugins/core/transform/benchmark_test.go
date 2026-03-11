package transform

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
)

func benchExecCtx() *engine.ExecutionContextImpl {
	compiler := expr.NewCompilerWithFunctions()
	ctx := engine.NewExecutionContext(
		engine.WithCompiler(compiler),
		engine.WithInput(map[string]any{
			"name":  "Alice",
			"email": "alice@example.com",
			"age":   30,
			"role":  "admin",
		}),
	)
	ctx.SetOutput("prev", map[string]any{
		"items": []any{
			map[string]any{"id": 1, "name": "Item 1", "price": 10.0},
			map[string]any{"id": 2, "name": "Item 2", "price": 20.0},
			map[string]any{"id": 3, "name": "Item 3", "price": 30.0},
		},
	})
	return ctx
}

func bench100Items() *engine.ExecutionContextImpl {
	compiler := expr.NewCompilerWithFunctions()
	items := make([]any, 100)
	for i := range items {
		items[i] = map[string]any{"id": i, "name": "item", "value": i * 10}
	}
	ctx := engine.NewExecutionContext(
		engine.WithCompiler(compiler),
		engine.WithInput(map[string]any{"items": items}),
	)
	return ctx
}

func BenchmarkTransformSet_3Fields(b *testing.B) {
	exec := newSetExecutor(nil)
	nCtx := benchExecCtx()
	config := map[string]any{
		"fields": map[string]any{
			"name":  "{{ input.name }}",
			"email": "{{ input.email }}",
			"role":  "{{ input.role }}",
		},
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = exec.Execute(ctx, nCtx, config, nil)
	}
}

func BenchmarkTransformSet_10Fields(b *testing.B) {
	exec := newSetExecutor(nil)
	nCtx := benchExecCtx()
	config := map[string]any{
		"fields": map[string]any{
			"f0": "{{ input.name }}",
			"f1": "{{ input.email }}",
			"f2": "{{ input.role }}",
			"f3": "{{ input.age }}",
			"f4": "{{ input.name }}",
			"f5": "{{ input.email }}",
			"f6": "{{ input.role }}",
			"f7": "{{ input.age }}",
			"f8": "Hello {{ input.name }}",
			"f9": "{{ input.age + 1 }}",
		},
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = exec.Execute(ctx, nCtx, config, nil)
	}
}

func BenchmarkTransformMap_100Items(b *testing.B) {
	exec := newMapExecutor(nil)
	nCtx := bench100Items()
	config := map[string]any{
		"collection": "{{ input.items }}",
		"expression": "{{ $item.value * 2 }}",
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = exec.Execute(ctx, nCtx, config, nil)
	}
}

// Verify test helper types satisfy interfaces.
var _ api.ExecutionContext = benchExecCtx()
