package auth

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
)

func TestCreateAndConsumeToken(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")

	create := newCreateTokenExecutor(nil)
	out, data, err := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": PurposeVerifyEmail,
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("create: out=%q err=%v", out, err)
	}
	token := data.(map[string]any)["token"].(string)

	consume := newConsumeTokenExecutor(nil)

	// wrong purpose → invalid
	out, _, err = consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "purpose": PurposeResetPassword,
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("wrong purpose: out=%q err=%v", out, err)
	}

	// correct → success with user_id, and email_verified_at set
	out, data, err = consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "purpose": PurposeVerifyEmail,
	}, testServices(db))
	if err != nil || out != api.OutputSuccess || data.(map[string]any)["user_id"] != userID {
		t.Fatalf("consume: out=%q data=%v err=%v", out, data, err)
	}
	var verified *time.Time
	db.Table("auth_users").Where("id = ?", userID).Pluck("email_verified_at", &verified)
	if verified == nil {
		t.Fatal("email_verified_at not set")
	}

	// second consume → invalid
	out, _, err = consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "purpose": PurposeVerifyEmail,
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("reuse: out=%q err=%v", out, err)
	}
}

func TestCreateTokenInvalidatesPrior(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")
	create := newCreateTokenExecutor(nil)
	consume := newConsumeTokenExecutor(nil)

	_, d1, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID, "purpose": PurposeResetPassword}, testServices(db))
	_, _, _ = create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID, "purpose": PurposeResetPassword}, testServices(db))

	out, _, err := consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": d1.(map[string]any)["token"], "purpose": PurposeResetPassword,
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("older token must be invalidated: out=%q err=%v", out, err)
	}
}

func TestConsumeExpiredToken(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")
	create := newCreateTokenExecutor(nil)
	_, data, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": PurposeVerifyEmail, "ttl": "1ns",
	}, testServices(db))
	time.Sleep(10 * time.Millisecond)
	consume := newConsumeTokenExecutor(nil)
	out, _, err := consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": data.(map[string]any)["token"], "purpose": PurposeVerifyEmail,
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("expired: out=%q err=%v", out, err)
	}
}

// Polarity check: this test MUST fail if the consumed_at guard is removed from
// the UPDATE's WHERE clause (i.e. if consumption stops being atomic).
func TestConsumeTokenConcurrentSingleUse(t *testing.T) {
	db := newTestDB(t) // MaxOpenConns(1) serializes; the guard does the correctness work
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")
	create := newCreateTokenExecutor(nil)
	_, data, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": PurposeVerifyEmail,
	}, testServices(db))
	token := data.(map[string]any)["token"].(string)

	consume := newConsumeTokenExecutor(nil)
	const n = 8
	var wg sync.WaitGroup
	successes := make(chan struct{}, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, _, err := consume.Execute(context.Background(), fakeCtx{}, map[string]any{
				"token": token, "purpose": PurposeVerifyEmail,
			}, testServices(db))
			if err == nil && out == api.OutputSuccess {
				successes <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(successes)
	count := 0
	for range successes {
		count++
	}
	if count != 1 {
		t.Fatalf("token consumed %d times; must be exactly 1", count)
	}
}

// TestConsumeTokenVerifyEmailTransactional proves the consume-UPDATE and the
// email_verified_at UPDATE commit or fail together. It forces the second
// statement to fail (by dropping auth_users after the token is minted) and
// asserts the token was NOT burned — i.e. the consume-UPDATE was rolled back
// along with the failed verify step, rather than being left committed.
func TestConsumeTokenVerifyEmailTransactional(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")
	create := newCreateTokenExecutor(nil)
	_, data, err := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": PurposeVerifyEmail,
	}, testServices(db))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	token := data.(map[string]any)["token"].(string)

	// Force the third statement (email_verified_at update) to fail.
	if err := db.Exec("DROP TABLE auth_users").Error; err != nil {
		t.Fatalf("drop auth_users: %v", err)
	}

	consume := newConsumeTokenExecutor(nil)
	out, _, err := consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "purpose": PurposeVerifyEmail,
	}, testServices(db))
	if err == nil {
		t.Fatalf("expected error when email_verified_at update fails, got out=%q", out)
	}

	var consumedAt sql.NullTime
	if err := db.Table("auth_tokens").Where("token_hash = ?", HashToken(token)).
		Pluck("consumed_at", &consumedAt).Error; err != nil {
		t.Fatalf("query token: %v", err)
	}
	if consumedAt.Valid {
		t.Fatalf("token was burned despite the failed transaction: consumed_at=%v", consumedAt.Time)
	}
}

func TestCreateTokenInvalidPurpose(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")
	create := newCreateTokenExecutor(nil)
	_, _, err := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": "invalid_purpose",
	}, testServices(db))
	if err == nil || err.Error() != "auth.create_token: invalid purpose \"invalid_purpose\"" {
		t.Fatalf("expected invalid purpose error, got %v", err)
	}
}

func TestCreateTokenInvalidTTL(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUser(t, db, "alice@example.com", hash, "active")
	create := newCreateTokenExecutor(nil)
	_, _, err := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": PurposeVerifyEmail, "ttl": "not_a_duration",
	}, testServices(db))
	if err == nil || err.Error() != "auth.create_token: ttl: time: invalid duration \"not_a_duration\"" {
		t.Fatalf("expected ttl error, got %v", err)
	}
}

func TestCreateTokenDescriptor(t *testing.T) {
	d := &createTokenDescriptor{}
	if d.Name() != "create_token" {
		t.Fatal("unexpected name")
	}
	if len(d.ServiceDeps()) != 2 {
		t.Fatal("unexpected service deps")
	}
	if len(d.ConfigSchema()["properties"].(map[string]any)) != 3 {
		t.Fatal("unexpected config schema")
	}
	if len(d.OutputDescriptions()) != 2 {
		t.Fatal("unexpected outputs")
	}
}

func TestConsumeTokenDescriptor(t *testing.T) {
	d := &consumeTokenDescriptor{}
	if d.Name() != "consume_token" {
		t.Fatal("unexpected name")
	}
	if len(d.OutputDescriptions()) != 3 {
		t.Fatal("unexpected outputs")
	}
}

func TestConsumeTokenInvalidPurpose(t *testing.T) {
	db := newTestDB(t)
	consume := newConsumeTokenExecutor(nil)
	_, _, err := consume.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": "something", "purpose": "invalid",
	}, testServices(db))
	if err == nil || err.Error() != "auth.consume_token: invalid purpose \"invalid\"" {
		t.Fatalf("expected invalid purpose error, got %v", err)
	}
}
