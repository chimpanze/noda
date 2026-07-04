package auth

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
)

func TestCreateUser(t *testing.T) {
	db := newTestDB(t)
	exec := newCreateUserExecutor(nil)
	out, data, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "  Alice@Example.COM ", "password": "password123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	user := data.(map[string]any)
	if user["email"] != "alice@example.com" {
		t.Fatalf("email not normalized: %v", user["email"])
	}
	if _, exists := user["password_hash"]; exists {
		t.Fatal("password_hash must be stripped from output")
	}
	roles, _ := user["roles"].([]string)
	if len(roles) != 1 || roles[0] != "user" {
		t.Fatalf("default roles wrong: %v", user["roles"])
	}

	// duplicate → exists
	out, _, err = exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "password123",
	}, testServices(db))
	if err != nil || out != "exists" {
		t.Fatalf("duplicate: out=%q err=%v", out, err)
	}
}

func TestCreateUserPasswordRules(t *testing.T) {
	db := newTestDB(t)
	exec := newCreateUserExecutor(nil)
	_, _, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "bob@example.com", "password": "short",
	}, testServices(db))
	if err == nil {
		t.Fatal("password < 8 chars must error")
	}
}
