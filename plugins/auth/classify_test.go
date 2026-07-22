package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// injectDriverErr makes every gorm operation of the given kind fail with a
// chosen SQLSTATE.
//
// SQLite cannot produce codes like 40001 or 57014, and this package's test
// fixture runs with foreign keys OFF (newTestDB's DSN uses modernc's
// _pragma= syntax, which the mattn driver ignores), so a real classifiable
// error cannot be provoked at most of these call sites. Injection drives
// the site through Classify deterministically instead.
//
// Register the injection AFTER seeding, or the seed will fail too.
func injectDriverErr(t *testing.T, db *gorm.DB, kind, sqlstate string) {
	t.Helper()
	fn := func(tx *gorm.DB) {
		_ = tx.AddError(&pgconn.PgError{Code: sqlstate, Message: "injected"})
	}
	var err error
	switch kind {
	case "query":
		err = db.Callback().Query().Before("gorm:query").Register("test:inject", fn)
	case "create":
		err = db.Callback().Create().Before("gorm:create").Register("test:inject", fn)
	case "update":
		err = db.Callback().Update().Before("gorm:update").Register("test:inject", fn)
	default:
		t.Fatalf("unknown callback kind %q", kind)
	}
	require.NoError(t, err)
}

// seedClassifyUser creates one user and returns its id.
func seedClassifyUser(t *testing.T, db *gorm.DB) string {
	t.Helper()
	out, data, err := newCreateUserExecutor(nil).Execute(context.Background(), fakeCtx{},
		map[string]any{"email": "seed@example.com", "password": "password123"},
		testServices(db))
	require.NoError(t, err)
	require.Equal(t, api.OutputSuccess, out)
	uid, _ := data.(map[string]any)["id"].(string)
	require.NotEmpty(t, uid)
	return uid
}

// A 40001 reaching any auth node must produce ServiceUnavailableError (503),
// matching what plugins/db has returned since #403 — not a bare 500.
func TestAuthNodesClassifySerializationFailure(t *testing.T) {
	cases := []struct {
		name string
		kind string
		run  func(db *gorm.DB, uid string) error
	}{
		{"get_user", "query", func(db *gorm.DB, uid string) error {
			_, _, err := newGetUserExecutor(nil).Execute(context.Background(), fakeCtx{},
				map[string]any{"user_id": uid}, testServices(db))
			return err
		}},
		{"verify_credentials", "query", func(db *gorm.DB, uid string) error {
			_, _, err := newVerifyCredentialsExecutor(nil).Execute(context.Background(), fakeCtx{},
				map[string]any{"email": "seed@example.com", "password": "password123"},
				testServices(db))
			return err
		}},
		{"create_session", "create", func(db *gorm.DB, uid string) error {
			_, _, err := newCreateSessionExecutor(nil).Execute(context.Background(), fakeCtx{},
				map[string]any{"user_id": uid}, testServices(db))
			return err
		}},
		{"revoke_session", "update", func(db *gorm.DB, uid string) error {
			_, _, err := newRevokeSessionExecutor(nil).Execute(context.Background(), fakeCtx{},
				map[string]any{"user_id": uid}, testServices(db))
			return err
		}},
		{"set_password", "update", func(db *gorm.DB, uid string) error {
			_, _, err := newSetPasswordExecutor(nil).Execute(context.Background(), fakeCtx{},
				map[string]any{"user_id": uid, "password": "newpassword123"}, testServices(db))
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			uid := seedClassifyUser(t, db)
			injectDriverErr(t, db, tc.kind, "40001")

			err := tc.run(db, uid)
			require.Error(t, err)

			var su *api.ServiceUnavailableError
			require.True(t, errors.As(err, &su), "want ServiceUnavailableError, got %v", err)
			require.Equal(t, "database", su.Service)
			require.True(t, errors.As(err, new(*pgconn.PgError)), "cause must stay recoverable")
		})
	}
}

