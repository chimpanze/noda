package config

import (
	"os"
	"path/filepath"
	"testing"
)

func saasBackendPath(b *testing.B) string {
	b.Helper()
	path := filepath.Join("..", "..", "examples", "saas-backend")
	if _, err := os.Stat(filepath.Join(path, "noda.json")); err != nil {
		b.Skipf("saas-backend example not found: %v", err)
	}
	return path
}

func BenchmarkValidateAll_SaaSBackend(b *testing.B) {
	path := saasBackendPath(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = ValidateAll(path, "development")
	}
}

func BenchmarkDiscover_SaaSBackend(b *testing.B) {
	path := saasBackendPath(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = Discover(path, "development")
	}
}

func BenchmarkLoadAll_SaaSBackend(b *testing.B) {
	path := saasBackendPath(b)
	discovered, err := Discover(path, "development")
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = LoadAll(discovered)
	}
}

func BenchmarkMergeOverlay(b *testing.B) {
	base := map[string]any{
		"server": map[string]any{
			"port": float64(3000),
			"host": "0.0.0.0",
		},
		"services": map[string]any{
			"db": map[string]any{
				"host":     "localhost",
				"port":     float64(5432),
				"database": "app",
			},
			"cache": map[string]any{
				"host": "localhost",
				"port": float64(6379),
			},
		},
	}
	overlay := map[string]any{
		"server": map[string]any{
			"port": float64(8080),
		},
		"services": map[string]any{
			"db": map[string]any{
				"host": "db.production.internal",
			},
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = MergeOverlay(base, overlay)
	}
}

func BenchmarkResolveEnvVars(b *testing.B) {
	os.Setenv("BENCH_DB_HOST", "localhost")
	os.Setenv("BENCH_DB_PORT", "5432")
	os.Setenv("BENCH_REDIS_URL", "redis://localhost:6379")
	defer func() {
		os.Unsetenv("BENCH_DB_HOST")
		os.Unsetenv("BENCH_DB_PORT")
		os.Unsetenv("BENCH_REDIS_URL")
	}()

	config := map[string]any{
		"services": map[string]any{
			"db": map[string]any{
				"host": "{{ $env('BENCH_DB_HOST') }}",
				"port": "{{ $env('BENCH_DB_PORT') }}",
			},
			"cache": map[string]any{
				"url": "{{ $env('BENCH_REDIS_URL') }}",
			},
		},
		"server": map[string]any{
			"port": float64(3000),
		},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, _ = ResolveEnvVars(config)
	}
}

func BenchmarkResolveRefs_SaaSBackend(b *testing.B) {
	path := saasBackendPath(b)
	discovered, err := Discover(path, "development")
	if err != nil {
		b.Fatal(err)
	}
	raw, errs := LoadAll(discovered)
	if len(errs) > 0 {
		b.Fatal(errs)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		// Make a copy to avoid mutation
		rc := &RawConfig{
			Root:        deepCopyMap(raw.Root),
			Schemas:     deepCopyMapMap(raw.Schemas),
			Routes:      deepCopyMapMap(raw.Routes),
			Workflows:   deepCopyMapMap(raw.Workflows),
			Workers:     deepCopyMapMap(raw.Workers),
			Schedules:   deepCopyMapMap(raw.Schedules),
			Connections: deepCopyMapMap(raw.Connections),
			Tests:       deepCopyMapMap(raw.Tests),
		}
		_ = ResolveRefs(rc)
	}
}

func BenchmarkValidate_SaaSBackend(b *testing.B) {
	path := saasBackendPath(b)
	discovered, err := Discover(path, "development")
	if err != nil {
		b.Fatal(err)
	}
	raw, errs := LoadAll(discovered)
	if len(errs) > 0 {
		b.Fatal(errs)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Validate(raw)
	}
}

func BenchmarkValidateCrossRefs_SaaSBackend(b *testing.B) {
	path := saasBackendPath(b)
	discovered, err := Discover(path, "development")
	if err != nil {
		b.Fatal(err)
	}
	raw, errs := LoadAll(discovered)
	if len(errs) > 0 {
		b.Fatal(errs)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = ValidateCrossRefs(raw)
	}
}

// Helpers for deep copying to avoid benchmark pollution from mutation.
func deepCopyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		switch val := v.(type) {
		case map[string]any:
			result[k] = deepCopyMap(val)
		case []any:
			cp := make([]any, len(val))
			copy(cp, val)
			result[k] = cp
		default:
			result[k] = v
		}
	}
	return result
}

func deepCopyMapMap(m map[string]map[string]any) map[string]map[string]any {
	if m == nil {
		return nil
	}
	result := make(map[string]map[string]any, len(m))
	for k, v := range m {
		result[k] = deepCopyMap(v)
	}
	return result
}
