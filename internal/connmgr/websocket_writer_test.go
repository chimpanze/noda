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
	var writes int32
	write := func(_ []byte) error {
		atomic.AddInt32(&writes, 1)
		<-release // simulate a stuck (non-reading) client
		return nil
	}

	w := newWSWriter(write, nil, slog.Default(), "c1")
	defer func() { close(release); w.stop() }()

	// The writer goroutine pops the first item and blocks in write(); further
	// sends fill the bounded buffer and then drop. Every send must return
	// promptly — none may block.
	done := make(chan error, 1)
	go func() {
		var lastErr error
		for i := 0; i < wsOutboundBuffer+10; i++ {
			lastErr = w.send([]byte("x"))
		}
		done <- lastErr
	}()

	select {
	case err := <-done:
		require.Error(t, err, "sends past the buffer must drop (return error), not block")
		assert.Contains(t, err.Error(), "buffer full")
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
