package bounded

import (
	"testing"

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
