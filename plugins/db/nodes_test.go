package db

import (
	"context"
	"errors"
	"fmt"
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

var errResolve = fmt.Errorf("resolve error")

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
	cancel()

	config := map[string]any{"query": "SELECT * FROM tasks"}
	_, _, _ = exec.Execute(ctx, nCtx, config, testServices(db))
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
		"data":  map[string]any{"title": "Minimal Task"},
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	var results []map[string]any
	db.Raw("SELECT description FROM tasks WHERE title = ?", "Minimal Task").Scan(&results)
	require.Len(t, results, 1)
	assert.Nil(t, results[0]["description"])
}

func TestCreateNode_UniqueConstraintViolation(t *testing.T) {
	db := newTestDB(t)
	sqlDB, _ := db.DB()
	_, _ = sqlDB.Exec("CREATE UNIQUE INDEX idx_tasks_title ON tasks(title)")
	db.Exec("INSERT INTO tasks (title) VALUES (?)", "Existing")

	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"data":  map[string]any{"title": "Existing"},
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.Error(t, err)
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
		"table": "tasks",
		"data":  map[string]any{"status": "done"},
		"where": map[string]any{"user_id": "u1"},
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
		"table": "tasks",
		"data":  map[string]any{"status": "done"},
		"where": map[string]any{"user_id": "nonexistent"},
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
		"table": "tasks",
		"where": map[string]any{"user_id": "u1"},
	}

	output, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, int64(2), result["rows_affected"])

	var count int64
	db.Raw("SELECT COUNT(*) FROM tasks").Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestDeleteNode_NoMatchingRows(t *testing.T) {
	db := newTestDB(t)

	exec := &deleteExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"where": map[string]any{"user_id": "nonexistent"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	result := data.(map[string]any)
	assert.Equal(t, int64(0), result["rows_affected"])
}

// --- db.find tests ---

func TestFindNode_BasicSelect(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T1", "u1")
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T2", "u1")

	exec := &findExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{"table": "tasks"}

	output, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	rows := data.([]map[string]any)
	assert.Len(t, rows, 2)
}

func TestFindNode_WithWhere(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T1", "u1")
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T2", "u2")

	exec := &findExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"where": map[string]any{"user_id": "u1"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	rows := data.([]map[string]any)
	require.Len(t, rows, 1)
	assert.Equal(t, "T1", rows[0]["title"])
}

func TestFindNode_WithWhereClause(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T1", "u1")
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T2", "u2")

	exec := &findExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"where_clause": map[string]any{
			"query":  "user_id = ?",
			"params": []any{"u2"},
		},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	rows := data.([]map[string]any)
	require.Len(t, rows, 1)
	assert.Equal(t, "T2", rows[0]["title"])
}

func TestFindNode_WithSelectColumns(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, status, user_id) VALUES (?, ?, ?)", "T1", "active", "u1")

	exec := &findExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":  "tasks",
		"select": []any{"title", "status"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	rows := data.([]map[string]any)
	require.Len(t, rows, 1)
	assert.Equal(t, "T1", rows[0]["title"])
	assert.Equal(t, "active", rows[0]["status"])
}

func TestFindNode_WithOrderLimitOffset(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title) VALUES (?)", "A")
	db.Exec("INSERT INTO tasks (title) VALUES (?)", "B")
	db.Exec("INSERT INTO tasks (title) VALUES (?)", "C")

	exec := &findExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":  "tasks",
		"select": []any{"title"},
		"order":  "title ASC",
		"limit":  float64(2),
		"offset": float64(1),
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	rows := data.([]map[string]any)
	require.Len(t, rows, 2)
	assert.Equal(t, "B", rows[0]["title"])
	assert.Equal(t, "C", rows[1]["title"])
}

func TestFindNode_EmptyResult(t *testing.T) {
	db := newTestDB(t)

	exec := &findExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"where": map[string]any{"user_id": "nonexistent"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	rows := data.([]map[string]any)
	assert.Empty(t, rows)
}

func TestFindNode_MissingTable(t *testing.T) {
	db := newTestDB(t)
	exec := &findExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx, map[string]any{}, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.find")
}

func TestFindNode_MissingService(t *testing.T) {
	exec := &findExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx, map[string]any{"table": "t"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

// --- db.findOne tests ---

func TestFindOneNode_ReturnsOneRow(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T1", "u1")

	exec := &findOneExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"where": map[string]any{"user_id": "u1"},
	}

	output, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	row := data.(map[string]any)
	assert.Equal(t, "T1", row["title"])
}

func TestFindOneNode_NotFoundRequired(t *testing.T) {
	db := newTestDB(t)

	exec := &findOneExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"where": map[string]any{"user_id": "nonexistent"},
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.Error(t, err)

	var notFound *api.NotFoundError
	assert.True(t, errors.As(err, &notFound))
	assert.Equal(t, "tasks", notFound.Resource)
}

func TestFindOneNode_NotFoundOptional(t *testing.T) {
	db := newTestDB(t)

	exec := &findOneExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":    "tasks",
		"where":    map[string]any{"user_id": "nonexistent"},
		"required": false,
	}

	output, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Nil(t, data)
}

func TestFindOneNode_WithWhere(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T1", "u1")
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T2", "u2")

	exec := &findOneExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"where": map[string]any{"user_id": "u2"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	row := data.(map[string]any)
	assert.Equal(t, "T2", row["title"])
}

func TestFindOneNode_MissingTable(t *testing.T) {
	db := newTestDB(t)
	exec := &findOneExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx, map[string]any{}, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.findOne")
}

func TestFindOneNode_MissingService(t *testing.T) {
	exec := &findOneExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx, map[string]any{"table": "t"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

// --- db.count tests ---

func TestCountNode_BasicCount(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title) VALUES (?)", "T1")
	db.Exec("INSERT INTO tasks (title) VALUES (?)", "T2")
	db.Exec("INSERT INTO tasks (title) VALUES (?)", "T3")

	exec := &countExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{"table": "tasks"}

	output, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, int64(3), result["count"])
}

func TestCountNode_WithWhere(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T1", "active")
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T2", "done")
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T3", "active")

	exec := &countExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"where": map[string]any{"status": "active"},
	}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	result := data.(map[string]any)
	assert.Equal(t, int64(2), result["count"])
}

func TestCountNode_EmptyTable(t *testing.T) {
	db := newTestDB(t)

	exec := &countExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{"table": "tasks"}

	_, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	result := data.(map[string]any)
	assert.Equal(t, int64(0), result["count"])
}

func TestCountNode_MissingTable(t *testing.T) {
	db := newTestDB(t)
	exec := &countExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx, map[string]any{}, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.count")
}

func TestCountNode_MissingService(t *testing.T) {
	exec := &countExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx, map[string]any{"table": "t"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

// --- db.upsert tests ---

func TestUpsertNode_InsertNew(t *testing.T) {
	db := newTestDB(t)
	// Create a table with unique constraint for upsert
	sqlDB, _ := db.DB()
	_, _ = sqlDB.Exec("CREATE UNIQUE INDEX idx_tasks_title ON tasks(title)")

	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":    "tasks",
		"data":     map[string]any{"title": "New", "status": "active"},
		"conflict": []any{"title"},
		"update":   []any{"status"},
	}

	output, data, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	record := data.(map[string]any)
	assert.Equal(t, "New", record["title"])

	var count int64
	db.Raw("SELECT COUNT(*) FROM tasks").Scan(&count)
	assert.Equal(t, int64(1), count)
}

func TestUpsertNode_UpdateExisting(t *testing.T) {
	db := newTestDB(t)
	sqlDB, _ := db.DB()
	_, _ = sqlDB.Exec("CREATE UNIQUE INDEX idx_tasks_title ON tasks(title)")
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "Existing", "old")

	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":    "tasks",
		"data":     map[string]any{"title": "Existing", "status": "new"},
		"conflict": []any{"title"},
		"update":   []any{"status"},
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	var results []map[string]any
	db.Raw("SELECT status FROM tasks WHERE title = ?", "Existing").Scan(&results)
	require.Len(t, results, 1)
	assert.Equal(t, "new", results[0]["status"])
}

func TestUpsertNode_UpdateSpecificColumns(t *testing.T) {
	db := newTestDB(t)
	sqlDB, _ := db.DB()
	_, _ = sqlDB.Exec("CREATE UNIQUE INDEX idx_tasks_title ON tasks(title)")
	db.Exec("INSERT INTO tasks (title, status, description) VALUES (?, ?, ?)", "Task", "old", "old desc")

	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":    "tasks",
		"data":     map[string]any{"title": "Task", "status": "new", "description": "new desc"},
		"conflict": []any{"title"},
		"update":   []any{"status"},
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)

	var results []map[string]any
	db.Raw("SELECT status, description FROM tasks WHERE title = ?", "Task").Scan(&results)
	require.Len(t, results, 1)
	assert.Equal(t, "new", results[0]["status"])
	// description should remain unchanged since only "status" is in update list
	assert.Equal(t, "old desc", results[0]["description"])
}

func TestUpsertNode_MissingTable(t *testing.T) {
	db := newTestDB(t)
	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"data":     map[string]any{"title": "T"},
		"conflict": []any{"title"},
	}
	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.upsert")
}

