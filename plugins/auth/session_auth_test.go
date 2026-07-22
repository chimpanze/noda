package auth

import (
	"context"
	"testing"
	"time"
)

func TestAuthenticateSession(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUserWithHash(t, db, "alice@example.com", hash, "active")

	create := newCreateSessionExecutor(nil)
	_, data, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	token := data.(map[string]any)["token"].(string)

	ad, err := svc.AuthenticateSession(context.Background(), db, token)
	if err != nil || ad == nil {
		t.Fatalf("ad=%v err=%v", ad, err)
	}
	if ad.UserID != userID || ad.Claims["email"] != "alice@example.com" || ad.Claims["email_verified"] != false {
		t.Fatalf("auth data wrong: %+v", ad)
	}
	if len(ad.Roles) != 1 || ad.Roles[0] != "user" {
		t.Fatalf("roles wrong: %v", ad.Roles)
	}

	// invalid token → (nil, nil)
	if ad, err := svc.AuthenticateSession(context.Background(), db, "garbage"); ad != nil || err != nil {
		t.Fatalf("garbage token: ad=%v err=%v", ad, err)
	}

	// revoked → nil
	revoke := newRevokeSessionExecutor(nil)
	if _, _, err := revoke.Execute(context.Background(), fakeCtx{}, map[string]any{"token": token}, testServices(db)); err != nil {
		t.Fatalf("revoke session: %v", err)
	}
	if ad, _ := svc.AuthenticateSession(context.Background(), db, token); ad != nil {
		t.Fatal("revoked session must not authenticate")
	}
}

func TestAuthenticateSessionExpiredAndDisabled(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")

	// expired session
	u1 := seedUserWithHash(t, db, "a@example.com", hash, "active")
	create := newCreateSessionExecutor(nil)
	_, d, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": u1, "ttl": "1ns"}, testServices(db))
	time.Sleep(10 * time.Millisecond)
	if ad, _ := svc.AuthenticateSession(context.Background(), db, d.(map[string]any)["token"].(string)); ad != nil {
		t.Fatal("expired session must not authenticate")
	}

	// disabled user
	u2 := seedUserWithHash(t, db, "b@example.com", hash, "disabled")
	_, d2, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": u2}, testServices(db))
	if ad, _ := svc.AuthenticateSession(context.Background(), db, d2.(map[string]any)["token"].(string)); ad != nil {
		t.Fatal("disabled user must not authenticate")
	}
}

func TestAuthenticateSessionTouchesLastUsed(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUserWithHash(t, db, "alice@example.com", hash, "active")
	create := newCreateSessionExecutor(nil)
	_, d, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	token := d.(map[string]any)["token"].(string)

	if _, err := svc.AuthenticateSession(context.Background(), db, token); err != nil {
		t.Fatalf("first authenticate: %v", err)
	}
	var first *time.Time
	db.Table("auth_sessions").Where("token_hash = ?", HashToken(token)).Pluck("last_used_at", &first)
	if first == nil {
		t.Fatal("last_used_at not set on first use")
	}
	if _, err := svc.AuthenticateSession(context.Background(), db, token); err != nil {
		t.Fatalf("second authenticate: %v", err)
	}
	var second *time.Time
	db.Table("auth_sessions").Where("token_hash = ?", HashToken(token)).Pluck("last_used_at", &second)
	if !second.Equal(*first) {
		t.Fatal("last_used_at must be throttled (unchanged within a minute)")
	}
}
