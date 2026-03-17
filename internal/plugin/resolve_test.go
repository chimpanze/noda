package plugin

import (
	"fmt"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockExecCtx struct {
	resolveFunc func(expr string) (any, error)
}

func (m *mockExecCtx) Input() any          { return nil }
func (m *mockExecCtx) Auth() *api.AuthData { return nil }
func (m *mockExecCtx) Trigger() api.TriggerData {
	return api.TriggerData{}
}
func (m *mockExecCtx) Resolve(expr string) (any, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(expr)
	}
	return expr, nil
}
func (m *mockExecCtx) ResolveWithVars(expr string, _ map[string]any) (any, error) {
	return m.Resolve(expr)
}
func (m *mockExecCtx) Log(_ string, _ string, _ map[string]any) {}

func identityResolve(expr string) (any, error) {
	return expr, nil
}

// --- ResolveMap tests ---

func TestResolveMap_MissingKey(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := ResolveMap(nCtx, map[string]any{}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestResolveMap_MapWithStringValues(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved_" + expr, nil
	}}
	config := map[string]any{
		"data": map[string]any{"name": "expr1", "email": "expr2"},
	}
	result, err := ResolveMap(nCtx, config, "data")
	require.NoError(t, err)
	assert.Equal(t, "resolved_expr1", result["name"])
	assert.Equal(t, "resolved_expr2", result["email"])
}

func TestResolveMap_MapWithNonStringValues(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"data": map[string]any{"count": 42, "active": true, "name": "val"},
	}
	result, err := ResolveMap(nCtx, config, "data")
	require.NoError(t, err)
	assert.Equal(t, 42, result["count"])
	assert.Equal(t, true, result["active"])
}

func TestResolveMap_MapResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("resolve failed")
	}}
	config := map[string]any{"data": map[string]any{"name": "bad"}}
	_, err := ResolveMap(nCtx, config, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve data.name")
}

func TestResolveMap_StringExpression(t *testing.T) {
	expected := map[string]any{"key": "value"}
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return expected, nil
	}}
	result, err := ResolveMap(nCtx, map[string]any{"data": "{{ expr }}"}, "data")
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestResolveMap_StringExpressionResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("error")
	}}
	_, err := ResolveMap(nCtx, map[string]any{"data": "{{ bad }}"}, "data")
	require.Error(t, err)
}

func TestResolveMap_StringExpressionResolvesToNonMap(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return "string", nil
	}}
	_, err := ResolveMap(nCtx, map[string]any{"data": "{{ expr }}"}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected map")
}

func TestResolveMap_InvalidType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := ResolveMap(nCtx, map[string]any{"data": 42}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an object or expression string")
}

// --- ResolveOptionalMap tests ---

func TestResolveOptionalMap_Absent(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	result, err := ResolveOptionalMap(nCtx, map[string]any{}, "data")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestResolveOptionalMap_Present(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{"data": map[string]any{"key": "val"}}
	result, err := ResolveOptionalMap(nCtx, config, "data")
	require.NoError(t, err)
	assert.Equal(t, "val", result["key"])
}

func TestResolveOptionalMap_AsExpression(t *testing.T) {
	expected := map[string]any{"a": "b"}
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return expected, nil
	}}
	result, err := ResolveOptionalMap(nCtx, map[string]any{"data": "{{ expr }}"}, "data")
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

// --- ResolveOptionalArray tests ---

func TestResolveOptionalArray_Absent(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	result, err := ResolveOptionalArray(nCtx, map[string]any{}, "params")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestResolveOptionalArray_ArrayWithStringItems(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved_" + expr, nil
	}}
	config := map[string]any{"params": []any{"a", "b"}}
	result, err := ResolveOptionalArray(nCtx, config, "params")
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "resolved_a", result[0])
	assert.Equal(t, "resolved_b", result[1])
}

func TestResolveOptionalArray_ArrayWithNonStringItems(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{"params": []any{42, true}}
	result, err := ResolveOptionalArray(nCtx, config, "params")
	require.NoError(t, err)
	assert.Equal(t, 42, result[0])
	assert.Equal(t, true, result[1])
}

