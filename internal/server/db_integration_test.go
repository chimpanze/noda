package server

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	dbplugin "github.com/chimpanze/noda/plugins/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newDBTestServer creates a test server with an in-memory SQLite database registered as "main-db".
func newDBTestServer(t *testing.T, routes map[string]map[string]any, workflows map[string]map[string]any) (*Server, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	// Create tasks table
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

	// Register the database in the service registry
	svcReg := registry.NewServiceRegistry()
	err = svcReg.Register("main-db", db, &dbplugin.Plugin{})
	require.NoError(t, err)

	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Routes:    routes,
		Workflows: workflows,
		Schemas:   map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, buildTestNodeRegistry())
	require.NoError(t, err)
	require.NoError(t, srv.Setup())
	return srv, db
}

// --- E2E: POST /api/tasks → creates task in database, returns 201 ---

func TestE2E_DB_CreateTask(t *testing.T) {
	srv, db := newDBTestServer(t,
		map[string]map[string]any{
			"create-task": {
				"method": "POST",
				"path":   "/api/tasks",
				"trigger": map[string]any{
					"workflow": "create-task",
					"input": map[string]any{
						"title":       "{{ body.title }}",
						"description": "{{ body.description }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"create-task": {
				"nodes": map[string]any{
					"insert": map[string]any{
						"type": "db.create",
						"services": map[string]any{
							"database": "main-db",
						},
						"config": map[string]any{
							"table": "tasks",
							"data": map[string]any{
								"title":       "{{ input.title }}",
								"description": "{{ input.description }}",
								"status":      "pending",
							},
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "201",
							"body":   "{{ nodes.insert }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "insert", "to": "respond"},
				},
			},
		},
	)

	body := `{"title": "Buy groceries", "description": "Milk, eggs, bread"}`
	req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, "Buy groceries", result["title"])
	assert.Equal(t, "pending", result["status"])

	// Verify in database
	var count int64
	db.Raw("SELECT COUNT(*) FROM tasks").Scan(&count)
	assert.Equal(t, int64(1), count)
}

// --- E2E: GET /api/tasks → queries tasks with pagination ---

func TestE2E_DB_ListTasks(t *testing.T) {
	srv, db := newDBTestServer(t,
		map[string]map[string]any{
			"list-tasks": {
				"method": "GET",
				"path":   "/api/tasks",
				"trigger": map[string]any{
					"workflow": "list-tasks",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"list-tasks": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type": "db.query",
						"services": map[string]any{
							"database": "main-db",
						},
						"config": map[string]any{
							"query": "SELECT * FROM tasks ORDER BY id",
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ nodes.fetch }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "fetch", "to": "respond"},
				},
			},
		},
	)

	// Insert test data
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "Task 1", "pending")
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "Task 2", "done")

	req := httptest.NewRequest("GET", "/api/tasks", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result []map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Len(t, result, 2)
	assert.Equal(t, "Task 1", result[0]["title"])
	assert.Equal(t, "Task 2", result[1]["title"])
}

// --- E2E: GET /api/tasks/:id → returns task or 404 ---

func TestE2E_DB_GetTask(t *testing.T) {
	srv, db := newDBTestServer(t,
		map[string]map[string]any{
			"get-task": {
				"method": "GET",
				"path":   "/api/tasks/:id",
				"trigger": map[string]any{
					"workflow": "get-task",
					"input": map[string]any{
						"id": "{{ params.id }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"get-task": {
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type": "db.query",
						"services": map[string]any{
							"database": "main-db",
						},
						"config": map[string]any{
							"query":  "SELECT * FROM tasks WHERE id = ?",
							"params": []any{"{{ input.id }}"},
						},
					},
					"check_empty": map[string]any{
						"type": "control.if",
						"config": map[string]any{
							"condition": "{{ len(nodes.fetch) == 0 }}",
						},
					},
					"not_found": map[string]any{
						"type": "response.error",
						"config": map[string]any{
							"status":  "404",
							"code":    "NOT_FOUND",
							"message": "Task not found",
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ nodes.fetch[0] }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "fetch", "to": "check_empty"},
					map[string]any{"from": "check_empty", "to": "not_found", "output": "then"},
					map[string]any{"from": "check_empty", "to": "respond", "output": "else"},
				},
			},
		},
	)

	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "My Task", "pending")

	// Found case
	req := httptest.NewRequest("GET", "/api/tasks/1", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, "My Task", result["title"])

	// Not found case
	req = httptest.NewRequest("GET", "/api/tasks/999", nil)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 404, resp.StatusCode)
}

// --- E2E: PUT /api/tasks/:id → updates task ---

