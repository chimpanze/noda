package db

import (
	"context"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// mockExecCtx implements api.ExecutionContext for testing.
type mockExecCtx struct {
	resolveFunc func(expr string) (any, error)
}

func (m *mockExecCtx) Input() any          { return nil }
func (m *mockExecCtx) Auth() *api.AuthData { return nil }
func (m *mockExecCtx) Trigger() api.TriggerData {
	return api.TriggerData{Type: "test", Timestamp: time.Now(), TraceID: "test-trace"}
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

// newTestDB creates an in-memory SQLite database with a test table.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Create test table
	sqlDB, _ := db.DB()
	_, err = sqlDB.Exec(`CREATE TABLE tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		description TEXT,
		status TEXT DEFAULT 'pending',
		user_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)
	return db
}

func testServices(db *gorm.DB) map[string]any {
	return map[string]any{"database": db}
}

// identityResolve returns the expression value as-is (passthrough).
func identityResolve(expr string) (any, error) {
	return expr, nil
}

// --- db.query tests ---

func TestQueryNode_SelectRows(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "Task 1", "user1")
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "Task 2", "user1")

	exec := &queryExecutor{}
	ctx := context.Background()
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"query":  "SELECT title FROM tasks WHERE user_id = ?",
		"params": []any{"user1"},
	}

	output, data, err := exec.Execute(ctx, nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	rows, ok := data.([]map[string]any)
	require.True(t, ok)
	assert.Len(t, rows, 2)
}

func TestQueryNode_ParameterizedQuery(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "Alpha", "u1")
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "Beta", "u2")

	exec := &queryExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"query":  "SELECT title FROM tasks WHERE user_id = ?",
		"params": []any{"u2"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	rows := data.([]map[string]any)
	require.Len(t, rows, 1)
	assert.Equal(t, "Beta", rows[0]["title"])
}

func TestQueryNode_EmptyResult(t *testing.T) {
	db := newTestDB(t)

	exec := &queryExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"query": "SELECT * FROM tasks WHERE user_id = 'nonexistent'",
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	rows := data.([]map[string]any)
	assert.Empty(t, rows)
}

func TestQueryNode_SQLError(t *testing.T) {
	db := newTestDB(t)

	exec := &queryExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"query": "SELECT * FROM nonexistent_table",
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.query")
}

func TestQueryNode_ContextCancellation(t *testing.T) {
	db := newTestDB(t)

	exec := &queryExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	config := map[string]any{
		"query": "SELECT * FROM tasks",
	}

	_, _, err := exec.Execute(ctx, nCtx, config, testServices(db))
	// SQLite may or may not respect context cancellation, but it shouldn't panic
	_ = err
}

func TestQueryNode_MissingService(t *testing.T) {
	exec := &queryExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx, map[string]any{"query": "SELECT 1"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

// --- db.exec tests ---

func TestExecNode_Insert(t *testing.T) {
	db := newTestDB(t)

	exec := &execExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"query":  "INSERT INTO tasks (title, user_id) VALUES (?, ?)",
		"params": []any{"New Task", "user1"},
	}

	output, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, int64(1), result["rows_affected"])
}

func TestExecNode_UpdateMultiple(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T1", "pending")
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T2", "pending")
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T3", "done")

	exec := &execExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"query":  "UPDATE tasks SET status = ? WHERE status = ?",
		"params": []any{"completed", "pending"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	result := data.(map[string]any)
	assert.Equal(t, int64(2), result["rows_affected"])
}

func TestExecNode_Delete(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title) VALUES (?)", "To Delete")

	exec := &execExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"query":  "DELETE FROM tasks WHERE title = ?",
		"params": []any{"To Delete"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	result := data.(map[string]any)
	assert.Equal(t, int64(1), result["rows_affected"])
}

func TestExecNode_SQLError(t *testing.T) {
	db := newTestDB(t)

	exec := &execExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"query": "INSERT INTO nonexistent (x) VALUES (1)",
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.Error(t, err)
}

// --- db.create tests ---

func TestCreateNode_InsertRecord(t *testing.T) {
	db := newTestDB(t)

	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"data": map[string]any{
			"title":   "New Task",
			"user_id": "user1",
		},
	}

	output, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	record := data.(map[string]any)
	assert.Equal(t, "New Task", record["title"])
	assert.Equal(t, "user1", record["user_id"])

	// Verify in database
	var count int64
	db.Raw("SELECT COUNT(*) FROM tasks").Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestCreateNode_MultipleFields(t *testing.T) {
	db := newTestDB(t)

	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"data": map[string]any{
			"title":       "Full Task",
			"description": "A task with all fields",
			"status":      "active",
			"user_id":     "user2",
		},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	record := data.(map[string]any)
	assert.Equal(t, "Full Task", record["title"])
	assert.Equal(t, "active", record["status"])
}

func TestCreateNode_NullFields(t *testing.T) {
	db := newTestDB(t)

	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"data": map[string]any{
			"title": "Minimal Task",
		},
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	// Verify description is NULL
	var results []map[string]any
	db.Raw("SELECT description FROM tasks WHERE title = ?", "Minimal Task").Scan(&results)
	require.Len(t, results, 1)
	assert.Nil(t, results[0]["description"])
}

func TestCreateNode_UniqueConstraintViolation(t *testing.T) {
	db := newTestDB(t)
	// Add unique constraint
	sqlDB, _ := db.DB()
	sqlDB.Exec("CREATE UNIQUE INDEX idx_tasks_title ON tasks(title)")
	db.Exec("INSERT INTO tasks (title) VALUES (?)", "Existing")

	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"data": map[string]any{
			"title": "Existing",
		},
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.Error(t, err)
	// SQLite uses "UNIQUE constraint failed" message
	assert.Contains(t, err.Error(), "constraint")
}

// --- db.update tests ---

func TestUpdateNode_UpdateRows(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, status, user_id) VALUES (?, ?, ?)", "T1", "pending", "u1")
	db.Exec("INSERT INTO tasks (title, status, user_id) VALUES (?, ?, ?)", "T2", "pending", "u1")

	exec := &updateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":     "tasks",
		"data":      map[string]any{"status": "done"},
		"condition": "user_id = ?",
		"params":    []any{"u1"},
	}

	output, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, int64(2), result["rows_affected"])
}

func TestUpdateNode_NoMatchingRows(t *testing.T) {
	db := newTestDB(t)

	exec := &updateExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":     "tasks",
		"data":      map[string]any{"status": "done"},
		"condition": "user_id = ?",
		"params":    []any{"nonexistent"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	result := data.(map[string]any)
	assert.Equal(t, int64(0), result["rows_affected"])
}

// --- db.delete tests ---

func TestDeleteNode_DeleteRows(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T1", "u1")
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T2", "u1")
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T3", "u2")

	exec := &deleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":     "tasks",
		"condition": "user_id = ?",
		"params":    []any{"u1"},
	}

	output, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, int64(2), result["rows_affected"])

	// Verify u2's task still exists
	var count int64
	db.Raw("SELECT COUNT(*) FROM tasks").Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestDeleteNode_NoMatchingRows(t *testing.T) {
	db := newTestDB(t)

	exec := &deleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":     "tasks",
		"condition": "user_id = ?",
		"params":    []any{"nonexistent"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	result := data.(map[string]any)
	assert.Equal(t, int64(0), result["rows_affected"])
}

// --- helpers tests ---

func TestGetDB_MissingService(t *testing.T) {
	_, err := getDB(map[string]any{})
	require.Error(t, err)
}

func TestGetDB_WrongType(t *testing.T) {
	_, err := getDB(map[string]any{"database": "not a db"})
	require.Error(t, err)
}

func TestResolveString_Missing(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := plugin.ResolveString(nCtx, map[string]any{}, "field")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestResolveMap_AsExpression(t *testing.T) {
	expected := map[string]any{"key": "value"}
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return expected, nil
	}}

	result, err := resolveMap(nCtx, map[string]any{"data": "{{ some_expr }}"}, "data")
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestResolveParams_AsExpression(t *testing.T) {
	expected := []any{"a", "b"}
	nCtx := &mockExecCtx{resolveFunc: func(_ string) (any, error) {
		return expected, nil
	}}

	result, err := resolveParams(nCtx, map[string]any{"params": "{{ expr }}"})
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}
