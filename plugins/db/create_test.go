package db

import (
	"errors"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// registerFakeDuplicateKeyErr installs a gorm "before create" callback that
// forces a raw driver-style duplicate-key error, simulating what a real
// Postgres unique constraint violation looks like (constraint name + value),
// without depending on a specific driver's error formatting.
func registerFakeDuplicateKeyErr(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		_ = tx.AddError(errors.New(`ERROR: duplicate key value violates unique constraint "tasks_title_key" (title)=(dup)`))
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
