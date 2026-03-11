package db

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- resolveMap tests ---

func TestResolveMap_MissingKey(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := resolveMap(nCtx, map[string]any{}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
	assert.Contains(t, err.Error(), "data")
}

func TestResolveMap_MapWithStringValues(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved_" + expr, nil
	}}

	config := map[string]any{
		"data": map[string]any{
			"name":  "expr1",
			"email": "expr2",
		},
	}

	result, err := resolveMap(nCtx, config, "data")
	require.NoError(t, err)
	assert.Equal(t, "resolved_expr1", result["name"])
	assert.Equal(t, "resolved_expr2", result["email"])
}

func TestResolveMap_MapWithNonStringValues(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"data": map[string]any{
			"count":  42,
			"active": true,
			"name":   "some_expr",
		},
	}

	result, err := resolveMap(nCtx, config, "data")
	require.NoError(t, err)
	assert.Equal(t, 42, result["count"])
	assert.Equal(t, true, result["active"])
	assert.Equal(t, "some_expr", result["name"]) // string value goes through resolve
}

func TestResolveMap_MapWithResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return nil, fmt.Errorf("resolve failed")
	}}

	config := map[string]any{
		"data": map[string]any{
			"name": "bad_expr",
		},
	}

	_, err := resolveMap(nCtx, config, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve data.name")
}

func TestResolveMap_StringExpressionResolvesToMap(t *testing.T) {
	expected := map[string]any{"key": "value", "num": 42}
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return expected, nil
	}}

	result, err := resolveMap(nCtx, map[string]any{"data": "{{ some_expr }}"}, "data")
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestResolveMap_StringExpressionResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("expression error")
	}}

	_, err := resolveMap(nCtx, map[string]any{"data": "{{ bad }}"}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve")
	assert.Contains(t, err.Error(), "data")
}

func TestResolveMap_StringExpressionResolvesToNonMap(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return "just a string", nil
	}}

	_, err := resolveMap(nCtx, map[string]any{"data": "{{ expr }}"}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected map")
}

func TestResolveMap_InvalidType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, err := resolveMap(nCtx, map[string]any{"data": 42}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an object or expression string")
}

func TestResolveMap_InvalidTypeSlice(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, err := resolveMap(nCtx, map[string]any{"data": []any{1, 2, 3}}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an object or expression string")
}

func TestResolveMap_InvalidTypeBool(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, err := resolveMap(nCtx, map[string]any{"data": true}, "data")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an object or expression string")
}

// --- resolveParams tests ---

func TestResolveParams_MissingKey(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	result, err := resolveParams(nCtx, map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestResolveParams_ArrayWithStringItems(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved_" + expr, nil
	}}

	config := map[string]any{
		"params": []any{"expr1", "expr2"},
	}

	result, err := resolveParams(nCtx, config)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "resolved_expr1", result[0])
	assert.Equal(t, "resolved_expr2", result[1])
}

func TestResolveParams_ArrayWithNonStringItems(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"params": []any{42, true, "expr1"},
	}

	result, err := resolveParams(nCtx, config)
	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, 42, result[0])
	assert.Equal(t, true, result[1])
	assert.Equal(t, "expr1", result[2]) // string goes through resolve which returns as-is
}

func TestResolveParams_ArrayWithResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("resolve failed")
	}}

	config := map[string]any{
		"params": []any{"bad_expr"},
	}

	_, err := resolveParams(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve params[0]")
}

