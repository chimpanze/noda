package control

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
)

func benchCtrlExecCtx(inputData map[string]any) *engine.ExecutionContextImpl {
	compiler := expr.NewCompilerWithFunctions()
	return engine.NewExecutionContext(
		engine.WithCompiler(compiler),
		engine.WithInput(inputData),
	)
}

func BenchmarkControlIf_TruePath(b *testing.B) {
	exec := newIfExecutor(nil)
	nCtx := benchCtrlExecCtx(map[string]any{"active": true})
	config := map[string]any{"condition": "{{ input.active }}"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = exec.Execute(ctx, nCtx, config, nil)
	}
}

func BenchmarkControlIf_FalsePath(b *testing.B) {
	exec := newIfExecutor(nil)
	nCtx := benchCtrlExecCtx(map[string]any{"active": false})
	config := map[string]any{"condition": "{{ input.active }}"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = exec.Execute(ctx, nCtx, config, nil)
	}
}

func BenchmarkControlSwitch_5Cases(b *testing.B) {
	exec := newSwitchExecutor(map[string]any{
		"cases": []any{"draft", "active", "paused", "completed", "archived"},
	})
	nCtx := benchCtrlExecCtx(map[string]any{"status": "completed"})
	config := map[string]any{"expression": "{{ input.status }}"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = exec.Execute(ctx, nCtx, config, nil)
	}
}

func BenchmarkControlSwitch_Default(b *testing.B) {
	exec := newSwitchExecutor(map[string]any{
		"cases": []any{"draft", "active", "paused"},
	})
	nCtx := benchCtrlExecCtx(map[string]any{"status": "unknown"})
	config := map[string]any{"expression": "{{ input.status }}"}
	ctx := context.Background()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _, _ = exec.Execute(ctx, nCtx, config, nil)
	}
}
