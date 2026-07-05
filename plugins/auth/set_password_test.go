package auth

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
)

func TestSetPassword(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	oldHash, _ := svc.HashPassword("oldpassword")
	userID := seedUser(t, db, "alice@example.com", oldHash, "active")

	create := newCreateSessionExecutor(nil)
	_, _, _ = create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))

	set := newSetPasswordExecutor(nil)
	out, data, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "password": "newpassword123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if data.(map[string]any)["revoked_sessions"].(int64) != 1 {
		t.Fatal("existing sessions must be revoked by default")
	}

	verify := newVerifyCredentialsExecutor(nil)
	out, _, _ = verify.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "newpassword123",
	}, testServices(db))
	if out != api.OutputSuccess {
		t.Fatal("new password must verify")
	}
	out, _, _ = verify.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "oldpassword",
	}, testServices(db))
	if out != "invalid" {
		t.Fatal("old password must no longer verify")
	}
}

func TestSetPasswordUnknownUser(t *testing.T) {
	db := newTestDB(t)
	set := newSetPasswordExecutor(nil)
	if _, _, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": "nope", "password": "newpassword123",
	}, testServices(db)); err == nil {
		t.Fatal("unknown user must error")
	}
}

func TestSetPasswordNoRevoke(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	oldHash, _ := svc.HashPassword("oldpassword")
	userID := seedUser(t, db, "alice@example.com", oldHash, "active")

	create := newCreateSessionExecutor(nil)
	_, _, _ = create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))

	set := newSetPasswordExecutor(nil)
	out, data, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "password": "newpassword123", "revoke_sessions": false,
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if data.(map[string]any)["revoked_sessions"].(int64) != 0 {
		t.Fatal("sessions should not be revoked when revoke_sessions=false")
	}
}

func TestSetPasswordDescriptor(t *testing.T) {
	d := &setPasswordDescriptor{}
	if d.Name() != "set_password" {
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
