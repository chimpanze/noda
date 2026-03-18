package db

import (
	"testing"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/clause"
)

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
		{"find", &findDescriptor{}},
		{"findOne", &findOneDescriptor{}},
		{"count", &countDescriptor{}},
		{"upsert", &upsertDescriptor{}},
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
		{"find", func(c map[string]any) interface{ Outputs() []string } { return newFindExecutor(c).(*findExecutor) }},
		{"findOne", func(c map[string]any) interface{ Outputs() []string } {
			return newFindOneExecutor(c).(*findOneExecutor)
		}},
		{"count", func(c map[string]any) interface{ Outputs() []string } { return newCountExecutor(c).(*countExecutor) }},
		{"upsert", func(c map[string]any) interface{ Outputs() []string } { return newUpsertExecutor(c).(*upsertExecutor) }},
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

	_, _, err := exec.Execute(t.Context(), nCtx, map[string]any{}, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.query")
}

func TestExecNode_MissingQueryField(t *testing.T) {
	db := newTestDB(t)
	exec := &execExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(t.Context(), nCtx, map[string]any{}, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.exec")
}

func TestCreateNode_MissingTableField(t *testing.T) {
	db := newTestDB(t)
	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{"data": map[string]any{"title": "test"}}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.create")
}

func TestCreateNode_MissingDataField(t *testing.T) {
	db := newTestDB(t)
	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{"table": "tasks"}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.create")
}

func TestUpdateNode_MissingTableField(t *testing.T) {
	db := newTestDB(t)
	exec := &updateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"data":  map[string]any{"status": "done"},
		"where": map[string]any{"id": "1"},
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
		"table": "tasks",
		"where": map[string]any{"id": "1"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.update")
}

func TestUpdateNode_MissingWhereField(t *testing.T) {
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
		"where": map[string]any{"id": "1"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.delete")
}

func TestDeleteNode_MissingWhereField(t *testing.T) {
	db := newTestDB(t)
	exec := &deleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{"table": "tasks"}
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
		return nil, errResolve
	}}

	config := map[string]any{"query": "SELECT 1", "params": []any{"bad"}}
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
		return nil, errResolve
	}}

	config := map[string]any{"query": "DELETE FROM tasks", "params": []any{"bad"}}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.exec")
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
	_, _, err := exec.Execute(t.Context(), nCtx, map[string]any{"table": "t", "data": map[string]any{}, "where": map[string]any{"id": "1"}}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestDeleteNode_MissingService(t *testing.T) {
	exec := &deleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, _, err := exec.Execute(t.Context(), nCtx, map[string]any{"table": "t", "where": map[string]any{"id": "1"}}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestGetDB_MissingService(t *testing.T) {
	_, err := plugin.GetService[any](map[string]any{}, "database")
	require.Error(t, err)
}

// --- Descriptor Description and OutputDescriptions coverage ---

func TestDescriptors_Description(t *testing.T) {
	descriptors := []struct {
		name string
		desc interface{ Description() string }
	}{
		{"query", &queryDescriptor{}},
		{"exec", &execDescriptor{}},
		{"create", &createDescriptor{}},
		{"update", &updateDescriptor{}},
		{"delete", &deleteDescriptor{}},
		{"find", &findDescriptor{}},
		{"findOne", &findOneDescriptor{}},
		{"count", &countDescriptor{}},
		{"upsert", &upsertDescriptor{}},
	}
	for _, tt := range descriptors {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.desc.Description())
		})
	}
}

func TestDescriptors_OutputDescriptions(t *testing.T) {
	descriptors := []struct {
		name string
		desc interface{ OutputDescriptions() map[string]string }
	}{
		{"query", &queryDescriptor{}},
		{"exec", &execDescriptor{}},
		{"create", &createDescriptor{}},
		{"update", &updateDescriptor{}},
		{"delete", &deleteDescriptor{}},
		{"find", &findDescriptor{}},
		{"findOne", &findOneDescriptor{}},
		{"count", &countDescriptor{}},
		{"upsert", &upsertDescriptor{}},
	}
	for _, tt := range descriptors {
		t.Run(tt.name, func(t *testing.T) {
			od := tt.desc.OutputDescriptions()
			assert.NotEmpty(t, od)
			assert.Contains(t, od, "success")
			assert.Contains(t, od, "error")
		})
	}
}

