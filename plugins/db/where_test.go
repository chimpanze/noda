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

// --- resolveWhereClause additional error paths ---

func TestResolveWhereClause_QueryNotString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"where_clause": map[string]any{
			"query": 123,
		},
	}
	_, err := resolveWhereClause(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestResolveWhereClause_ResolveQueryError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return nil, errResolve
	}}
	config := map[string]any{
		"where_clause": map[string]any{
			"query": "some_expr",
		},
	}
	_, err := resolveWhereClause(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve query")
}

func TestResolveWhereClause_ResolveQueryNonString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return 42, nil
	}}
	config := map[string]any{
		"where_clause": map[string]any{
			"query": "some_expr",
		},
	}
	_, err := resolveWhereClause(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected string")
}

// --- resolveJoins additional error paths ---

func TestResolveJoins_ItemNotObject(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{"not an object"},
	}
	_, err := resolveJoins(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an object")
}

func TestResolveJoins_InvalidJoinType(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{
				"type":  "NATURAL",
				"table": "users",
				"on":    "a = b",
			},
		},
	}
	_, err := resolveJoins(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid join type")
}

func TestResolveJoins_TableNotString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{
				"table": 123,
				"on":    "a = b",
			},
		},
	}
	_, err := resolveJoins(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestResolveJoins_InvalidTableIdentifier(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{
				"table": "1invalid",
				"on":    "a = b",
			},
		},
	}
	_, err := resolveJoins(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

func TestResolveJoins_OnNotString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{
				"table": "users",
				"on":    123,
			},
		},
	}
	_, err := resolveJoins(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestResolveJoins_OnInvalidSQLFragment(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{
				"table": "users",
				"on":    "a = b; DROP TABLE users",
			},
		},
	}
	_, err := resolveJoins(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "semicolons")
}

func TestResolveJoins_WithParams(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"joins": []any{
			map[string]any{
				"type":   "LEFT",
				"table":  "users",
				"on":     "tasks.user_id = users.id AND users.active = ?",
				"params": []any{true},
			},
		},
	}
	joins, err := resolveJoins(nCtx, config)
	require.NoError(t, err)
	require.Len(t, joins, 1)
	assert.Equal(t, []any{true}, joins[0].Params)
}

// --- resolveSelectColumns additional error paths ---

func TestResolveSelectColumns_ItemNotString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"select": []any{"id", 123},
	}
	_, err := resolveSelectColumns(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

func TestResolveSelectColumns_ResolveError(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return nil, errResolve
	}}
	config := map[string]any{
		"select": []any{"some_expr"},
	}
	_, err := resolveSelectColumns(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve select")
}

func TestResolveSelectColumns_ResolvedNonString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return 42, nil
	}}
	config := map[string]any{
		"select": []any{"some_expr"},
	}
	_, err := resolveSelectColumns(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected string")
}

func TestResolveSelectColumns_InvalidIdentifier(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"select": []any{"valid_col", "1invalid"},
	}
	_, err := resolveSelectColumns(nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid identifier")
}

// --- applyQueryOptions additional coverage ---

func TestApplyQueryOptions_InvalidOrder(t *testing.T) {
	db := newTestDB(t)
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"order": "name; DROP TABLE users",
	}
	tx := db.Table("tasks")
	_, err := applyQueryOptions(tx, nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "order")
}

func TestApplyQueryOptions_GroupBy(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T1", "active")
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T2", "active")
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T3", "done")

	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"select": []any{"status"},
		"group":  "status",
	}
	tx := db.Table("tasks")
	tx, err := applyQueryOptions(tx, nCtx, config)
	require.NoError(t, err)
	var results []map[string]any
	tx.Scan(&results)
	assert.Len(t, results, 2)
}

func TestApplyQueryOptions_GroupByInvalidIdentifier(t *testing.T) {
	db := newTestDB(t)
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"group": "1invalid",
	}
	tx := db.Table("tasks")
	_, err := applyQueryOptions(tx, nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "group")
}

func TestApplyQueryOptions_HavingString(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T1", "active")
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T2", "active")

	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"select": []any{"status"},
		"group":  "status",
		"having": "count(status) > 0",
	}
	tx := db.Table("tasks")
	tx, err := applyQueryOptions(tx, nCtx, config)
	require.NoError(t, err)
	var results []map[string]any
	tx.Scan(&results)
	assert.NotEmpty(t, results)
}

func TestApplyQueryOptions_HavingStringInvalidSQLFragment(t *testing.T) {
	db := newTestDB(t)
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"having": "1; DROP TABLE users",
	}
	tx := db.Table("tasks")
	_, err := applyQueryOptions(tx, nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "having")
}

func TestApplyQueryOptions_HavingStringResolveError(t *testing.T) {
	db := newTestDB(t)
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return nil, errResolve
	}}
	config := map[string]any{
		"having": "some_expr",
	}
	tx := db.Table("tasks")
	_, err := applyQueryOptions(tx, nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "having")
}

func TestApplyQueryOptions_HavingStringResolvesNonString(t *testing.T) {
	db := newTestDB(t)
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return 42, nil
	}}
	config := map[string]any{
		"having": "some_expr",
	}
	tx := db.Table("tasks")
	_, err := applyQueryOptions(tx, nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "having resolved to")
}

func TestApplyQueryOptions_HavingObjectFormat(t *testing.T) {
	db := newTestDB(t)
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T1", "active")
	db.Exec("INSERT INTO tasks (title, status) VALUES (?, ?)", "T2", "active")

	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"select": []any{"status"},
		"group":  "status",
		"having": map[string]any{
			"query":  "count(status) > ?",
			"params": []any{float64(0)},
		},
	}
	tx := db.Table("tasks")
	tx, err := applyQueryOptions(tx, nCtx, config)
	require.NoError(t, err)
	var results []map[string]any
	tx.Scan(&results)
	assert.NotEmpty(t, results)
}

func TestApplyQueryOptions_HavingObjectMissingQuery(t *testing.T) {
	db := newTestDB(t)
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"having": map[string]any{
			"params": []any{1},
		},
	}
	tx := db.Table("tasks")
	_, err := applyQueryOptions(tx, nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestApplyQueryOptions_HavingObjectResolveError(t *testing.T) {
	db := newTestDB(t)
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return nil, errResolve
	}}
	config := map[string]any{
		"having": map[string]any{
			"query": "count(*) > 0",
		},
	}
	tx := db.Table("tasks")
	_, err := applyQueryOptions(tx, nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "having")
}

func TestApplyQueryOptions_HavingObjectResolvesNonString(t *testing.T) {
	db := newTestDB(t)
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return 42, nil
	}}
	config := map[string]any{
		"having": map[string]any{
			"query": "some_expr",
		},
	}
	tx := db.Table("tasks")
	_, err := applyQueryOptions(tx, nCtx, config)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "having.query resolved to")
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
