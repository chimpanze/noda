package expr

import (
	"fmt"
	"testing"
)

// Realistic context matching what workflows produce at runtime.
func benchContext() map[string]any {
	return map[string]any{
		"input": map[string]any{
			"name":    "Alice",
			"email":   "alice@example.com",
			"age":     30,
			"role":    "admin",
			"company": "Acme Inc",
		},
		"request": map[string]any{
			"method":  "POST",
			"path":    "/api/tasks",
			"headers": map[string]any{"content-type": "application/json"},
		},
		"nodes": map[string]any{
			"validate": map[string]any{
				"data": map[string]any{
					"user": map[string]any{
						"id":    "usr_123",
						"email": "alice@example.com",
						"plan":  "pro",
					},
				},
			},
			"fetch_user": map[string]any{
				"data": map[string]any{
					"id":         42,
					"name":       "Alice",
					"workspace":  "ws_456",
					"created_at": "2025-01-01T00:00:00Z",
				},
			},
			"check_perms": map[string]any{
				"allowed": true,
				"role":    "editor",
			},
		},
		"auth": map[string]any{
			"user_id": "usr_123",
			"roles":   []any{"admin", "editor"},
		},
	}
}

// --- Parse benchmarks ---

func BenchmarkParse_Literal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Parse("hello world")
	}
}

func BenchmarkParse_Simple(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Parse("{{ input.name }}")
	}
}

func BenchmarkParse_Interpolated(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Parse("Hello {{ input.name }}, your email is {{ input.email }} and role is {{ input.role }}")
	}
}

func BenchmarkParse_NestedBraces(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = Parse(`{{ {"key": input.name} }}`)
	}
}

// --- Compile benchmarks ---

func BenchmarkCompile_SimpleAccess(b *testing.B) {
	c := NewCompiler()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.cache = make(map[string]*CompiledExpression) // clear cache each iteration
		_, _ = c.Compile("{{ input.name }}")
	}
}

func BenchmarkCompile_Arithmetic(b *testing.B) {
	c := NewCompiler()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.cache = make(map[string]*CompiledExpression)
		_, _ = c.Compile("{{ input.age + 10 }}")
	}
}

func BenchmarkCompile_FunctionCall(b *testing.B) {
	c := NewCompilerWithFunctions()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.cache = make(map[string]*CompiledExpression)
		_, _ = c.Compile("{{ upper(input.name) }}")
	}
}

func BenchmarkCompile_CacheHit(b *testing.B) {
	c := NewCompiler()
	_, _ = c.Compile("{{ input.name }}")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = c.Compile("{{ input.name }}")
	}
}

// --- Evaluate benchmarks ---

func BenchmarkEvaluate_FlatAccess(b *testing.B) {
	c := NewCompiler()
	compiled, _ := c.Compile("{{ input.name }}")
	ctx := benchContext()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Evaluate(compiled, ctx)
	}
}

func BenchmarkEvaluate_DeepNesting(b *testing.B) {
	c := NewCompiler()
	compiled, _ := c.Compile("{{ nodes.validate.data.user.email }}")
	ctx := benchContext()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Evaluate(compiled, ctx)
	}
}

func BenchmarkEvaluate_Arithmetic(b *testing.B) {
	c := NewCompiler()
	compiled, _ := c.Compile("{{ input.age * 2 + 10 }}")
	ctx := benchContext()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Evaluate(compiled, ctx)
	}
}

func BenchmarkEvaluate_Interpolated(b *testing.B) {
	c := NewCompiler()
	compiled, _ := c.Compile("Hello {{ input.name }}, you are {{ input.role }}")
	ctx := benchContext()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Evaluate(compiled, ctx)
	}
}

// --- Resolve end-to-end benchmarks ---

func BenchmarkResolve_Cached(b *testing.B) {
	c := NewCompiler()
	ctx := benchContext()
	r := NewResolver(c, ctx)
	// Warm the cache
	_, _ = r.Resolve("{{ input.name }}")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = r.Resolve("{{ input.name }}")
	}
}

func BenchmarkResolveMap_5Fields(b *testing.B) {
	c := NewCompiler()
	ctx := benchContext()
	r := NewResolver(c, ctx)
	config := map[string]any{
		"name":  "{{ input.name }}",
		"email": "{{ input.email }}",
		"role":  "{{ input.role }}",
		"id":    "{{ nodes.fetch_user.data.id }}",
		"plan":  "{{ nodes.validate.data.user.plan }}",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = r.ResolveMap(config)
	}
}

func BenchmarkResolveMap_20Fields(b *testing.B) {
	c := NewCompiler()
	ctx := benchContext()
	r := NewResolver(c, ctx)
	config := make(map[string]any, 20)
	for i := 0; i < 20; i++ {
		switch i % 5 {
		case 0:
			config[fmt.Sprintf("field_%d", i)] = "{{ input.name }}"
		case 1:
			config[fmt.Sprintf("field_%d", i)] = "{{ input.email }}"
		case 2:
			config[fmt.Sprintf("field_%d", i)] = "{{ nodes.fetch_user.data.id }}"
		case 3:
			config[fmt.Sprintf("field_%d", i)] = "{{ input.age + 1 }}"
		case 4:
			config[fmt.Sprintf("field_%d", i)] = "Hello {{ input.name }}"
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = r.ResolveMap(config)
	}
}
