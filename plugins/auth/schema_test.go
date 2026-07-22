package auth

import (
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const testSchema = `
CREATE TABLE auth_users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  email_verified_at TIMESTAMP,
  status TEXT NOT NULL DEFAULT 'active',
  roles TEXT NOT NULL DEFAULT '["user"]',
  metadata TEXT NOT NULL DEFAULT '{}',
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);
CREATE TABLE auth_sessions (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
  token_hash TEXT NOT NULL UNIQUE,
  created_at TIMESTAMP NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  last_used_at TIMESTAMP,
  ip TEXT,
  user_agent TEXT,
  revoked_at TIMESTAMP
);
CREATE TABLE auth_tokens (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
  purpose TEXT NOT NULL,
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMP NOT NULL,
  consumed_at TIMESTAMP,
  created_at TIMESTAMP NOT NULL
);
`

// newTestDB builds testSchema on an in-memory SQLite with foreign keys
// actually enforced.
//
// "_foreign_keys=1" is the DSN parameter mattn/go-sqlite3 (what
// gorm.io/driver/sqlite wraps) understands; modernc/glebarez's
// "_pragma=foreign_keys(1)" is silently ignored by mattn, which is how this
// fixture spent its early life advertising referential integrity it never
// enforced. The PRAGMA read below makes that failure mode loud: a DSN typo or
// a driver swap fails every test in the package instead of quietly turning
// every REFERENCES clause in testSchema back into a comment.
//
// A single connection is mandatory: SQLite's foreign_keys pragma is
// per-connection, so a second connection from the pool would arrive with
// enforcement off.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_foreign_keys=1"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	var fkEnabled int
	if err := db.Raw("PRAGMA foreign_keys").Scan(&fkEnabled).Error; err != nil {
		t.Fatal(err)
	}
	if fkEnabled != 1 {
		t.Fatalf("foreign_keys pragma is %d; testSchema's REFERENCES clauses would be inert", fkEnabled)
	}

	if err := db.Exec(testSchema).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

// The pragma check in newTestDB proves enforcement is switched on; these two
// prove it reaches testSchema's own clauses. Without them a future schema edit
// could drop a REFERENCES clause and no test would notice.
func TestFixtureRejectsDanglingForeignKey(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()

	for _, tc := range []struct {
		table string
		row   map[string]any
	}{
		{"auth_sessions", map[string]any{
			"id": "s1", "user_id": "no-such-user", "token_hash": "h1",
			"created_at": now, "expires_at": now.Add(time.Hour),
		}},
		{"auth_tokens", map[string]any{
			"id": "t1", "user_id": "no-such-user", "purpose": PurposeVerifyEmail,
			"token_hash": "h2", "expires_at": now.Add(time.Hour), "created_at": now,
		}},
	} {
		t.Run(tc.table, func(t *testing.T) {
			if err := db.Table(tc.table).Create(tc.row).Error; err == nil {
				t.Fatalf("insert into %s with a dangling user_id succeeded; the foreign key is inert", tc.table)
			}
		})
	}
}

func TestFixtureCascadesUserDelete(t *testing.T) {
	db := newTestDB(t)
	now := time.Now().UTC()
	userID := seedUser(t, db, "cascade@example.com", "hash", "active")

	if err := db.Table("auth_sessions").Create(map[string]any{
		"id": "s1", "user_id": userID, "token_hash": "h1",
		"created_at": now, "expires_at": now.Add(time.Hour),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("auth_tokens").Create(map[string]any{
		"id": "t1", "user_id": userID, "purpose": PurposeVerifyEmail,
		"token_hash": "h2", "expires_at": now.Add(time.Hour), "created_at": now,
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Exec("DELETE FROM auth_users WHERE id = ?", userID).Error; err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"auth_sessions", "auth_tokens"} {
		var count int64
		if err := db.Table(table).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s: %d rows survived the owner's deletion; ON DELETE CASCADE is inert", table, count)
		}
	}
}

func testService() *Service {
	svc, err := newService(map[string]any{"database": "test-db"})
	if err != nil {
		panic(err)
	}
	// fast argon params for tests
	svc.Argon = ArgonParams{MemoryKiB: 1024, Iterations: 1, Parallelism: 1, SaltLen: 16, KeyLen: 32}
	return svc
}

func testServices(db *gorm.DB) map[string]any {
	return map[string]any{"auth": testService(), "database": db}
}

// fakeCtx implements api.ExecutionContext for unit tests. Resolve is identity:
// tests pass literal config values, not expressions.
type fakeCtx struct{}

func (fakeCtx) Input() any          { return nil }
func (fakeCtx) Auth() *api.AuthData { return nil }
func (fakeCtx) Trigger() api.TriggerData {
	return api.TriggerData{}
}
func (fakeCtx) Resolve(expr string) (any, error) { return expr, nil }
func (fakeCtx) ResolveWithVars(expr string, _ map[string]any) (any, error) {
	return expr, nil
}
func (fakeCtx) Log(string, string, map[string]any) {}
