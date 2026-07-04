package auth

import (
	"testing"
	"time"
)

func TestNewServiceDefaults(t *testing.T) {
	svc, err := newService(map[string]any{"database": "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	if svc.DatabaseName != "postgres" {
		t.Fatalf("DatabaseName = %q", svc.DatabaseName)
	}
	if svc.SessionTTL != 720*time.Hour {
		t.Fatalf("SessionTTL = %v", svc.SessionTTL)
	}
	if svc.Cookie.Name != "noda_session" || !svc.Cookie.Secure || !svc.Cookie.HTTPOnly || svc.Cookie.SameSite != "Lax" || svc.Cookie.Path != "/" {
		t.Fatalf("cookie defaults wrong: %+v", svc.Cookie)
	}
	if svc.TokenTTL(PurposeVerifyEmail) != 24*time.Hour || svc.TokenTTL(PurposeResetPassword) != time.Hour {
		t.Fatal("token TTL defaults wrong")
	}
	if svc.Argon.MemoryKiB != 65536 || svc.Argon.Iterations != 3 || svc.Argon.Parallelism != 2 {
		t.Fatalf("argon defaults wrong: %+v", svc.Argon)
	}
}

func TestNewServiceValidation(t *testing.T) {
	if _, err := newService(map[string]any{}); err == nil {
		t.Fatal("missing database must error")
	}
	if _, err := newService(map[string]any{"database": "db", "session": map[string]any{"ttl": "nope"}}); err == nil {
		t.Fatal("bad ttl must error")
	}
	if _, err := newService(map[string]any{"database": "db", "tokens": map[string]any{"verify_email_ttl": "nope"}}); err == nil {
		t.Fatal("bad verify_email_ttl must error")
	}
	if _, err := newService(map[string]any{"database": "db", "tokens": map[string]any{"reset_password_ttl": "nope"}}); err == nil {
		t.Fatal("bad reset_password_ttl must error")
	}
}

func TestNewServiceFullConfig(t *testing.T) {
	svc, err := newService(map[string]any{
		"database": "pg",
		"session": map[string]any{
			"ttl": "12h",
			"cookie": map[string]any{
				"name": "sid", "path": "/app", "domain": "example.com",
				"same_site": "Strict", "secure": false, "http_only": false,
			},
		},
		"argon2": map[string]any{
			"memory_kib": float64(32768), "iterations": float64(2),
			"parallelism": float64(4), "salt_len": float64(24), "key_len": float64(48),
		},
		"tokens": map[string]any{
			"verify_email_ttl": "48h", "reset_password_ttl": "30m",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if svc.SessionTTL != 12*time.Hour {
		t.Fatalf("SessionTTL = %v", svc.SessionTTL)
	}
	c := svc.Cookie
	if c.Name != "sid" || c.Path != "/app" || c.Domain != "example.com" || c.SameSite != "Strict" || c.Secure || c.HTTPOnly {
		t.Fatalf("cookie overrides wrong: %+v", c)
	}
	a := svc.Argon
	if a.MemoryKiB != 32768 || a.Iterations != 2 || a.Parallelism != 4 || a.SaltLen != 24 || a.KeyLen != 48 {
		t.Fatalf("argon overrides wrong: %+v", a)
	}
	if svc.TokenTTL(PurposeVerifyEmail) != 48*time.Hour || svc.TokenTTL(PurposeResetPassword) != 30*time.Minute {
		t.Fatal("token TTL overrides wrong")
	}
	if svc.TokenTTL("unknown_purpose") != time.Hour {
		t.Fatal("unknown purpose must fall back to 1h")
	}
}

func TestSessionCookieObjects(t *testing.T) {
	svc, err := newService(map[string]any{"database": "pg"})
	if err != nil {
		t.Fatal(err)
	}
	c := svc.SessionCookieObject("raw-token", 2*time.Hour)
	if c["name"] != "noda_session" || c["value"] != "raw-token" || c["max_age"] != float64(7200) {
		t.Fatalf("cookie object wrong: %+v", c)
	}
	cl := svc.ClearCookieObject()
	if cl["value"] != "" || cl["max_age"] != float64(-1) {
		t.Fatalf("clear cookie object wrong: %+v", cl)
	}
}

func TestPluginContract(t *testing.T) {
	p := &Plugin{}
	if p.Name() != "auth" || p.Prefix() != "auth" || !p.HasServices() {
		t.Fatal("plugin identity wrong")
	}
	svc, err := p.CreateService(map[string]any{"database": "db"})
	if err != nil {
		t.Fatal(err)
	}
	if err := p.HealthCheck(svc); err != nil {
		t.Fatal(err)
	}
	if err := p.Shutdown(svc); err != nil {
		t.Fatal(err)
	}
	if err := p.HealthCheck("not a service"); err == nil {
		t.Fatal("HealthCheck must reject wrong service type")
	}
}
