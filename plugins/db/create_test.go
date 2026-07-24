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
//
// sqlite3.Error's message is an unexported field with no exported
// constructor, so this fake carries no message text at all — there is no
// driver detail available to assert against. That's fine: dberr.Classify
// derives ConflictError.Reason from a fixed lookup table keyed on
// ExtendedCode (see sqliteConflict in internal/dberr/sqlite.go), never
// from the driver error's message, so Reason can never contain
// constraint/column names regardless of what the fake's message holds.
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

// registerFakeForeignKeyErr is registerFakeDuplicateKeyErr for a foreign-key
// violation. Foreign-key violations still surface as *api.ConflictError (only
// unique violations were rerouted to the "exists" output in #436), so this is
// what now exercises the Reason-is-safe guarantee.
func registerFakeForeignKeyErr(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	err := db.Callback().Create().Before("gorm:create").Register(name, func(tx *gorm.DB) {
		_ = tx.AddError(sqlite3.Error{
			Code:         sqlite3.ErrConstraint,
			ExtendedCode: sqlite3.ErrConstraintForeignKey,
		})
	})
	require.NoError(t, err)
}

// TestConflictError_ReasonIsSafe drives db.create against a constraint
// violation and asserts the returned *api.ConflictError.Reason is the safe,
// constant lookup-table string — not the raw driver error (which would leak
// the constraint name and offending value).
func TestConflictError_ReasonIsSafe(t *testing.T) {
	db := newTestDB(t)
	registerFakeForeignKeyErr(t, db, "fake_fk_err_create")

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
	require.Equal(t, "foreign key constraint violation", cfErr.Reason)
}

// TestUpsertConflictError_ReasonIsSafe is the same check for db.upsert.
func TestUpsertConflictError_ReasonIsSafe(t *testing.T) {
	db := newTestDB(t)
	registerFakeForeignKeyErr(t, db, "fake_fk_err_upsert")

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
	require.Equal(t, "foreign key constraint violation", cfErr.Reason)
}

// A unique violation is routed to the "exists" output instead of raising,
// for every write node that has one (#436).
func TestUniqueViolation_FiresExistsOutput(t *testing.T) {
	cases := []struct {
		name   string
		exec   api.NodeExecutor
		config map[string]any
	}{
		{"create", &createExecutor{}, map[string]any{
			"table": "tasks", "data": map[string]any{"title": "dup"},
		}},
		{"update", &updateExecutor{}, map[string]any{
			"table": "tasks", "data": map[string]any{"title": "dup"},
			"where": map[string]any{"id": 1},
		}},
		{"upsert", &upsertExecutor{}, map[string]any{
			"table": "tasks", "data": map[string]any{"title": "dup"},
			"conflict": "id",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			registerFakeDuplicateKeyErr(t, db, "fake_dup_"+tc.name)
			// db.update goes through the Update callback, not Create.
			if tc.name == "update" {
				require.NoError(t, db.Callback().Update().Before("gorm:update").
					Register("fake_dup_upd", func(tx *gorm.DB) {
						_ = tx.AddError(sqlite3.Error{
							Code:         sqlite3.ErrConstraint,
							ExtendedCode: sqlite3.ErrConstraintUnique,
						})
					}))
			}

			nCtx := &mockExecCtx{resolveFunc: identityResolve}
			output, data, err := tc.exec.Execute(t.Context(), nCtx, tc.config, testServices(db))
			require.NoError(t, err, "a unique violation must not raise")
			require.Equal(t, "exists", output)
			require.NotNil(t, data)
		})
	}
}