func TestE2E_DB_UpdateTask(t *testing.T) {
	srv, db := newDBTestServer(t,
		map[string]map[string]any{
			"update-task": {
				"method": "PUT",
				"path":   "/api/tasks/:id",
				"trigger": map[string]any{
					"workflow": "update-task",
					"input": map[string]any{
						"id":    "{{ params.id }}",
						"title": "{{ body.title }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"update-task": {
				"nodes": map[string]any{
					"update": map[string]any{
						"type": "db.update",
						"services": map[string]any{
							"database": "main-db",
						},
						"config": map[string]any{
							"table": "tasks",
							"data":  map[string]any{"title": "{{ input.title }}"},
							"where": map[string]any{"id": "{{ input.id }}"},
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ nodes.update }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "update", "to": "respond"},
				},
			},
		},
	)

	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "Old Title", "pending")

	body := `{"title": "New Title"}`
	req := httptest.NewRequest("PUT", "/api/tasks/1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	assert.Equal(t, float64(1), result["rows_affected"])

	// Verify in database
	var title string
	db.Raw("SELECT title FROM tasks WHERE id = 1").Scan(&title)
	assert.Equal(t, "New Title", title)
}

// --- E2E: DELETE /api/tasks/:id → deletes task ---

func TestE2E_DB_DeleteTask(t *testing.T) {
	srv, db := newDBTestServer(t,
		map[string]map[string]any{
			"delete-task": {
				"method": "DELETE",
				"path":   "/api/tasks/:id",
				"trigger": map[string]any{
					"workflow": "delete-task",
					"input": map[string]any{
						"id": "{{ params.id }}",
					},
				},
			},
		},
		map[string]map[string]any{
			"delete-task": {
				"nodes": map[string]any{
					"delete": map[string]any{
						"type": "db.delete",
						"services": map[string]any{
							"database": "main-db",
						},
						"config": map[string]any{
							"table": "tasks",
							"where": map[string]any{"id": "{{ input.id }}"},
						},
					},
					"respond": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": "200",
							"body":   "{{ nodes.delete }}",
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "delete", "to": "respond"},
				},
			},
		},
	)

	db.Exec("INSERT INTO tasks (title) VALUES (?)", "To Delete")
	db.Exec("INSERT INTO tasks (title) VALUES (?)", "To Keep")

	req := httptest.NewRequest("DELETE", "/api/tasks/1", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify only one task remains
	var count int64
	db.Raw("SELECT COUNT(*) FROM tasks").Scan(&count)
	assert.Equal(t, int64(1), count)
}

// --- E2E: No response node → 202 Accepted ---

func TestE2E_DB_NoResponseNode(t *testing.T) {
	srv, _ := newDBTestServer(t,
		map[string]map[string]any{
			"fire-forget": {
				"method": "POST",
				"path":   "/api/fire",
				"trigger": map[string]any{
					"workflow": "fire-forget",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"fire-forget": {
				"nodes": map[string]any{
					"insert": map[string]any{
						"type": "db.exec",
						"services": map[string]any{
							"database": "main-db",
						},
						"config": map[string]any{
							"query": "INSERT INTO tasks (title) VALUES ('background')",
						},
					},
				},
				"edges": []any{},
			},
		},
	)

	req := httptest.NewRequest("POST", "/api/fire", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 202, resp.StatusCode)
}

// --- E2E: Workflow error → standardized error response ---

func TestE2E_DB_WorkflowError(t *testing.T) {
	// Use a response.error node to explicitly return an error response
	// This tests the error path through the workflow
	srv, _ := newDBTestServer(t,
		map[string]map[string]any{
			"error-route": {
				"method": "GET",
				"path":   "/api/fail",
				"trigger": map[string]any{
					"workflow": "fail-wf",
					"input":    map[string]any{},
				},
			},
		},
		map[string]map[string]any{
			"fail-wf": {
				"nodes": map[string]any{
					"error_resp": map[string]any{
						"type": "response.error",
						"config": map[string]any{
							"status":  "500",
							"code":    "INTERNAL_ERROR",
							"message": "Something went wrong",
						},
					},
				},
				"edges": []any{},
			},
		},
	)

	req := httptest.NewRequest("GET", "/api/fail", nil)
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 500, resp.StatusCode)

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))
	errorObj, ok := result["error"].(map[string]any)
	require.True(t, ok, "expected error object in response, got: %s", string(respBody))
	assert.Equal(t, "INTERNAL_ERROR", errorObj["code"])
	assert.Equal(t, "Something went wrong", errorObj["message"])
}

// --- E2E: Full CRUD walkthrough ---

func TestE2E_DB_FullCRUDWalkthrough(t *testing.T) {
	routes := map[string]map[string]any{
		"create-task": {
			"method": "POST",
			"path":   "/api/tasks",
			"trigger": map[string]any{
				"workflow": "create-task",
				"input": map[string]any{
					"title": "{{ body.title }}",
				},
			},
		},
		"list-tasks": {
			"method": "GET",
			"path":   "/api/tasks",
			"trigger": map[string]any{
				"workflow": "list-tasks",
				"input":    map[string]any{},
			},
		},
		"get-task": {
			"method": "GET",
			"path":   "/api/tasks/:id",
			"trigger": map[string]any{
				"workflow": "get-task",
				"input": map[string]any{
					"id": "{{ params.id }}",
				},
			},
		},
		"update-task": {
			"method": "PUT",
			"path":   "/api/tasks/:id",
			"trigger": map[string]any{
				"workflow": "update-task",
				"input": map[string]any{
					"id":    "{{ params.id }}",
					"title": "{{ body.title }}",
				},
			},
		},
		"delete-task": {
			"method": "DELETE",
			"path":   "/api/tasks/:id",
			"trigger": map[string]any{
				"workflow": "delete-task",
				"input": map[string]any{
					"id": "{{ params.id }}",
				},
			},
		},
	}

	workflows := map[string]map[string]any{
		"create-task": {
			"nodes": map[string]any{
				"insert": map[string]any{
					"type":     "db.create",
					"services": map[string]any{"database": "main-db"},
					"config": map[string]any{
						"table": "tasks",
						"data":  map[string]any{"title": "{{ input.title }}", "status": "pending"},
					},
				},
				"respond": map[string]any{
					"type":   "response.json",
					"config": map[string]any{"status": "201", "body": "{{ nodes.insert }}"},
				},
			},
			"edges": []any{map[string]any{"from": "insert", "to": "respond"}},
		},
		"list-tasks": {
			"nodes": map[string]any{
				"fetch": map[string]any{
					"type":     "db.query",
					"services": map[string]any{"database": "main-db"},
					"config":   map[string]any{"query": "SELECT * FROM tasks ORDER BY id"},
				},
				"respond": map[string]any{
					"type":   "response.json",
					"config": map[string]any{"status": "200", "body": "{{ nodes.fetch }}"},
				},
			},
			"edges": []any{map[string]any{"from": "fetch", "to": "respond"}},
		},
		"get-task": {
			"nodes": map[string]any{
				"fetch": map[string]any{
					"type":     "db.query",
					"services": map[string]any{"database": "main-db"},
					"config": map[string]any{
						"query":  "SELECT * FROM tasks WHERE id = ?",
						"params": []any{"{{ input.id }}"},
					},
				},
				"respond": map[string]any{
					"type":   "response.json",
					"config": map[string]any{"status": "200", "body": "{{ nodes.fetch }}"},
				},
			},
			"edges": []any{map[string]any{"from": "fetch", "to": "respond"}},
		},
		"update-task": {
			"nodes": map[string]any{
				"update": map[string]any{
					"type":     "db.update",
					"services": map[string]any{"database": "main-db"},
					"config": map[string]any{
						"table": "tasks", "data": map[string]any{"title": "{{ input.title }}"},
						"where": map[string]any{"id": "{{ input.id }}"},
					},
				},
				"respond": map[string]any{
					"type":   "response.json",
					"config": map[string]any{"status": "200", "body": "{{ nodes.update }}"},
				},
			},
			"edges": []any{map[string]any{"from": "update", "to": "respond"}},
		},
		"delete-task": {
			"nodes": map[string]any{
				"delete": map[string]any{
					"type":     "db.delete",
					"services": map[string]any{"database": "main-db"},
					"config": map[string]any{
						"table": "tasks", "where": map[string]any{"id": "{{ input.id }}"},
					},
				},
				"respond": map[string]any{
					"type":   "response.json",
					"config": map[string]any{"status": "200", "body": "{{ nodes.delete }}"},
				},
			},
			"edges": []any{map[string]any{"from": "delete", "to": "respond"}},
		},
	}

	srv, _ := newDBTestServer(t, routes, workflows)

	// 1. Create a task
	req := httptest.NewRequest("POST", "/api/tasks", strings.NewReader(`{"title": "Test Task"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	var created map[string]any
	_ = json.Unmarshal(body, &created)
	assert.Equal(t, "Test Task", created["title"])

	// 2. List tasks
	req = httptest.NewRequest("GET", "/api/tasks", nil)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	var list []map[string]any
	_ = json.Unmarshal(body, &list)
	assert.Len(t, list, 1)

	// 3. Get single task
	req = httptest.NewRequest("GET", "/api/tasks/1", nil)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 4. Update task
	req = httptest.NewRequest("PUT", "/api/tasks/1", strings.NewReader(`{"title": "Updated Task"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, _ = io.ReadAll(resp.Body)
	var updated map[string]any
	_ = json.Unmarshal(body, &updated)
	assert.Equal(t, float64(1), updated["rows_affected"])

	// 5. Delete task
	req = httptest.NewRequest("DELETE", "/api/tasks/1", nil)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// 6. Verify empty list
	req = httptest.NewRequest("GET", "/api/tasks", nil)
	resp, err = srv.App().Test(req)
	require.NoError(t, err)
	body, _ = io.ReadAll(resp.Body)
	var emptyList []map[string]any
	_ = json.Unmarshal(body, &emptyList)
	assert.Empty(t, emptyList)
}
