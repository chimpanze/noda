package response

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
)

func benchRespExecCtx() *engine.ExecutionContextImpl {
	compiler := expr.NewCompilerWithFunctions()
	ctx := engine.NewExecutionContext(
		engine.WithCompiler(compiler),
		engine.WithInput(map[string]any{
			"name":  "Alice",
			"email": "alice@example.com",
			"age":   30,
		}),
	)
	ctx.SetOutput("query", map[string]any{
		"rows": []any{
			map[string]any{"id": 1, "title": "Task 1"},
			map[string]any{"id": 2, "title": "Task 2"},
		},
	})
	return ctx
}

func BenchmarkResponseJSON_Static(b *testing.B) {
	exec := newJSONExecutor(nil)
	nCtx := benchRespExecCtx()
	config := map[string]any{
		"status": "200",
		"body": map[string]any{
			"message": "ok",
			"version": "1.0",
		},
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = exec.Execute(ctx, nCtx, config, nil)
	}
}

func BenchmarkResponseJSON_Expressions(b *testing.B) {
	exec := newJSONExecutor(nil)
	nCtx := benchRespExecCtx()
	config := map[string]any{
		"status": "200",
		"body": map[string]any{
			"name":  "{{ input.name }}",
			"email": "{{ input.email }}",
			"age":   "{{ input.age }}",
			"tasks": "{{ nodes.query.rows }}",
		},
	}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = exec.Execute(ctx, nCtx, config, nil)
	}
}
