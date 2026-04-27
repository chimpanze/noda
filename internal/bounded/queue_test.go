package bounded

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_PanicsOnZeroCapacity(t *testing.T) {
	assert.Panics(t, func() {
		New[int](0, DropNewest)
	})
}

func TestNew_PanicsOnNegativeCapacity(t *testing.T) {
	assert.Panics(t, func() {
		New[int](-1, DropNewest)
	})
}

func TestPush_DropNewest_AcceptsWhileSpaceAvailable(t *testing.T) {
	q := New[int](3, DropNewest)
	require.True(t, q.Push(1))
	require.True(t, q.Push(2))
	require.True(t, q.Push(3))
	assert.Equal(t, uint64(0), q.Dropped())
}

func TestPush_DropNewest_DropsOnFull(t *testing.T) {
	q := New[int](2, DropNewest)
	require.True(t, q.Push(1))
	require.True(t, q.Push(2))
	require.False(t, q.Push(3), "third push should be dropped")
	assert.Equal(t, uint64(1), q.Dropped())
}

func TestPush_DropOldest_AcceptsWhileSpaceAvailable(t *testing.T) {
	q := New[int](3, DropOldest)
	require.True(t, q.Push(1))
	require.True(t, q.Push(2))
	require.True(t, q.Push(3))
	assert.Equal(t, uint64(0), q.Dropped())
}

func TestPush_DropOldest_EvictsHeadOnFull(t *testing.T) {
	q := New[int](2, DropOldest)
	require.True(t, q.Push(1))
	require.True(t, q.Push(2))
	require.True(t, q.Push(3), "third push should succeed (drop oldest)")
	assert.Equal(t, uint64(1), q.Dropped())

	// Drain and check order: 2 then 3 (1 was evicted).
	got1, ok := q.tryPop()
	require.True(t, ok)
	got2, ok := q.tryPop()
	require.True(t, ok)
	assert.Equal(t, 2, got1)
	assert.Equal(t, 3, got2)
}

func TestPop_BlocksUntilPush(t *testing.T) {
	q := New[int](2, DropNewest)
	got := make(chan int, 1)
	go func() {
		v, ok := q.Pop(context.Background())
		require.True(t, ok)
		got <- v
	}()

	// Brief delay to ensure Pop is parked.
	time.Sleep(20 * time.Millisecond)
	require.True(t, q.Push(42))

	select {
	case v := <-got:
		assert.Equal(t, 42, v)
	case <-time.After(time.Second):
		t.Fatal("Pop did not return after Push")
	}
}

func TestPop_ReturnsFalseOnCtxCancel(t *testing.T) {
	q := New[int](2, DropNewest)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_, ok := q.Pop(ctx)
		assert.False(t, ok)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Pop did not return after ctx cancel")
	}
}

func TestClose_UnblocksParkedPop(t *testing.T) {
	q := New[int](2, DropNewest)

	done := make(chan struct{})
	go func() {
		_, ok := q.Pop(context.Background())
		assert.False(t, ok)
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	q.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Pop did not return after Close")
	}
}

func TestClose_PushAfterCloseReturnsFalse(t *testing.T) {
	q := New[int](2, DropNewest)
	q.Close()
	assert.False(t, q.Push(1))
}

func TestClose_PopDrainsRemainingThenReturnsFalse(t *testing.T) {
	q := New[int](2, DropNewest)
	require.True(t, q.Push(1))
	require.True(t, q.Push(2))
	q.Close()

	v1, ok := q.Pop(context.Background())
	require.True(t, ok)
	assert.Equal(t, 1, v1)

	v2, ok := q.Pop(context.Background())
	require.True(t, ok)
	assert.Equal(t, 2, v2)

	_, ok = q.Pop(context.Background())
	assert.False(t, ok)
}

func TestClose_Idempotent(t *testing.T) {
	q := New[int](2, DropNewest)
	q.Close()
	q.Close() // must not panic
}
