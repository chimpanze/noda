package auth

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
)

func TestGetUserByEmailAndID(t *testing.T) {
	db := newTestDB(t)
	create := newCreateUserExecutor(nil)
	_, data, err := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "password123",
	}, testServices(db))
	if err != nil {
		t.Fatal(err)
	}
	id := data.(map[string]any)["id"].(string)

	get := newGetUserExecutor(nil)
	out, got, err := get.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": id}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("by id: out=%q err=%v", out, err)
	}
	if got.(map[string]any)["email"] != "alice@example.com" {
		t.Fatal("wrong user")
	}
	if _, exists := got.(map[string]any)["password_hash"]; exists {
		t.Fatal("password_hash must be stripped")
	}

	out, _, err = get.Execute(context.Background(), fakeCtx{}, map[string]any{"email": "ALICE@example.com"}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("by email: out=%q err=%v", out, err)
	}

	out, _, err = get.Execute(context.Background(), fakeCtx{}, map[string]any{"email": "nobody@example.com"}, testServices(db))
	if err != nil || out != "not_found" {
		t.Fatalf("missing: out=%q err=%v", out, err)
	}

	if _, _, err := get.Execute(context.Background(), fakeCtx{}, map[string]any{}, testServices(db)); err == nil {
		t.Fatal("neither user_id nor email must error")
	}
}
