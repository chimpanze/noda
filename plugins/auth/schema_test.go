package auth

import (
	"testing"

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

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_pragma=foreign_keys(1)"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec(testSchema).Error; err != nil {
		t.Fatal(err)
	}
	return db
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
