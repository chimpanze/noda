package lifecycle

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComponent struct {
	name      string
	startErr  error
	stopErr   error
	started   bool
	stopped   bool
	log       *[]string
	startGate chan struct{} // if non-nil, Start blocks until closed (used to interleave shutdown mid-boot)

	mu sync.Mutex // guards started/stopped for concurrent-access tests
}

func (m *mockComponent) Name() string { return m.name }
func (m *mockComponent) Start(_ context.Context) error {
	if m.log != nil {
		*m.log = append(*m.log, "start:"+m.name)
	}
	if m.startGate != nil {
		<-m.startGate
	}
	if m.startErr != nil {
		return m.startErr
	}
	m.mu.Lock()
	m.started = true
	m.mu.Unlock()
	return nil
}
func (m *mockComponent) Stop(_ context.Context) error {
	if m.log != nil {
		*m.log = append(*m.log, "stop:"+m.name)
	}
	m.mu.Lock()
	m.stopped = true
	m.mu.Unlock()
	return m.stopErr
}
func (m *mockComponent) isStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}
func (m *mockComponent) isStopped() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopped
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestStartAll_Success(t *testing.T) {
	var log []string
	lc := New(testLogger())
	a := &mockComponent{name: "a", log: &log}
	b := &mockComponent{name: "b", log: &log}
	lc.Register(a)
	lc.Register(b)

	err := lc.StartAll(context.Background())
	require.NoError(t, err)
	assert.True(t, a.started)
	assert.True(t, b.started)
	assert.Equal(t, []string{"start:a", "start:b"}, log)
}

func TestStartAll_FailureRollsBack(t *testing.T) {
	var log []string
	lc := New(testLogger())
	a := &mockComponent{name: "a", log: &log}
	b := &mockComponent{name: "b", startErr: fmt.Errorf("boom"), log: &log}
	c := &mockComponent{name: "c", log: &log}
	lc.Register(a)
	lc.Register(b)
	lc.Register(c)

	err := lc.StartAll(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
	// a was started and should be rolled back, c was never started
	assert.True(t, a.stopped)
	assert.False(t, c.started)
	assert.Equal(t, []string{"start:a", "start:b", "stop:a"}, log)
}

func TestStopAll_ReverseOrder(t *testing.T) {
	var log []string
	lc := New(testLogger())
	a := &mockComponent{name: "a", log: &log}
	b := &mockComponent{name: "b", log: &log}
	c := &mockComponent{name: "c", log: &log}
	lc.Register(a)
	lc.Register(b)
	lc.Register(c)

	require.NoError(t, lc.StartAll(context.Background()))
	log = nil // reset
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lc.StopAll(ctx)

	assert.Equal(t, []string{"stop:c", "stop:b", "stop:a"}, log)
}

func TestStopAll_ContinuesOnError(t *testing.T) {
	var log []string
	lc := New(testLogger())
	a := &mockComponent{name: "a", log: &log}
	b := &mockComponent{name: "b", stopErr: fmt.Errorf("fail"), log: &log}
	lc.Register(a)
	lc.Register(b)

	require.NoError(t, lc.StartAll(context.Background()))
	log = nil
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	lc.StopAll(ctx)

	// Both stopped despite b's error
	assert.Equal(t, []string{"stop:b", "stop:a"}, log)
	assert.True(t, a.stopped)
	assert.True(t, b.stopped)
}

func TestStopAll_NoStarted(t *testing.T) {
	lc := New(testLogger())
	lc.Register(&mockComponent{name: "a"})
	// Never called StartAll, so StopAll should be a no-op
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	lc.StopAll(ctx)
}

type fakeComponent struct {
	name    string
	startFn func(context.Context) error
	stopFn  func(context.Context) error
}

func (f *fakeComponent) Name() string { return f.name }
func (f *fakeComponent) Start(ctx context.Context) error {
	if f.startFn != nil {
		return f.startFn(ctx)
	}
	return nil
}
func (f *fakeComponent) Stop(ctx context.Context) error {
	if f.stopFn != nil {
		return f.stopFn(ctx)
	}
	return nil
}

func TestStopAll_PropagatesParentCancel(t *testing.T) {
	lc := New(testLogger())

	stopCalled := make(chan context.Context, 2)
	lc.Register(&fakeComponent{
		name: "first",
		stopFn: func(ctx context.Context) error {
			stopCalled <- ctx
			// Block until ctx is done so parent-cancel observation is forced.
			<-ctx.Done()
			return ctx.Err()
		},
	})
	lc.Register(&fakeComponent{
		name: "second",
		stopFn: func(ctx context.Context) error {
			stopCalled <- ctx
			return nil
		},
	})

	require.NoError(t, lc.StartAll(context.Background()))

	parent, cancelParent := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelParent()

	doneStop := make(chan struct{})
	go func() {
		lc.StopAll(parent)
		close(doneStop)
	}()

	// Wait until "second" (reverse order: stops first) is in Stop.
	<-stopCalled
	cancelParent()

	select {
	case <-doneStop:
	case <-time.After(2 * time.Second):
		t.Fatal("StopAll did not return after parent ctx cancelled")
	}
}

func TestStartAll_ShutdownDuringBootIsHonored(t *testing.T) {
	lc := New(testLogger())
	gate := make(chan struct{})
	a := &mockComponent{name: "a"}                  // starts immediately
	b := &mockComponent{name: "b", startGate: gate} // blocks in Start
	lc.Register(a)
	lc.Register(b)

	startErr := make(chan error, 1)
	go func() { startErr <- lc.StartAll(context.Background()) }()

	// Wait until "a" has started and StartAll is blocked in b.Start.
	require.Eventually(t, func() bool { return a.isStarted() }, time.Second, 5*time.Millisecond)

	// Fire shutdown while b is still starting.
	go lc.StopAll(context.Background())
	// Let b finish starting.
	time.Sleep(20 * time.Millisecond)
	close(gate)

	<-startErr
	require.Eventually(t, func() bool { return a.isStopped() }, time.Second, 5*time.Millisecond,
		"started component must be stopped by the shutdown")
	// b must also end up stopped if it started, OR never counted — in all cases not left running.
	require.Eventually(t, func() bool { return !b.isStarted() || b.isStopped() }, time.Second, 5*time.Millisecond,
		"a component that started must not be left running after shutdown")
}

func TestStartAll_AbortsWhenAlreadyShuttingDown(t *testing.T) {
	lc := New(testLogger())
	x := &mockComponent{name: "x"}
	lc.Register(x)
	lc.StopAll(context.Background()) // sets shuttingDown
	err := lc.StartAll(context.Background())
	require.Error(t, err)
	require.False(t, x.isStarted(), "no component should start once shutting down")
}

func TestRegisterOrder(t *testing.T) {
	lc := New(testLogger())
	assert.Empty(t, lc.components)
	lc.Register(&mockComponent{name: "x"})
	lc.Register(&mockComponent{name: "y"})
	assert.Len(t, lc.components, 2)
	assert.Equal(t, "x", lc.components[0].Name())
	assert.Equal(t, "y", lc.components[1].Name())
}
