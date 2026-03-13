package devmode

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Watcher tests ---

func TestWatcher_DetectsFileChange(t *testing.T) {
	dir := t.TempDir()

	var called atomic.Int32
	var lastPath string
	var mu sync.Mutex

	w, err := NewWatcher(func(path string) {
		mu.Lock()
		lastPath = path
		mu.Unlock()
		called.Add(1)
	}, slog.Default())
	require.NoError(t, err)

	// Reduce debounce for faster tests
	w.debounce = 50 * time.Millisecond

	require.NoError(t, w.WatchDir(dir))
	w.Start()
	defer w.Stop()

	// Write a JSON file
	testFile := filepath.Join(dir, "test.json")
	require.NoError(t, os.WriteFile(testFile, []byte(`{"hello":"world"}`), 0644))

	// Wait for debounce + processing
	time.Sleep(200 * time.Millisecond)

	assert.GreaterOrEqual(t, called.Load(), int32(1))
	mu.Lock()
	assert.Equal(t, testFile, lastPath)
	mu.Unlock()
}

func TestWatcher_IgnoresNonJSON(t *testing.T) {
	dir := t.TempDir()

	var called atomic.Int32
	w, err := NewWatcher(func(_ string) {
		called.Add(1)
	}, slog.Default())
	require.NoError(t, err)
	w.debounce = 50 * time.Millisecond

	require.NoError(t, w.WatchDir(dir))
	w.Start()
	defer w.Stop()

	// Write a non-JSON file
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# Hello"), 0644))

	time.Sleep(200 * time.Millisecond)
	assert.Equal(t, int32(0), called.Load())
}

func TestWatcher_Debounce(t *testing.T) {
	dir := t.TempDir()

	var called atomic.Int32
	w, err := NewWatcher(func(_ string) {
		called.Add(1)
	}, slog.Default())
	require.NoError(t, err)
	w.debounce = 100 * time.Millisecond

	require.NoError(t, w.WatchDir(dir))
	w.Start()
	defer w.Stop()

	testFile := filepath.Join(dir, "test.json")

	// Rapid writes — should only trigger once after debounce
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(testFile, []byte(`{"v":`+string(rune('0'+i))+`}`), 0644)
		time.Sleep(20 * time.Millisecond)
	}

	time.Sleep(300 * time.Millisecond)
	assert.Equal(t, int32(1), called.Load())
}

func TestWatcher_SubdirectoryWatch(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "workflows")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	var called atomic.Int32
	w, err := NewWatcher(func(_ string) {
		called.Add(1)
	}, slog.Default())
	require.NoError(t, err)
	w.debounce = 50 * time.Millisecond

	require.NoError(t, w.WatchDir(dir))
	w.Start()
	defer w.Stop()

	// Write in subdirectory
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "flow.json"), []byte(`{}`), 0644))

	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, called.Load(), int32(1))
}

// --- Reloader tests ---

func TestReloader_HandleChange_Valid(t *testing.T) {
	// Create a minimal valid config directory
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "noda.json"), map[string]any{
		"server": map[string]any{"port": 3000},
	})

	hub := trace.NewEventHub()
	initial := &config.ResolvedConfig{
		Root:      map[string]any{"server": map[string]any{"port": 3000}},
		FileCount: 1,
	}

	var reloadCalled atomic.Int32
	r := NewReloader(dir, "", initial, hub, slog.Default())
	r.OnReload(func(rc *config.ResolvedConfig) {
		reloadCalled.Add(1)
	})

	// Track events
	var events []trace.Event
	var mu sync.Mutex
	unsub := hub.Subscribe(func(data []byte) {
		var e trace.Event
		_ = json.Unmarshal(data, &e)
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	defer unsub()

	r.HandleChange(filepath.Join(dir, "noda.json"))

	assert.Equal(t, int32(1), reloadCalled.Load())
	assert.NotNil(t, r.Config())

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, events, 1)
	assert.Equal(t, trace.EventType("config:reloaded"), events[0].Type)
}

func TestReloader_HandleChange_Invalid(t *testing.T) {
	// Create a directory with an invalid config
	dir := t.TempDir()
	// Write invalid JSON
	_ = os.WriteFile(filepath.Join(dir, "noda.json"), []byte(`{invalid`), 0644)

	hub := trace.NewEventHub()
	initial := &config.ResolvedConfig{
		Root:      map[string]any{"server": map[string]any{"port": 3000}},
		FileCount: 1,
	}

	var reloadCalled atomic.Int32
	r := NewReloader(dir, "", initial, hub, slog.Default())
	r.OnReload(func(rc *config.ResolvedConfig) {
		reloadCalled.Add(1)
	})

	// Track events
	var events []trace.Event
	var mu sync.Mutex
	unsub := hub.Subscribe(func(data []byte) {
		var e trace.Event
		_ = json.Unmarshal(data, &e)
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	defer unsub()

	r.HandleChange(filepath.Join(dir, "noda.json"))

	// Should NOT have called reload
	assert.Equal(t, int32(0), reloadCalled.Load())

	// Original config should still be active
	assert.Equal(t, 3000, r.Config().Root["server"].(map[string]any)["port"])

	// Should have emitted error event
	mu.Lock()
	defer mu.Unlock()
	require.Len(t, events, 1)
	assert.Equal(t, trace.EventType("file:error"), events[0].Type)
	assert.NotEmpty(t, events[0].Error)
}

func TestReloader_Config_ThreadSafe(t *testing.T) {
	initial := &config.ResolvedConfig{
		Root:      map[string]any{},
		FileCount: 1,
	}
	r := NewReloader(".", "", initial, nil, slog.Default())

	// Access from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Config()
		}()
	}
	wg.Wait()
}

// --- Shutdown tests ---

type mockServer struct {
	stopped atomic.Bool
}

func (m *mockServer) Stop() error {
	m.stopped.Store(true)
	return nil
}

type mockScheduler struct {
	stopped atomic.Bool
}

func (m *mockScheduler) Stop(ctx context.Context) error {
	m.stopped.Store(true)
	return nil
}

type mockWasm struct {
	stopped atomic.Bool
}

func (m *mockWasm) StopAll(_ context.Context) {
	m.stopped.Store(true)
}

type mockTracer struct {
	shutdown atomic.Bool
}

func (m *mockTracer) Shutdown(_ context.Context) error {
	m.shutdown.Store(true)
	return nil
}

func TestShutdownSequence_AllComponentsStopped(t *testing.T) {
	srv := &mockServer{}
	sched := &mockScheduler{}
	wasm := &mockWasm{}
	tracer := &mockTracer{}

	dir := t.TempDir()
	watcher, err := NewWatcher(func(_ string) {}, slog.Default())
	require.NoError(t, err)
	_ = watcher.WatchDir(dir)
	watcher.Start()

	ShutdownSequence(slog.Default(), 5*time.Second, srv, sched, nil, wasm, watcher, nil, nil, tracer)

	assert.True(t, srv.stopped.Load())
	assert.True(t, sched.stopped.Load())
	assert.True(t, wasm.stopped.Load())
	assert.True(t, tracer.shutdown.Load())
}

func TestShutdownSequence_NilComponents(t *testing.T) {
	// Should not panic with nil components
	ShutdownSequence(slog.Default(), 5*time.Second, nil, nil, nil, nil, nil, nil, nil, nil)
}

// --- helpers ---

func writeJSON(t *testing.T, path string, data any) {
	t.Helper()
	b, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, b, 0644))
}
