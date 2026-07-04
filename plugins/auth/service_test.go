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
}
