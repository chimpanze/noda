package migrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	return db
}

func TestCreate_GeneratesFiles(t *testing.T) {
	dir := t.TempDir()
	upFile, downFile, err := Create(dir, "create_users")
	require.NoError(t, err)

	assert.FileExists(t, upFile)
	assert.FileExists(t, downFile)

	assert.Contains(t, upFile, "create_users.up.sql")
	assert.Contains(t, downFile, "create_users.down.sql")

	upContent, _ := os.ReadFile(upFile)
	assert.Contains(t, string(upContent), "Write your migration SQL here")

	downContent, _ := os.ReadFile(downFile)
	assert.Contains(t, string(downContent), "Write your rollback SQL here")
}

func TestCreate_FileNaming(t *testing.T) {
	dir := t.TempDir()
	upFile, _, err := Create(dir, "add_index")
	require.NoError(t, err)

	base := filepath.Base(upFile)
	// Should be YYYYMMDDHHMMSS_add_index.up.sql (14-char timestamp + _ + name)
	assert.Len(t, base[:14], 14, "timestamp should be 14 characters")
	assert.Contains(t, base, "_add_index.up.sql")
}

func setupMigrations(t *testing.T, dir string) {
	t.Helper()
	// Create two migration files
	_ = os.WriteFile(filepath.Join(dir, "20260101000000_create_tasks.up.sql"), []byte(`
		CREATE TABLE tasks (id INTEGER PRIMARY KEY, title TEXT);
	`), 0644)
	_ = os.WriteFile(filepath.Join(dir, "20260101000000_create_tasks.down.sql"), []byte(`
		DROP TABLE tasks;
	`), 0644)

	_ = os.WriteFile(filepath.Join(dir, "20260102000000_add_status.up.sql"), []byte(`
		ALTER TABLE tasks ADD COLUMN status TEXT DEFAULT 'pending';
	`), 0644)
	_ = os.WriteFile(filepath.Join(dir, "20260102000000_add_status.down.sql"), []byte(`
		-- SQLite doesn't support DROP COLUMN, so recreate
		CREATE TABLE tasks_backup AS SELECT id, title FROM tasks;
		DROP TABLE tasks;
		ALTER TABLE tasks_backup RENAME TO tasks;
	`), 0644)
}

func TestUp_AppliesPendingMigrations(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	setupMigrations(t, dir)

	ran, err := Up(db, dir)
	require.NoError(t, err)
	assert.Len(t, ran, 2)
	assert.Equal(t, "20260101000000_create_tasks", ran[0])
	assert.Equal(t, "20260102000000_add_status", ran[1])

	// Verify table exists
	var count int64
	db.Raw("SELECT COUNT(*) FROM tasks").Scan(&count)
	assert.Equal(t, int64(0), count) // table exists, no rows
}

func TestUp_SkipsAlreadyApplied(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	setupMigrations(t, dir)

	// Run once
	ran1, err := Up(db, dir)
	require.NoError(t, err)
	assert.Len(t, ran1, 2)

	// Run again — should skip all
	ran2, err := Up(db, dir)
	require.NoError(t, err)
	assert.Empty(t, ran2)
}

func TestUp_SchemaMigrationsTableCreatedAutomatically(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	setupMigrations(t, dir)

	_, err := Up(db, dir)
	require.NoError(t, err)

	// schema_migrations table should exist
	var records []schemaMigration
	err = db.Find(&records).Error
	require.NoError(t, err)
	assert.Len(t, records, 2)
}

func TestDown_RollsBackLastMigration(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	setupMigrations(t, dir)

	_, err := Up(db, dir)
	require.NoError(t, err)

	// Roll back the last migration (add_status)
	rolled, err := Down(db, dir)
	require.NoError(t, err)
	assert.Equal(t, "20260102000000_add_status", rolled)

	// Check status: only first migration should be applied
	statuses, err := Status(db, dir)
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	assert.True(t, statuses[0].Applied)
	assert.False(t, statuses[1].Applied)
}

func TestDown_NoMigrations(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()

	// Ensure migrations table exists
	_ = ensureMigrationsTable(db)

	_, err := Down(db, dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no migrations to roll back")
}

func TestStatus_ShowsCorrectState(t *testing.T) {
	db := newTestDB(t)
	dir := t.TempDir()
	setupMigrations(t, dir)

	// Before running
	statuses, err := Status(db, dir)
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	assert.False(t, statuses[0].Applied)
	assert.False(t, statuses[1].Applied)

	// After running
	_, _ = Up(db, dir)
	statuses, err = Status(db, dir)
	require.NoError(t, err)
	require.Len(t, statuses, 2)
	assert.True(t, statuses[0].Applied)
	assert.NotNil(t, statuses[0].AppliedAt)
	assert.True(t, statuses[1].Applied)
	assert.NotNil(t, statuses[1].AppliedAt)
}

func TestFindMigrations_Sorted(t *testing.T) {
	dir := t.TempDir()
	// Create files out of order
	_ = os.WriteFile(filepath.Join(dir, "20260301000000_third.up.sql"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(dir, "20260101000000_first.up.sql"), []byte(""), 0644)
	_ = os.WriteFile(filepath.Join(dir, "20260201000000_second.up.sql"), []byte(""), 0644)

	migrations, err := findMigrations(dir)
	require.NoError(t, err)
	require.Len(t, migrations, 3)
	assert.Equal(t, "20260101000000", migrations[0].Version)
	assert.Equal(t, "20260201000000", migrations[1].Version)
	assert.Equal(t, "20260301000000", migrations[2].Version)
}

func TestFindMigrations_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	migrations, err := findMigrations(dir)
	require.NoError(t, err)
	assert.Empty(t, migrations)
}

func TestFindMigrations_NonexistentDir(t *testing.T) {
	migrations, err := findMigrations("/nonexistent/path")
	require.NoError(t, err)
	assert.Empty(t, migrations)
}
