package auth

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
)

func TestCreateSession(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUserWithHash(t, db, "alice@example.com", hash, "active")

	exec := newCreateSessionExecutor(nil)
	out, data, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	res := data.(map[string]any)
	token, _ := res["token"].(string)
	if token == "" {
		t.Fatal("missing token")
	}
	cookie, _ := res["cookie"].(map[string]any)
	if cookie["name"] != "noda_session" || cookie["value"] != token || cookie["http_only"] != true {
		t.Fatalf("cookie object wrong: %v", cookie)
	}
	if _, ok := cookie["max_age"].(float64); !ok {
		t.Fatalf("max_age must be float64 for response.json, got %T", cookie["max_age"])
	}

	// raw token must not be stored
	var count int64
	db.Table("auth_sessions").Where("token_hash = ?", token).Count(&count)
	if count != 0 {
		t.Fatal("raw token stored in token_hash column")
	}
	db.Table("auth_sessions").Where("token_hash = ?", HashToken(token)).Count(&count)
	if count != 1 {
		t.Fatal("hashed token not stored")
	}
}

// metaCtx is a fakeCtx whose trigger carries HTTP request metadata.
type metaCtx struct{ fakeCtx }

func (metaCtx) Trigger() api.TriggerData {
	return api.TriggerData{Type: "http", ClientIP: "203.0.113.7", UserAgent: "curl/8.6.0"}
}

func TestCreateSessionStoresRequestMeta(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUserWithHash(t, db, "alice@example.com", hash, "active")

	exec := newCreateSessionExecutor(nil)
	out, data, err := exec.Execute(context.Background(), metaCtx{}, map[string]any{"user_id": userID}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	token := data.(map[string]any)["token"].(string)

	row := map[string]any{}
	if err := db.Table("auth_sessions").Where("token_hash = ?", HashToken(token)).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row["ip"] != "203.0.113.7" || row["user_agent"] != "curl/8.6.0" {
		t.Fatalf("request meta not stored: ip=%v user_agent=%v", row["ip"], row["user_agent"])
	}

	// non-HTTP trigger (fakeCtx): columns stay NULL, no empty strings
	_, d2, err := exec.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	if err != nil {
		t.Fatal(err)
	}
	row2 := map[string]any{}
	tok2 := d2.(map[string]any)["token"].(string)
	if err := db.Table("auth_sessions").Where("token_hash = ?", HashToken(tok2)).Take(&row2).Error; err != nil {
		t.Fatal(err)
	}
	if row2["ip"] != nil || row2["user_agent"] != nil {
		t.Fatalf("meta must stay NULL without request context: ip=%v ua=%v", row2["ip"], row2["user_agent"])
	}
}

func TestRevokeSession(t *testing.T) {
	db := newTestDB(t)
	svc := testService()
	hash, _ := svc.HashPassword("password123")
	userID := seedUserWithHash(t, db, "alice@example.com", hash, "active")

	create := newCreateSessionExecutor(nil)
	_, d1, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	_, d2, _ := create.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	token1 := d1.(map[string]any)["token"].(string)
	_ = d2

	revoke := newRevokeSessionExecutor(nil)
	out, data, err := revoke.Execute(context.Background(), fakeCtx{}, map[string]any{"token": token1}, testServices(db))
	if err != nil || out != api.OutputSuccess {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if data.(map[string]any)["revoked_count"].(int64) != 1 {
		t.Fatalf("revoked_count = %v", data.(map[string]any)["revoked_count"])
	}
	cc := data.(map[string]any)["clear_cookie"].(map[string]any)
	if cc["value"] != "" || cc["max_age"].(float64) != -1 {
		t.Fatalf("clear_cookie wrong: %v", cc)
	}

	// idempotent
	out, data, err = revoke.Execute(context.Background(), fakeCtx{}, map[string]any{"token": token1}, testServices(db))
	if err != nil || out != api.OutputSuccess || data.(map[string]any)["revoked_count"].(int64) != 0 {
		t.Fatalf("re-revoke must be idempotent success: out=%q err=%v", out, err)
	}

	// revoke all for user (one remains active)
	out, data, err = revoke.Execute(context.Background(), fakeCtx{}, map[string]any{"user_id": userID}, testServices(db))
	if err != nil || out != api.OutputSuccess || data.(map[string]any)["revoked_count"].(int64) != 1 {
		t.Fatalf("revoke-all: out=%q data=%v err=%v", out, data, err)
	}

	// exactly-one-selector validation
	if _, _, err := revoke.Execute(context.Background(), fakeCtx{}, map[string]any{}, testServices(db)); err == nil {
		t.Fatal("no selector must error")
	}
	if _, _, err := revoke.Execute(context.Background(), fakeCtx{}, map[string]any{"token": "x", "user_id": "y"}, testServices(db)); err == nil {
		t.Fatal("two selectors must error")
	}
}
