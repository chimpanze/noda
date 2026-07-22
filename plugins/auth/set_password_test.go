package auth

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
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
	branches := d.ConfigSchema()["oneOf"].([]any)
	if len(branches) != 2 {
		t.Fatal("unexpected config schema")
	}
	for _, b := range branches {
		if len(b.(map[string]any)["properties"].(map[string]any)) != 4 {
			t.Fatal("unexpected config schema")
		}
	}
	if len(d.OutputDescriptions()) != 3 {
		t.Fatal("unexpected outputs")
	}
}

// mintResetToken creates a real reset_password token for userID and returns
// the raw token string.
func mintResetToken(t *testing.T, db *gorm.DB, userID string) string {
	t.Helper()
	create := newCreateTokenExecutor(nil)
	out, data, err := create.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": userID, "purpose": PurposeResetPassword,
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("mint token: out=%q err=%v", out, err)
	}
	return data.(map[string]any)["token"].(string)
}

func TestSetPasswordWithTokenConsumesAtomically(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	oldHash, _ := svc.HashPassword("oldpassword")
	userID := seedUser(t, db, "alice@example.com", oldHash, "active")
	token := mintResetToken(t, db, userID)

	set := newSetPasswordExecutor(nil)
	out, data, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "password": "newpassword123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if _, ok := data.(map[string]any)["revoked_sessions"]; !ok {
		t.Fatal("success output must include revoked_sessions")
	}

	verify := newVerifyCredentialsExecutor(nil)
	out, _, _ = verify.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "newpassword123",
	}, testServices(db))
	if out != api.OutputSuccess {
		t.Fatal("new password must verify")
	}

	// The token is single-use: a second attempt must be invalid.
	out, _, err = set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "password": "anotherpassword1",
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("reused token: out=%q err=%v", out, err)
	}
}

func TestSetPasswordWithUnknownTokenIsInvalid(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	oldHash, _ := svc.HashPassword("oldpassword")
	seedUser(t, db, "alice@example.com", oldHash, "active")

	set := newSetPasswordExecutor(nil)
	out, _, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": "never-minted", "password": "newpassword123",
	}, testServices(db))
	if err != nil || out != "invalid" {
		t.Fatalf("out=%q err=%v", out, err)
	}

	verify := newVerifyCredentialsExecutor(nil)
	out, _, _ = verify.Execute(context.Background(), fakeCtx{}, map[string]any{
		"email": "alice@example.com", "password": "oldpassword",
	}, testServices(db))
	if out != api.OutputSuccess {
		t.Fatal("password must be unchanged after an invalid token")
	}
}

// TestSetPasswordTokenSurvivesBadPassword is the auth-3 regression test: a
// rejected new password must NOT burn the reset token.
func TestSetPasswordTokenSurvivesBadPassword(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	oldHash, _ := svc.HashPassword("oldpassword")
	userID := seedUser(t, db, "alice@example.com", oldHash, "active")
	token := mintResetToken(t, db, userID)

	set := newSetPasswordExecutor(nil)
	out, _, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "password": "short",
	}, testServices(db))
	if err == nil {
		t.Fatalf("bad password must error, got out=%q", out)
	}

	// Same token must still work with a valid password.
	out, _, err = set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"token": token, "password": "newpassword123",
	}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("token must survive a rejected password: out=%q err=%v", out, err)
	}
}

func TestSetPasswordUserIDTokenMutuallyExclusive(t *testing.T) {
	db := newTestDB(t)
	set := newSetPasswordExecutor(nil)

	if _, _, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"user_id": "u1", "token": "tok", "password": "newpassword123",
	}, testServices(db)); err == nil {
		t.Fatal("user_id + token together must error")
	}
	if _, _, err := set.Execute(context.Background(), fakeCtx{}, map[string]any{
		"password": "newpassword123",
	}, testServices(db)); err == nil {
		t.Fatal("neither user_id nor token must error")
	}
}
