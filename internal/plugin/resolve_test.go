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
