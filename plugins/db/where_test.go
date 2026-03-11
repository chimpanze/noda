package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- resolveWhereClause tests ---

func TestResolveWhereClause_Absent(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	wc, err := resolveWhereClause(nCtx, map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, wc)
}

func TestResolveWhereClause_Valid(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"where_clause": map[string]any{
			"query":  "created_at > ?",
			"params": []any{"2024-01-01"},
		},
	}
	wc, err := resolveWhereClause(nCtx, config)
	require.NoError(t, err)
	require.NotNil(t, wc)
	assert.Equal(t, "created_at > ?", wc.Query)
	assert.Equal(t, []any{"2024-01-01"}, wc.Params)
}

func TestResolveWhereClause_MissingQuery(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"where_clause": map[string]any{
			"params": []any{"val"},
		},
	}
	_, err := resolveWhereClause(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestResolveWhereClause_InvalidType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{"where_clause": "not an object"}
	_, err := resolveWhereClause(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an object")
}

func TestResolveWhereClause_NoParams(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"where_clause": map[string]any{"query": "active = true"},
	}
	wc, err := resolveWhereClause(nCtx, config)
	require.NoError(t, err)
	assert.Equal(t, "active = true", wc.Query)
	assert.Nil(t, wc.Params)
}

// --- resolveJoins tests ---

func TestResolveJoins_Absent(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	joins, err := resolveJoins(nCtx, map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, joins)
}

func TestResolveJoins_Single(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{
				"type":  "LEFT",
				"table": "users",
				"on":    "tasks.user_id = users.id",
			},
		},
	}
	joins, err := resolveJoins(nCtx, config)
	require.NoError(t, err)
	require.Len(t, joins, 1)
	assert.Equal(t, "LEFT JOIN users ON tasks.user_id = users.id", joins[0].Query)
}

func TestResolveJoins_DefaultType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{
				"table": "users",
				"on":    "tasks.user_id = users.id",
			},
		},
	}
	joins, err := resolveJoins(nCtx, config)
	require.NoError(t, err)
	assert.Equal(t, "INNER JOIN users ON tasks.user_id = users.id", joins[0].Query)
}

func TestResolveJoins_Multiple(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{"type": "LEFT", "table": "users", "on": "t.uid = u.id"},
			map[string]any{"type": "RIGHT", "table": "roles", "on": "u.rid = r.id"},
		},
	}
	joins, err := resolveJoins(nCtx, config)
	require.NoError(t, err)
	require.Len(t, joins, 2)
}

func TestResolveJoins_MissingTable(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{"on": "a = b"},
		},
	}
	_, err := resolveJoins(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestResolveJoins_MissingOn(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{"table": "users"},
		},
	}
	_, err := resolveJoins(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestResolveJoins_InvalidType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{"joins": "not an array"}
	_, err := resolveJoins(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an array")
}

// --- resolveSelectColumns tests ---

func TestResolveSelectColumns_Absent(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	cols, err := resolveSelectColumns(nCtx, map[string]any{})
	require.NoError(t, err)
	assert.Nil(t, cols)
}

func TestResolveSelectColumns_Array(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"select": []any{"id", "title", "status"},
	}
	cols, err := resolveSelectColumns(nCtx, config)
	require.NoError(t, err)
	assert.Equal(t, []string{"id", "title", "status"}, cols)
}

func TestResolveSelectColumns_InvalidType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{"select": "not an array"}
	_, err := resolveSelectColumns(nCtx, config)
	require.Error(t, err)
}

// --- applyWhere tests ---

func TestApplyWhere_WhereOnly(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T1", "u1")
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T2", "u2")

	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"where": map[string]any{"user_id": "u1"},
	}

	tx := db.Table("tasks")
	tx, err := applyWhere(tx, nCtx, config)
	require.NoError(t, err)

	var results []map[string]any
	tx.Scan(&results)
	assert.Len(t, results, 1)
}

func TestApplyWhere_WhereClauseOnly(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T1", "u1")
	db.Exec("INSERT INTO tasks (title, user_id) VALUES (?, ?)", "T2", "u2")

	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"where_clause": map[string]any{
			"query":  "user_id = ?",
			"params": []any{"u2"},
		},
	}

	tx := db.Table("tasks")
	tx, err := applyWhere(tx, nCtx, config)
	require.NoError(t, err)

	var results []map[string]any
	tx.Scan(&results)
	assert.Len(t, results, 1)
}

func TestApplyWhere_BothCombined(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, status, user_id) VALUES (?, ?, ?)", "T1", "active", "u1")
	db.Exec("INSERT INTO tasks (title, status, user_id) VALUES (?, ?, ?)", "T2", "done", "u1")
	db.Exec("INSERT INTO tasks (title, status, user_id) VALUES (?, ?, ?)", "T3", "active", "u2")

	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"where": map[string]any{"user_id": "u1"},
		"where_clause": map[string]any{
			"query":  "status = ?",
			"params": []any{"active"},
		},
	}

	tx := db.Table("tasks")
	tx, err := applyWhere(tx, nCtx, config)
	require.NoError(t, err)

	var results []map[string]any
	tx.Scan(&results)
	assert.Len(t, results, 1)
	assert.Equal(t, "T1", results[0]["title"])
}