// An unmapped driver code keeps today's behavior exactly: wrapped with the
// node's own context prefix, so existing messages and %w chains survive.
func TestAuthNodesFallThroughUnmapped(t *testing.T) {
	db := newTestDB(t)
	uid := seedClassifyUser(t, db)
	injectDriverErr(t, db, "query", "42703") // undefined column: author bug, stays 500

	_, _, err := newGetUserExecutor(nil).Execute(context.Background(), fakeCtx{},
		map[string]any{"user_id": uid}, testServices(db))
	require.Error(t, err)

	var su *api.ServiceUnavailableError
	var ve *api.ValidationError
	require.False(t, errors.As(err, &su))
	require.False(t, errors.As(err, &ve))
	require.Contains(t, err.Error(), "auth.get_user:")
}

// The token nodes classify like every other node.
func TestTokenNodesClassifySerializationFailure(t *testing.T) {
	t.Run("create_token", func(t *testing.T) {
		db := newTestDB(t)
		uid := seedClassifyUser(t, db)
		injectDriverErr(t, db, "update", "40001") // hits the invalidate-prior UPDATE

		_, _, err := newCreateTokenExecutor(nil).Execute(context.Background(), fakeCtx{},
			map[string]any{"user_id": uid, "purpose": PurposeResetPassword}, testServices(db))
		require.Error(t, err)

		var su *api.ServiceUnavailableError
		require.True(t, errors.As(err, &su), "want ServiceUnavailableError, got %v", err)
	})

	t.Run("consume_token", func(t *testing.T) {
		db := newTestDB(t)
		uid := seedClassifyUser(t, db)

		out, data, err := newCreateTokenExecutor(nil).Execute(context.Background(), fakeCtx{},
			map[string]any{"user_id": uid, "purpose": PurposeResetPassword}, testServices(db))
		require.NoError(t, err)
		require.Equal(t, api.OutputSuccess, out)
		raw, _ := data.(map[string]any)["token"].(string)
		require.NotEmpty(t, raw)

		injectDriverErr(t, db, "update", "40001")

		_, _, err = newConsumeTokenExecutor(nil).Execute(context.Background(), fakeCtx{},
			map[string]any{"token": raw, "purpose": PurposeResetPassword}, testServices(db))
		require.Error(t, err)

		var su *api.ServiceUnavailableError
		require.True(t, errors.As(err, &su), "want ServiceUnavailableError, got %v", err)
	})
}

// INVARIANT 1 (spec): the "exists" edge stays bound to IsUniqueViolation.
//
// A 23505 must still return "exists" with a nil error — that edge is the
// anti-enumeration register flow. A 23503 (foreign key) must NOT: routing it
// to "exists" would report a false "email already registered" for an FK bug.
func TestCreateUserExistsEdgeStaysOnUniqueViolation(t *testing.T) {
	t.Run("23505 still yields exists", func(t *testing.T) {
		db := newTestDB(t)
		injectDriverErr(t, db, "create", "23505")

		out, _, err := newCreateUserExecutor(nil).Execute(context.Background(), fakeCtx{},
			map[string]any{"email": "dup@example.com", "password": "password123"},
			testServices(db))
		require.NoError(t, err, "unique violation must not surface as an error")
		require.Equal(t, "exists", out)
	})

	t.Run("23503 does not yield exists", func(t *testing.T) {
		db := newTestDB(t)
		injectDriverErr(t, db, "create", "23503")

		out, _, err := newCreateUserExecutor(nil).Execute(context.Background(), fakeCtx{},
			map[string]any{"email": "fk@example.com", "password": "password123"},
			testServices(db))
		require.Error(t, err)
		require.NotEqual(t, "exists", out, "an FK violation must never read as 'email taken'")

		var ce *api.ConflictError
		require.True(t, errors.As(err, &ce), "want ConflictError, got %v", err)
		require.Equal(t, "user", ce.Resource)
	})
}

// INVARIANT 2 (spec): verify_credentials' anti-enumeration edges are unchanged.
// An unknown email must still return "invalid" with a nil error — never a
// typed error and never a distinguishable status.
func TestVerifyCredentialsUnknownEmailStillInvalid(t *testing.T) {
	db := newTestDB(t)
	seedClassifyUser(t, db)

	out, _, err := newVerifyCredentialsExecutor(nil).Execute(context.Background(), fakeCtx{},
		map[string]any{"email": "nobody@example.com", "password": "password123"},
		testServices(db))
	require.NoError(t, err)
	require.Equal(t, "invalid", out)
}
