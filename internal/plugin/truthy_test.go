package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsTruthy_Nil(t *testing.T) {
	assert.False(t, IsTruthy(nil))
}

func TestIsTruthy_BoolTrue(t *testing.T) {
	assert.True(t, IsTruthy(true))
}

func TestIsTruthy_BoolFalse(t *testing.T) {
	assert.False(t, IsTruthy(false))
}

func TestIsTruthy_IntZero(t *testing.T) {
	assert.False(t, IsTruthy(0))
}

func TestIsTruthy_IntNonZero(t *testing.T) {
	assert.True(t, IsTruthy(42))
	assert.True(t, IsTruthy(-1))
}

func TestIsTruthy_Int64Zero(t *testing.T) {
	assert.False(t, IsTruthy(int64(0)))
}

func TestIsTruthy_Int64NonZero(t *testing.T) {
	assert.True(t, IsTruthy(int64(100)))
}

func TestIsTruthy_Float64Zero(t *testing.T) {
	assert.False(t, IsTruthy(float64(0)))
}

func TestIsTruthy_Float64NonZero(t *testing.T) {
	assert.True(t, IsTruthy(float64(3.14)))
	assert.True(t, IsTruthy(float64(-0.5)))
}

func TestIsTruthy_EmptyString(t *testing.T) {
	assert.False(t, IsTruthy(""))
}

func TestIsTruthy_NonEmptyString(t *testing.T) {
	assert.True(t, IsTruthy("hello"))
	assert.True(t, IsTruthy(" "))
}

func TestIsTruthy_EmptySlice(t *testing.T) {
	assert.False(t, IsTruthy([]any{}))
	assert.False(t, IsTruthy([]string{}))
	assert.False(t, IsTruthy([]int{}))
}

func TestIsTruthy_NonEmptySlice(t *testing.T) {
	assert.True(t, IsTruthy([]any{1}))
	assert.True(t, IsTruthy([]string{"a"}))
}

func TestIsTruthy_EmptyArray(t *testing.T) {
	assert.False(t, IsTruthy([0]int{}))
}

func TestIsTruthy_NonEmptyArray(t *testing.T) {
	assert.True(t, IsTruthy([1]int{5}))
}

func TestIsTruthy_Map(t *testing.T) {
	// Maps are not slices/arrays, so they fall to the default (truthy)
	assert.True(t, IsTruthy(map[string]any{}))
	assert.True(t, IsTruthy(map[string]any{"a": 1}))
}

func TestIsTruthy_Struct(t *testing.T) {
	type s struct{}
	assert.True(t, IsTruthy(s{}))
}
