package connmgr

import (
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realtime-1: the per-conn outbound writer must be non-blocking — a stuck
// client (slow/blocked write) must not block the sender, and overflow must be
// dropped rather than blocking the whole channel.
func TestWSWriter_NonBlockingAndDropsOnFull(t *testing.T) {
	release := make(chan struct{})
	var writes atomic.Int32
	write := func(_ []byte) error {
		writes.Add(1)
		<-release // simulate a stuck (non-reading) client
		return nil
	}

	w := newWSWriter(write, nil, slog.Default(), "c1")
	defer func() { close(release); w.stop() }()

	// Park the writer goroutine deterministically: send one primer frame and
	// wait until write() has picked it up. Without this, a late-scheduled pop
	// can free a buffer slot mid-fill and let the final send sneak in.
	require.NoError(t, w.send([]byte("primer")))
	require.Eventually(t, func() bool { return writes.Load() == 1 },
		2*time.Second, time.Millisecond, "writer goroutine never picked up the primer frame")

	// The writer is stuck in write() and the buffer is empty; the next
	// wsOutboundBuffer sends fill it and exactly the 10 past that must drop.
	// Every send must return promptly — none may block.
	done := make(chan error, 1)
	var dropped atomic.Int32
	go func() {
		var lastErr error
		for range wsOutboundBuffer + 10 {
			if err := w.send([]byte("x")); err != nil {
				dropped.Add(1)
				lastErr = err
			}
		}
		done <- lastErr
	}()

	select {
	case err := <-done:
		require.Error(t, err, "sends past the buffer must drop (return error), not block")
		assert.Contains(t, err.Error(), "buffer full")
		assert.EqualValues(t, 10, dropped.Load(), "exactly the overflow sends must drop")
	case <-time.After(2 * time.Second):
		t.Fatal("w.send blocked — head-of-line blocking not fixed")
	}
}

// realtime-1: after stop(), send must reject rather than enqueue.
func TestWSWriter_SendAfterStop(t *testing.T) {
	w := newWSWriter(func(_ []byte) error { return nil }, nil, slog.Default(), "c1")
	w.stop()
	err := w.send([]byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}
