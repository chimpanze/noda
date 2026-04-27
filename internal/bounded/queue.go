// Package bounded provides a generic bounded queue with explicit drop
// policies. Use New to construct; Push is non-blocking; Pop blocks until
// a value, ctx cancellation, or Close.
package bounded

import (
	"context"
	"sync"
	"sync/atomic"
)

// Policy controls Queue behaviour when Push is called on a full buffer.
type Policy int

const (
	// DropNewest rejects the incoming value; the buffer is unchanged.
	DropNewest Policy = iota
	// DropOldest evicts the head and accepts the new value.
	DropOldest
)

// Queue is a bounded, generic queue.
type Queue[T any] struct {
	mu      sync.Mutex
	cond    *sync.Cond
	buf     []T
	cap     int
	policy  Policy
	closed  atomic.Bool
	dropped atomic.Uint64
}

// New creates a Queue with the given capacity (>0) and policy.
// Panics if capacity <= 0.
func New[T any](capacity int, policy Policy) *Queue[T] {
	if capacity <= 0 {
		panic("bounded.New: capacity must be > 0")
	}
	q := &Queue[T]{
		buf:    make([]T, 0, capacity),
		cap:    capacity,
		policy: policy,
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Push enqueues v according to the configured policy. Returns true if
// accepted, false if dropped. Never blocks.
func (q *Queue[T]) Push(v T) bool {
	if q.closed.Load() {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed.Load() {
		return false
	}
	if len(q.buf) < q.cap {
		q.buf = append(q.buf, v)
		q.cond.Signal()
		return true
	}
	// Full.
	switch q.policy {
	case DropNewest:
		q.dropped.Add(1)
		return false
	case DropOldest:
		q.buf = append(q.buf[1:], v)
		q.dropped.Add(1)
		q.cond.Signal()
		return true
	default:
		q.dropped.Add(1)
		return false
	}
}

// Dropped returns the cumulative count of values rejected or evicted on full.
func (q *Queue[T]) Dropped() uint64 {
	return q.dropped.Load()
}

// tryPop returns (value, true) if a value was available, or (zero, false)
// otherwise. Test-only non-blocking helper. Do not export.
func (q *Queue[T]) tryPop() (T, bool) {
	var zero T
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.buf) == 0 {
		return zero, false
	}
	v := q.buf[0]
	q.buf = q.buf[1:]
	return v, true
}

// Pop returns the next value. It blocks until a value is available, ctx
// is cancelled, or Close is called. Returns (value, true) on success or
// (zero, false) on cancel/close.
func (q *Queue[T]) Pop(ctx context.Context) (T, bool) {
	var zero T

	// Fast path: try once without spawning the watcher.
	q.mu.Lock()
	if len(q.buf) > 0 {
		v := q.buf[0]
		q.buf = q.buf[1:]
		q.mu.Unlock()
		return v, true
	}
	if q.closed.Load() {
		q.mu.Unlock()
		return zero, false
	}
	q.mu.Unlock()

	// Pre-cancelled ctx: skip the watcher allocation.
	if ctx.Err() != nil {
		return zero, false
	}

	// Slow path: register a context watcher that broadcasts on cancel
	// to wake the Pop goroutine.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			q.mu.Lock()
			q.cond.Broadcast()
			q.mu.Unlock()
		case <-stop:
		}
	}()

	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if len(q.buf) > 0 {
			v := q.buf[0]
			q.buf = q.buf[1:]
			return v, true
		}
		if q.closed.Load() {
			return zero, false
		}
		if ctx.Err() != nil {
			return zero, false
		}
		q.cond.Wait()
	}
}