// --- Invalid table identifier tests ---

func TestCreateNode_InvalidTableName(t *testing.T) {
	db := newTestDB(t)
	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "1invalid",
		"data":  map[string]any{"title": "test"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

func TestUpdateNode_InvalidTableName(t *testing.T) {
	db := newTestDB(t)
	exec := &updateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "my table",
		"data":  map[string]any{"status": "done"},
		"where": map[string]any{"id": "1"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

func TestDeleteNode_InvalidTableName(t *testing.T) {
	db := newTestDB(t)
	exec := &deleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "drop;table",
		"where": map[string]any{"id": "1"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

func TestCountNode_InvalidTableName(t *testing.T) {
	db := newTestDB(t)
	exec := &countExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{"table": "1bad"}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

func TestFindNode_InvalidTableName(t *testing.T) {
	db := newTestDB(t)
	exec := &findExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{"table": "1bad"}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

func TestFindOneNode_InvalidTableName(t *testing.T) {
	db := newTestDB(t)
	exec := &findOneExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{"table": "1bad"}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

func TestUpsertNode_InvalidTableName(t *testing.T) {
	db := newTestDB(t)
	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":    "1bad",
		"data":     map[string]any{"title": "T"},
		"conflict": "id",
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

// --- Upsert resolveConflictColumns / resolveUpdateSpec coverage ---

func TestResolveConflictColumns_InvalidItemType(t *testing.T) {
	config := map[string]any{"conflict": []any{"id", 123}}
	_, err := resolveConflictColumns(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestResolveConflictColumns_InvalidType(t *testing.T) {
	config := map[string]any{"conflict": 123}
	_, err := resolveConflictColumns(config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string or array")
}

func TestResolveUpdateSpec_MapFormat(t *testing.T) {
	config := map[string]any{
		"update": map[string]any{
			"status": "active",
		},
	}
	onConflict := &clause.OnConflict{}
	err := resolveUpdateSpec(config, map[string]any{"title": "T", "status": "old"}, nil, onConflict)
	require.NoError(t, err)
	assert.NotEmpty(t, onConflict.DoUpdates)
}

func TestResolveUpdateSpec_InvalidType(t *testing.T) {
	config := map[string]any{"update": 123}
	onConflict := &clause.OnConflict{}
	err := resolveUpdateSpec(config, map[string]any{}, nil, onConflict)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an array or object")
}

func TestResolveUpdateSpec_ArrayInvalidItem(t *testing.T) {
	config := map[string]any{"update": []any{"status", 123}}
	onConflict := &clause.OnConflict{}
	err := resolveUpdateSpec(config, map[string]any{}, nil, onConflict)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestUpsertNode_InvalidConflictColumn(t *testing.T) {
	db := newTestDB(t)
	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	// String conflict with invalid identifier
	config := map[string]any{
		"table":    "tasks",
		"data":     map[string]any{"title": "T"},
		"conflict": "bad;col",
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")

	// Array conflict with invalid identifier
	config["conflict"] = []any{"id", "bad;col"}
	_, _, err = exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

func TestUpsertNode_InvalidUpdateColumn(t *testing.T) {
	db := newTestDB(t)
	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	// Array update with invalid identifier
	config := map[string]any{
		"table":    "tasks",
		"data":     map[string]any{"title": "T"},
		"conflict": "id",
		"update":   []any{"bad;col"},
	}
	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")

	// Map update with invalid identifier key
	config["update"] = map[string]any{"bad;col": "value"}
	_, _, err = exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

func TestResolveUpdateSpec_DefaultAllNonConflict(t *testing.T) {
	config := map[string]any{}
	onConflict := &clause.OnConflict{}
	cols := []clause.Column{{Name: "id"}}
	err := resolveUpdateSpec(config, map[string]any{"id": 1, "name": "test", "status": "active"}, cols, onConflict)
	require.NoError(t, err)
	assert.NotEmpty(t, onConflict.DoUpdates)
}
