//go:build integration

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// pgAuthDB builds the auth schema on a real Postgres, where foreign keys are
// actually enforced — unlike the SQLite unit fixture.
func pgAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.Open(containers.StartPostgres(t)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.Exec(testSchema).Error)
	return db
}

// A real FK violation from a real driver must classify as ConflictError,
// and must NOT be mistaken for the "exists" edge.
func TestCreateSessionRealForeignKeyViolation(t *testing.T) {
	db := pgAuthDB(t)

	_, _, err := newCreateSessionExecutor(nil).Execute(context.Background(), fakeCtx{},
		map[string]any{"user_id": "00000000-0000-0000-0000-000000000000"}, testServices(db))
	require.Error(t, err)

	var ce *api.ConflictError
	require.True(t, errors.As(err, &ce), "want ConflictError, got %v", err)
	require.Equal(t, "session", ce.Resource)
}

// sqliteFKAuthDB builds the auth schema on a real SQLite database with
// foreign keys actually enforced. The mattn/go-sqlite3 driver this project
// uses takes "_foreign_keys=1" as its DSN pragma param — unlike
// schema_test.go's newTestDB, which uses modernc's "_pragma=foreign_keys(1)"
// syntax. mattn silently ignores that unrecognized param, so that fixture
// runs with foreign keys OFF; that is a known pre-existing bug in
// schema_test.go and is left alone here. A single connection is required:
// SQLite's foreign_keys pragma is per-connection, and gorm may otherwise
// hand out a second, pristine connection that never saw the pragma.
func sqliteFKAuthDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_foreign_keys=1"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	var fkEnabled int
	require.NoError(t, db.Raw("PRAGMA foreign_keys").Scan(&fkEnabled).Error)
	require.Equal(t, 1, fkEnabled, "foreign_keys pragma must be enabled for this test to prove anything")

	require.NoError(t, db.Exec(testSchema).Error)
	return db
}

// The SQLite counterpart to TestCreateSessionRealForeignKeyViolation: the
// same driver condition (a foreign-key violation) must classify identically
// regardless of which database produced it.
func TestCreateSessionRealForeignKeyViolationSQLite(t *testing.T) {
	db := sqliteFKAuthDB(t)

	_, _, err := newCreateSessionExecutor(nil).Execute(context.Background(), fakeCtx{},
		map[string]any{"user_id": "00000000-0000-0000-0000-000000000000"}, testServices(db))
	require.Error(t, err)

	var ce *api.ConflictError
	require.True(t, errors.As(err, &ce), "want ConflictError, got %v", err)
	require.Equal(t, "session", ce.Resource)
}

// A real unique violation still routes to the "exists" edge, not a 409.
func TestCreateUserRealUniqueViolationStillExists(t *testing.T) {
	db := pgAuthDB(t)
	cfg := map[string]any{"email": "dup@example.com", "password": "password123"}

	out, _, err := newCreateUserExecutor(nil).Execute(context.Background(), fakeCtx{}, cfg, testServices(db))
	require.NoError(t, err)
	require.Equal(t, api.OutputSuccess, out)

	out, _, err = newCreateUserExecutor(nil).Execute(context.Background(), fakeCtx{}, cfg, testServices(db))
	require.NoError(t, err, "unique violation must stay on the exists edge")
	require.Equal(t, "exists", out)
}
