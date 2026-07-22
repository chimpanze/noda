package auth

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func seedUser(t *testing.T, db *gorm.DB, email, passwordHash, status string) string {
	t.Helper()
	id := uuid.NewString()
	now := time.Now().UTC()
	err := db.Table("auth_users").Create(map[string]any{
		"id": id, "email": email, "password_hash": passwordHash,
		"status": status, "roles": `["user"]`, "metadata": `{}`,
		"created_at": now, "updated_at": now,
	}).Error
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestVerifyCredentials(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	seedUser(t, db, "alice@example.com", hash, "active")
	exec := newVerifyCredentialsExecutor(nil)

	out, data, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "ALICE@example.com", "password": "password123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if _, exists := data.(map[string]any)["password_hash"]; exists {
		t.Fatal("password_hash must be stripped")
	}

	for name, cfg := range map[string]map[string]any{
		"wrong password": {"email": "alice@example.com", "password": "nope-nope"},
		"unknown user":   {"email": "ghost@example.com", "password": "password123"},
	} {
		out, _, err := exec.Execute(context.Background(), fakeCtx{}, cfg, testServices(db))
		if err != nil || out != "invalid" {
			t.Fatalf("%s: out=%q err=%v", name, out, err)
		}
	}
}

func TestVerifyCredentialsDisabledUser(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	seedUser(t, db, "off@example.com", hash, "disabled")
	exec := newVerifyCredentialsExecutor(nil)
	out, _, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "off@example.com", "password": "password123",
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("disabled user must be invalid: out=%q err=%v", out, err)
	}
}

func TestVerifyCredentialsUnrecognizedHashIsInvalid(t *testing.T) {
	db := newTestDB(t)
	exec := newVerifyCredentialsExecutor(nil)

	for name, hash := range map[string]string{
		"bcrypt 2x variant":  "$2x$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy",
		"truncated argon2id": "$argon2id$v=19$m=65536,t=3",
		"pbkdf2":             "pbkdf2_sha256$260000$abcdef$0123456789abcdef",
		"empty":              "",
	} {
		email := strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "@example.com"
		seedUser(t, db, email, hash, "active")
		out, _, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
			"email": email, "password": "password123",
		}, testServices(db))
		if err != nil || out != "invalid" {
			t.Fatalf("%s: must be invalid (no error), got out=%q err=%v", name, out, err)
		}
	}
}

func TestVerifyCredentialsBcryptUpgrade(t *testing.T) {
	db := newTestDB(t)
	id := seedUser(t, db, "old@example.com", mustBcrypt(t, "password123"), "active")
	exec := newVerifyCredentialsExecutor(nil)
	out, _, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "old@example.com", "password": "password123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	var newHash string
	db.Table("auth_users").Where("id = ?", id).Pluck("password_hash", &newHash)
	if !strings.HasPrefix(newHash, "$argon2id$") {
		t.Fatalf("hash must be upgraded to argon2id, got %q", newHash[:10])
	}
}