func TestUpsertNode_MissingData(t *testing.T) {
	db := newTestDB(t)
	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":    "tasks",
		"conflict": []any{"title"},
	}
	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.upsert")
}

func TestUpsertNode_MissingConflict(t *testing.T) {
	db := newTestDB(t)
	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table": "tasks",
		"data":  map[string]any{"title": "T"},
	}
	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db.upsert")
}

func TestUpsertNode_MissingService(t *testing.T) {
	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx, map[string]any{"table": "t", "data": map[string]any{}, "conflict": "id"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestUpsertNode_StringConflict(t *testing.T) {
	db := newTestDB(t)
	sqlDB, _ := db.DB()
	_, _ = sqlDB.Exec("CREATE UNIQUE INDEX idx_tasks_title ON tasks(title)")

	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	config := map[string]any{
		"table":    "tasks",
		"data":     map[string]any{"title": "Single", "status": "active"},
		"conflict": "title",
		"update":   []any{"status"},
	}

	_, _, err := exec.Execute(context.Background(), nCtx, config, testServices(db))
	require.NoError(t, err)
}

// --- helpers tests ---

func TestGetDB_WrongType(t *testing.T) {
	_, err := plugin.GetService[*gorm.DB](map[string]any{"database": "not a db"}, "database")
	require.Error(t, err)
}

func TestResolveString_Missing(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := plugin.ResolveString(nCtx, map[string]any{}, "field")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestDB_ConcurrentQueries(t *testing.T) {
	// Use shared-cache in-memory SQLite so all connections see the same data.
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })

	_, err = sqlDB.Exec(`CREATE TABLE tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT NOT NULL,
		description TEXT,
		status TEXT DEFAULT 'pending',
		user_id TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	require.NoError(t, err)

	// Constrain pool to expose contention
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetMaxIdleConns(2)

	// Seed data
	for i := range 20 {
		db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", fmt.Sprintf("task-%d", i), "user1")
	}

	exec := &queryExecutor{}
	config := map[string]any{
		"query":  "SELECT title FROM tasks WHERE user_id = ?",
		"params": []any{"user1"},
	}
	services := testServices(db)
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	// Run 100 concurrent queries
	const goroutines = 100
	errs := make(chan error, goroutines)
	for range goroutines {
		go func() {
			_, _, err := exec.Execute(context.Background(), nCtx, config, services)
			errs <- err
		}()
	}

	for range goroutines {
		err := <-errs
		assert.NoError(t, err)
	}
}