func TestResolveOptionalArray_ArrayResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("resolve failed")
	}}
	config := map[string]any{"items": []any{"bad"}}
	_, err := ResolveOptionalArray(nCtx, config, "items")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve items[0]")
}

func TestResolveOptionalArray_StringExpression(t *testing.T) {
	expected := []any{"a", "b"}
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return expected, nil
	}}
	result, err := ResolveOptionalArray(nCtx, map[string]any{"params": "{{ expr }}"}, "params")
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestResolveOptionalArray_StringExpressionResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("error")
	}}
	_, err := ResolveOptionalArray(nCtx, map[string]any{"params": "{{ bad }}"}, "params")
	require.Error(t, err)
}

func TestResolveOptionalArray_StringExpressionResolvesToNonArray(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return "string", nil
	}}
	_, err := ResolveOptionalArray(nCtx, map[string]any{"params": "{{ expr }}"}, "params")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected array")
}

func TestResolveOptionalArray_InvalidType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := ResolveOptionalArray(nCtx, map[string]any{"params": 42}, "params")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an array or expression string")
}

// --- ResolveString tests ---

func TestResolveString_Success(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "hello", nil
	}}
	result, err := ResolveString(nCtx, map[string]any{"name": "expr"}, "name")
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestResolveString_MissingKey(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := ResolveString(nCtx, map[string]any{}, "name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestResolveString_NotAString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := ResolveString(nCtx, map[string]any{"name": 42}, "name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestResolveString_ResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("boom")
	}}
	_, err := ResolveString(nCtx, map[string]any{"name": "expr"}, "name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve")
}

func TestResolveString_ResolvesToNonString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return 42, nil
	}}
	_, err := ResolveString(nCtx, map[string]any{"name": "expr"}, "name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected string")
}

// --- ResolveOptionalString tests ---

func TestResolveOptionalString_Absent(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	val, ok, err := ResolveOptionalString(nCtx, map[string]any{}, "name")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, "", val)
}

func TestResolveOptionalString_Present(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved", nil
	}}
	val, ok, err := ResolveOptionalString(nCtx, map[string]any{"name": "expr"}, "name")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "resolved", val)
}

func TestResolveOptionalString_NotAString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, _, err := ResolveOptionalString(nCtx, map[string]any{"name": 42}, "name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestResolveOptionalString_ResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("boom")
	}}
	_, _, err := ResolveOptionalString(nCtx, map[string]any{"name": "expr"}, "name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve")
}

func TestResolveOptionalString_ResolvesToNonString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return 123, nil
	}}
	_, _, err := ResolveOptionalString(nCtx, map[string]any{"name": "expr"}, "name")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected string")
}

// --- ResolveAny tests ---

func TestResolveAny_MissingKey(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := ResolveAny(nCtx, map[string]any{}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestResolveAny_StringValue(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved_" + expr, nil
	}}
	val, err := ResolveAny(nCtx, map[string]any{"data": "expr"}, "data")
	require.NoError(t, err)
	assert.Equal(t, "resolved_expr", val)
}

func TestResolveAny_NonStringValue(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	val, err := ResolveAny(nCtx, map[string]any{"data": 42}, "data")
	require.NoError(t, err)
	assert.Equal(t, 42, val)
}

func TestResolveAny_ResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("boom")
	}}
	_, err := ResolveAny(nCtx, map[string]any{"data": "expr"}, "data")
	require.Error(t, err)
}

// --- ResolveOptionalAny tests ---

func TestResolveOptionalAny_Absent(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	val, ok, err := ResolveOptionalAny(nCtx, map[string]any{}, "data")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Nil(t, val)
}

func TestResolveOptionalAny_StringValue(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved", nil
	}}
	val, ok, err := ResolveOptionalAny(nCtx, map[string]any{"data": "expr"}, "data")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "resolved", val)
}

func TestResolveOptionalAny_NonStringValue(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	val, ok, err := ResolveOptionalAny(nCtx, map[string]any{"data": 99}, "data")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 99, val)
}

func TestResolveOptionalAny_ResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("boom")
	}}
	_, _, err := ResolveOptionalAny(nCtx, map[string]any{"data": "expr"}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve")
}

