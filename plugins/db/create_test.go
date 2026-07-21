package db

import (
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// registerFakeDuplicateKeyErr installs a gorm "before create" callback that
// forces a typed sqlite3.Error with the unique-constraint extended code,
// simulating what a real SQLite unique constraint violation looks like.
// dberr.Classify keys off the typed driver error, not message text, so the
// fake must be a real sqlite3.Error rather than a plain errors.New.
func registerFakeDuplicateKeyErr(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		_ = tx.AddError(sqlite3.Error{
			Code:         sqlite3.ErrConstraint,
			ExtendedCode: sqlite3.ErrConstraintUnique,
		})
	})
	require.NoError(t, err)
}

// TestConflictError_ReasonIsSafe drives db.create against a duplicate-key
// error and asserts the returned *api.ConflictError.Reason is the safe,
// constant string "unique constraint violation" — not the raw driver error
// (which would leak the constraint name and offending value).
func TestConflictError_ReasonIsSafe(t *testing.T) {
	db := newTestDB(t)
	registerFakeDuplicateKeyErr(t, db, "fake_dup_err_create")

	exec := &createExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"table": "tasks",
		"data":  map[string]any{"title": "dup"},
	}

	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)

	var cfErr *api.ConflictError
	require.ErrorAs(t, err, &cfErr)
	require.Equal(t, "tasks", cfErr.Resource)
	require.Equal(t, "unique constraint violation", cfErr.Reason)
	require.NotContains(t, cfErr.Reason, "tasks_title_key")
	require.NotContains(t, cfErr.Reason, "dup")
}

// TestUpsertConflictError_ReasonIsSafe is the same check for db.upsert.
func TestUpsertConflictError_ReasonIsSafe(t *testing.T) {
	db := newTestDB(t)
	registerFakeDuplicateKeyErr(t, db, "fake_dup_err_upsert")

	exec := &upsertExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	config := map[string]any{
		"table":    "tasks",
		"data":     map[string]any{"title": "dup"},
		"conflict": "id",
	}

	_, _, err := exec.Execute(t.Context(), nCtx, config, testServices(db))
	require.Error(t, err)

	var cfErr *api.ConflictError
	require.ErrorAs(t, err, &cfErr)
	require.Equal(t, "tasks", cfErr.Resource)
	require.Equal(t, "unique constraint violation", cfErr.Reason)
	require.NotContains(t, cfErr.Reason, "tasks_title_key")
	require.NotContains(t, cfErr.Reason, "dup")
}
