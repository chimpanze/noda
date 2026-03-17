package lifecycle

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComponent struct {
	name     string
	startErr error
	stopErr  error
	started  bool
	stopped  bool
	log      *[]string
}

func (m *mockComponent) Name() string { return m.name }
func (m *mockComponent) Start(_ context.Context) error {
	if m.log != nil {
		*m.log = append(*m.log, "start:"+m.name)
	}
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}
func (m *mockComponent) Stop(_ context.Context) error {
	if m.log != nil {
		*m.log = append(*m.log, "stop:"+m.name)
	}
	m.stopped = true
	return m.stopErr
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
	lc.StopAll(30 * time.Second)

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
	lc.StopAll(30 * time.Second)

	// Both stopped despite b's error
	assert.Equal(t, []string{"stop:b", "stop:a"}, log)
	assert.True(t, a.stopped)
	assert.True(t, b.stopped)
}

func TestStopAll_NoStarted(t *testing.T) {
	lc := New(testLogger())
	lc.Register(&mockComponent{name: "a"})
	// Never called StartAll, so StopAll should be a no-op
	lc.StopAll(5 * time.Second)
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