// --- ResolveDeepAny tests ---

func TestResolveDeepAny_MissingKey(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := ResolveDeepAny(nCtx, map[string]any{}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestResolveDeepAny_StringValue(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved_" + expr, nil
	}}
	val, err := ResolveDeepAny(nCtx, map[string]any{"data": "expr"}, "data")
	require.NoError(t, err)
	assert.Equal(t, "resolved_expr", val)
}

func TestResolveDeepAny_NestedMap(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "R:" + expr, nil
	}}
	config := map[string]any{
		"data": map[string]any{
			"name": "n",
			"nested": map[string]any{
				"inner": "i",
			},
		},
	}
	val, err := ResolveDeepAny(nCtx, config, "data")
	require.NoError(t, err)
	m := val.(map[string]any)
	assert.Equal(t, "R:n", m["name"])
	nested := m["nested"].(map[string]any)
	assert.Equal(t, "R:i", nested["inner"])
}

func TestResolveDeepAny_NestedSlice(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "R:" + expr, nil
	}}
	config := map[string]any{
		"data": []any{"a", "b"},
	}
	val, err := ResolveDeepAny(nCtx, config, "data")
	require.NoError(t, err)
	arr := val.([]any)
	assert.Equal(t, "R:a", arr[0])
	assert.Equal(t, "R:b", arr[1])
}

func TestResolveDeepAny_NonStringNonContainer(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	val, err := ResolveDeepAny(nCtx, map[string]any{"data": 42}, "data")
	require.NoError(t, err)
	assert.Equal(t, 42, val)
}

func TestResolveDeepAny_ResolveErrorInMap(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("boom")
	}}
	config := map[string]any{
		"data": map[string]any{"key": "expr"},
	}
	_, err := ResolveDeepAny(nCtx, config, "data")
	require.Error(t, err)
}

func TestResolveDeepAny_ResolveErrorInSlice(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("boom")
	}}
	config := map[string]any{
		"data": []any{"expr"},
	}
	_, err := ResolveDeepAny(nCtx, config, "data")
	require.Error(t, err)
}

// --- ResolveOptionalInt tests ---

func TestResolveOptionalInt_Absent(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	val, ok, err := ResolveOptionalInt(nCtx, map[string]any{}, "limit")
	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, 0, val)
}

func TestResolveOptionalInt_Float64(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	val, ok, err := ResolveOptionalInt(nCtx, map[string]any{"limit": float64(10)}, "limit")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 10, val)
}

func TestResolveOptionalInt_Int(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	val, ok, err := ResolveOptionalInt(nCtx, map[string]any{"limit": 5}, "limit")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 5, val)
}

func TestResolveOptionalInt_StringResolvesToFloat64(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return float64(42), nil
	}}
	val, ok, err := ResolveOptionalInt(nCtx, map[string]any{"limit": "expr"}, "limit")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 42, val)
}

func TestResolveOptionalInt_StringResolvesToInt(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return 7, nil
	}}
	val, ok, err := ResolveOptionalInt(nCtx, map[string]any{"limit": "expr"}, "limit")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 7, val)
}

func TestResolveOptionalInt_StringResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("boom")
	}}
	_, _, err := ResolveOptionalInt(nCtx, map[string]any{"limit": "expr"}, "limit")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve")
}

func TestResolveOptionalInt_StringResolvesToNonInt(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return "not a number", nil
	}}
	_, _, err := ResolveOptionalInt(nCtx, map[string]any{"limit": "expr"}, "limit")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected int")
}

func TestResolveOptionalInt_InvalidType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, _, err := ResolveOptionalInt(nCtx, map[string]any{"limit": true}, "limit")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid type")
}

// --- ToInt tests ---

func TestToInt_Float64(t *testing.T) {
	v, ok := ToInt(float64(3.7))
	assert.True(t, ok)
	assert.Equal(t, 3, v)
}

func TestToInt_Int(t *testing.T) {
	v, ok := ToInt(42)
	assert.True(t, ok)
	assert.Equal(t, 42, v)
}

