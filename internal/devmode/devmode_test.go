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
	defer func() { _ = w.Stop(context.Background()) }()

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
	defer func() { _ = w.Stop(context.Background()) }()

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
	w.debounce = 500 * time.Millisecond

	require.NoError(t, w.WatchDir(dir))
	w.Start()
	defer func() { _ = w.Stop(context.Background()) }()

	testFile := filepath.Join(dir, "test.json")

	// Rapid writes — should only trigger once after debounce
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(testFile, []byte(`{"v":`+string(rune('0'+i))+`}`), 0644)
		time.Sleep(5 * time.Millisecond)
	}

	require.Eventually(t, func() bool { return called.Load() >= 1 }, 3*time.Second, 20*time.Millisecond)
	time.Sleep(700 * time.Millisecond)
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
	defer func() { _ = w.Stop(context.Background()) }()

	// Write in subdirectory
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "flow.json"), []byte(`{}`), 0644))

	time.Sleep(200 * time.Millisecond)
	assert.GreaterOrEqual(t, called.Load(), int32(1))
}

func TestWatcher_ReactsToNewSubdirAndDelete(t *testing.T) {
	dir := t.TempDir()
	changes := make(chan string, 8)
	w, err := NewWatcher(func(p string) { changes <- p }, slog.Default())
	require.NoError(t, err)
	w.debounce = 20 * time.Millisecond
	require.NoError(t, w.WatchDir(dir))
	w.Start()
	defer func() { _ = w.Stop(context.Background()) }()

	// (platform-5) a .json created under a NEW subdirectory triggers a reload.
	sub := filepath.Join(dir, "routes")
	require.NoError(t, os.Mkdir(sub, 0755))
	time.Sleep(50 * time.Millisecond) // let the watcher pick up the new dir
	require.NoError(t, os.WriteFile(filepath.Join(sub, "r.json"), []byte("{}"), 0644))
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("no reload for a .json created under a new subdirectory")
	}

	// (platform-6) deleting a watched .json triggers a reload.
	f := filepath.Join(dir, "top.json")
	require.NoError(t, os.WriteFile(f, []byte("{}"), 0644))
	<-changes // the create
	require.NoError(t, os.Remove(f))
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("no reload on .json delete")
	}
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

	// Events are delivered asynchronously via per-subscriber inbox goroutine;
	// allow a brief window for the goroutine to drain the inbox.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) == 1
	}, time.Second, 5*time.Millisecond)

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

	// Events are delivered asynchronously via per-subscriber inbox goroutine;
	// allow a brief window for the goroutine to drain the inbox.
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(events) == 1
	}, time.Second, 5*time.Millisecond)

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

func TestReloader_ConfigVisibleOnlyAfterOnReloadCompletes(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "noda.json"), map[string]any{
		"server": map[string]any{"port": 3000},
	})

	initial := &config.ResolvedConfig{
		Root:      map[string]any{"server": map[string]any{"port": 3000}},
		FileCount: 1,
	}

	r := NewReloader(dir, "", initial, nil, slog.Default())

	// onReload sets a "in progress" flag, sleeps, clears the flag.
	var inProgress atomic.Bool
	r.OnReload(func(rc *config.ResolvedConfig) {
		inProgress.Store(true)
		time.Sleep(50 * time.Millisecond)
		inProgress.Store(false)
	})

	// Reader: continuously reads Config(); record any (config, inProgress) sample.
	var (
		mu         sync.Mutex
		violations int
	)
	stopReader := make(chan struct{})
	readerDone := make(chan struct{})
	originalConfig := r.Config()

	go func() {
		defer close(readerDone)
		for {
			select {
			case <-stopReader:
				return
			default:
			}
			cur := r.Config()
			progress := inProgress.Load()
			// Violation: reader sees a *new* config pointer while onReload
			// is still in flight.
			if cur != originalConfig && progress {
				mu.Lock()
				violations++
				mu.Unlock()
			}
		}
	}()

	// Trigger the reload (synchronous — blocks until onReload completes).
	r.HandleChange(filepath.Join(dir, "noda.json"))

	close(stopReader)
	<-readerDone

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, violations,
		"reader observed new config while onReload was still running")
}

func TestReloader_ShutdownAwaitsInFlightReload(t *testing.T) {
	dir := t.TempDir()
	writeJSON(t, filepath.Join(dir, "noda.json"), map[string]any{
		"server": map[string]any{"port": 3000},
	})

	initial := &config.ResolvedConfig{
		Root:      map[string]any{"server": map[string]any{"port": 3000}},
		FileCount: 1,
	}

	r := NewReloader(dir, "", initial, nil, slog.Default())
	inReload := make(chan struct{})
	releaseReload := make(chan struct{})
	var reloadRan atomic.Bool
	var closeOnce sync.Once
	r.OnReload(func(*config.ResolvedConfig) {
		reloadRan.Store(true)
		closeOnce.Do(func() { close(inReload) })
		<-releaseReload // hold the reload open
	})

	go r.HandleChange(filepath.Join(dir, "noda.json")) // enters onReload, blocks
	<-inReload

	shutdownReturned := make(chan struct{})
	go func() { r.Shutdown(context.Background()); close(shutdownReturned) }()
	select {
	case <-shutdownReturned:
		t.Fatal("Shutdown returned before the in-flight reload finished")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseReload) // let the reload finish
	<-shutdownReturned   // Shutdown now returns

	// A reload after shutdown must not fire onReload again.
	reloadRan.Store(false)
	r.HandleChange(filepath.Join(dir, "noda.json"))
	require.False(t, reloadRan.Load(), "onReload must not fire after Shutdown")
}

// #287: Shutdown must respect the ctx budget instead of draining reloadMu
// unboundedly when a reload is stuck.
func TestReloaderShutdown_BoundedByContext(t *testing.T) {
	initial := &config.ResolvedConfig{
		Root:      map[string]any{"server": map[string]any{"port": 3000}},
		FileCount: 1,
	}
	r := NewReloader(t.TempDir(), "", initial, nil, slog.Default())

	release := make(chan struct{})
	r.reloadMu.Lock()
	go func() { <-release; r.reloadMu.Unlock() }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	r.Shutdown(ctx)
	assert.Less(t, time.Since(start), time.Second, "Shutdown must return on ctx expiry")
	close(release)
}

func TestReloaderShutdown_WaitsForBarrierWhenUnpressured(t *testing.T) {
	initial := &config.ResolvedConfig{
		Root:      map[string]any{"server": map[string]any{"port": 3000}},
		FileCount: 1,
	}
	r := NewReloader(t.TempDir(), "", initial, nil, slog.Default())

	r.reloadMu.Lock()
	go func() { time.Sleep(30 * time.Millisecond); r.reloadMu.Unlock() }()
	start := time.Now()
	r.Shutdown(context.Background()) // no deadline: waits for the barrier
	assert.GreaterOrEqual(t, time.Since(start), 25*time.Millisecond)
}

// --- helpers ---

func writeJSON(t *testing.T, path string, data any) {
	t.Helper()
	b, err := json.MarshalIndent(data, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, b, 0644))
}