func TestResolveParams_StringExpressionResolvesToArray(t *testing.T) {
	expected := []any{"a", "b", "c"}
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return expected, nil
	}}

	result, err := resolveParams(nCtx, map[string]any{"params": "{{ expr }}"})
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestResolveParams_StringExpressionResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return nil, fmt.Errorf("expression error")
	}}

	_, err := resolveParams(nCtx, map[string]any{"params": "{{ bad }}"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve params")
}

func TestResolveParams_StringExpressionResolvesToNonArray(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return "just a string", nil
	}}

	_, err := resolveParams(nCtx, map[string]any{"params": "{{ expr }}"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected array")
}

func TestResolveParams_InvalidType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, err := resolveParams(nCtx, map[string]any{"params": 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an array or expression string")
}

func TestResolveParams_InvalidTypeMap(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, err := resolveParams(nCtx, map[string]any{"params": map[string]any{"key": "val"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an array or expression string")
}

func TestResolveParams_InvalidTypeBool(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, err := resolveParams(nCtx, map[string]any{"params": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an array or expression string")
}

// --- Descriptor tests for coverage ---

func TestDescriptors_ConfigSchema(t *testing.T) {
	descriptors := []struct {
		name string
		desc interface{ ConfigSchema() map[string]any }
	}{
		{"query", &queryDescriptor{}},
		{"exec", &execDescriptor{}},
		{"create", &createDescriptor{}},
		{"update", &updateDescriptor{}},
		{"delete", &deleteDescriptor{}},
	}

	for _, tt := range descriptors {
		t.Run(tt.name, func(t *testing.T) {
			schema := tt.desc.ConfigSchema()
			require.NotNil(t, schema)
			assert.Equal(t, "object", schema["type"])
			props, ok := schema["properties"].(map[string]any)
			require.True(t, ok, "properties should be a map")
			assert.NotEmpty(t, props)
		})
	}
}

func TestDescriptors_Outputs(t *testing.T) {
	executors := []struct {
		name    string
		factory func(map[string]any) interface{ Outputs() []string }
	}{
		{"query", func(c map[string]any) interface{ Outputs() []string } { return newQueryExecutor(c).(*queryExecutor) }},
		{"exec", func(c map[string]any) interface{ Outputs() []string } { return newExecExecutor(c).(*execExecutor) }},
		{"create", func(c map[string]any) interface{ Outputs() []string } { return newCreateExecutor(c).(*createExecutor) }},
		{"update", func(c map[string]any) interface{ Outputs() []string } { return newUpdateExecutor(c).(*updateExecutor) }},
		{"delete", func(c map[string]any) interface{ Outputs() []string } { return newDeleteExecutor(c).(*deleteExecutor) }},
	}

	for _, tt := range executors {
		t.Run(tt.name, func(t *testing.T) {
			executor := tt.factory(nil)
			outputs := executor.Outputs()
			assert.Contains(t, outputs, "success")
			assert.Contains(t, outputs, "error")
		})
	}
}

// --- Node-level error path tests for coverage ---

func TestQueryNode_MissingQueryField(t *testing.T) {
	db := newTestDB(t)
	exec := &queryExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.query")
}

func TestExecNode_MissingQueryField(t *testing.T) {
	db := newTestDB(t)
	exec := &execExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.exec")
}

func TestCreateNode_MissingTableField(t *testing.T) {
	db := newTestDB(t)
	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"data": map[string]any{"title": "test"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.create")
}

func TestCreateNode_MissingDataField(t *testing.T) {
	db := newTestDB(t)
	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.create")
}

func TestUpdateNode_MissingTableField(t *testing.T) {
	db := newTestDB(t)
	exec := &updateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"data":      map[string]any{"status": "done"},
		"condition": "id = ?",
		"params":    []any{1},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.update")
}

func TestUpdateNode_MissingDataField(t *testing.T) {
	db := newTestDB(t)
	exec := &updateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":     "tasks",
		"condition": "id = ?",
		"params":    []any{1},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.update")
}

func TestUpdateNode_MissingConditionField(t *testing.T) {
	db := newTestDB(t)
	exec := &updateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"data":  map[string]any{"status": "done"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.update")
}

func TestDeleteNode_MissingTableField(t *testing.T) {
	db := newTestDB(t)
	exec := &deleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"condition": "id = ?",
		"params":    []any{1},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.delete")
}

func TestDeleteNode_MissingConditionField(t *testing.T) {
	db := newTestDB(t)
	exec := &deleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.delete")
}

func TestQueryNode_ResolveParamsError(t *testing.T) {
	db := newTestDB(t)
	exec := &queryExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		if expr == "SELECT 1" {
			return "SELECT 1", nil
		}
		return nil, fmt.Errorf("resolve error")
	}}

	config := map[string]any{
		"query":  "SELECT 1",
		"params": []any{"bad_expr"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.query")
}

func TestExecNode_ResolveParamsError(t *testing.T) {
	db := newTestDB(t)
	exec := &execExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		if expr == "DELETE FROM tasks" {
			return "DELETE FROM tasks", nil
		}
		return nil, fmt.Errorf("resolve error")
	}}

	config := map[string]any{
		"query":  "DELETE FROM tasks",
		"params": []any{"bad_expr"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.exec")
}

func TestUpdateNode_ResolveParamsError(t *testing.T) {
	db := newTestDB(t)
	exec := &updateExecutor{}
	callCount := 0
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		callCount++
		// Allow table, data values, and condition to resolve, but fail on params
		if expr == "tasks" || expr == "done" || expr == "id = ?" {
			return expr, nil
		}
		return nil, fmt.Errorf("resolve error")
	}}

	config := map[string]any{
		"table":     "tasks",
		"data":      map[string]any{"status": "done"},
		"condition": "id = ?",
		"params":    []any{"bad_expr"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.update")
}

func TestDeleteNode_ResolveParamsError(t *testing.T) {
	db := newTestDB(t)
	exec := &deleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		if expr == "tasks" || expr == "id = ?" {
			return expr, nil
		}
		return nil, fmt.Errorf("resolve error")
	}}

	config := map[string]any{
		"table":     "tasks",
		"condition": "id = ?",
		"params":    []any{"bad_expr"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.delete")
}

func TestExecNode_MissingService(t *testing.T) {
	exec := &execExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(t.Context(), nCtx, map[string]any{"query": "SELECT 1"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestCreateNode_MissingService(t *testing.T) {
	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(t.Context(), nCtx, map[string]any{"table": "t", "data": map[string]any{}}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestUpdateNode_MissingService(t *testing.T) {
	exec := &updateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(t.Context(), nCtx, map[string]any{"table": "t", "data": map[string]any{}, "condition": "1=1"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestDeleteNode_MissingService(t *testing.T) {
	exec := &deleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(t.Context(), nCtx, map[string]any{"table": "t", "condition": "1=1"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}