func TestToInt_Int64(t *testing.T) {
	v, ok := ToInt(int64(100))
	assert.True(t, ok)
	assert.Equal(t, 100, v)
}

func TestToInt_StringValid(t *testing.T) {
	v, ok := ToInt("99")
	assert.True(t, ok)
	assert.Equal(t, 99, v)
}

func TestToInt_StringInvalid(t *testing.T) {
	v, ok := ToInt("abc")
	assert.False(t, ok)
	assert.Equal(t, 0, v)
}

func TestToInt_UnsupportedType(t *testing.T) {
	v, ok := ToInt(true)
	assert.False(t, ok)
	assert.Equal(t, 0, v)
}

// --- ToInt64 tests ---

func TestToInt64_Float64(t *testing.T) {
	v, ok := ToInt64(float64(3.7))
	assert.True(t, ok)
	assert.Equal(t, int64(3), v)
}

func TestToInt64_Int(t *testing.T) {
	v, ok := ToInt64(42)
	assert.True(t, ok)
	assert.Equal(t, int64(42), v)
}

func TestToInt64_Int64(t *testing.T) {
	v, ok := ToInt64(int64(100))
	assert.True(t, ok)
	assert.Equal(t, int64(100), v)
}

func TestToInt64_UnsupportedType(t *testing.T) {
	v, ok := ToInt64("hello")
	assert.False(t, ok)
	assert.Equal(t, int64(0), v)
}

// --- ResolveRawInt tests ---

func TestResolveRawInt_Float64(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	v, err := ResolveRawInt(nCtx, float64(10))
	require.NoError(t, err)
	assert.Equal(t, 10, v)
}

func TestResolveRawInt_Int(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	v, err := ResolveRawInt(nCtx, 5)
	require.NoError(t, err)
	assert.Equal(t, 5, v)
}

func TestResolveRawInt_StringResolvesToFloat64(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return float64(42), nil
	}}
	v, err := ResolveRawInt(nCtx, "expr")
	require.NoError(t, err)
	assert.Equal(t, 42, v)
}

func TestResolveRawInt_StringResolvesToInt(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return 7, nil
	}}
	v, err := ResolveRawInt(nCtx, "expr")
	require.NoError(t, err)
	assert.Equal(t, 7, v)
}

func TestResolveRawInt_StringResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("boom")
	}}
	_, err := ResolveRawInt(nCtx, "expr")
	require.Error(t, err)
}

func TestResolveRawInt_StringResolvesToNonNumber(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return "not a number", nil
	}}
	_, err := ResolveRawInt(nCtx, "expr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected number")
}

func TestResolveRawInt_UnsupportedType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := ResolveRawInt(nCtx, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected number")
}

// --- ResolveHeaders tests ---

func TestResolveHeaders_Absent(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	result, err := ResolveHeaders(nCtx, map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestResolveHeaders_NotAMap(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := ResolveHeaders(nCtx, map[string]any{"headers": "bad"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a map")
}

func TestResolveHeaders_StringValues(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "val:" + expr, nil
	}}
	config := map[string]any{
		"headers": map[string]any{
			"Content-Type":  "ct_expr",
			"Authorization": "auth_expr",
		},
	}
	result, err := ResolveHeaders(nCtx, config)
	require.NoError(t, err)
	assert.Equal(t, "val:ct_expr", result["Content-Type"])
	assert.Equal(t, "val:auth_expr", result["Authorization"])
}

func TestResolveHeaders_NonStringValues(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"headers": map[string]any{
			"X-Count": 42,
		},
	}
	result, err := ResolveHeaders(nCtx, config)
	require.NoError(t, err)
	assert.Equal(t, "42", result["X-Count"])
}

func TestResolveHeaders_ResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("boom")
	}}
	config := map[string]any{
		"headers": map[string]any{"X-Key": "expr"},
	}
	_, err := ResolveHeaders(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve header")
}

func TestResolveHeaders_NonStringResolvedValue(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return 99, nil
	}}
	config := map[string]any{
		"headers": map[string]any{"X-Num": "expr"},
	}
	result, err := ResolveHeaders(nCtx, config)
	require.NoError(t, err)
	assert.Equal(t, "99", result["X-Num"])
}
