package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chimpanze/noda/internal/secrets"
)

func TestDefaultProviders_ProcessEnvHonoredAndWins(t *testing.T) {
	dir := t.TempDir()
	// A .env that sets the key to one value...
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("NODA_TEST_KEY=from_dotenv\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// ...and a process env var that should override it.
	t.Setenv("NODA_TEST_KEY", "from_process")
	t.Setenv("NODA_ONLY_PROCESS", "present")

	providers := defaultSecretsProviders(dir, "")
	sm := secrets.New(providers...)
	if err := sm.Load(context.Background()); err != nil {
		t.Fatalf("load: %v", err)
	}

	// Process env is honored at all (the #238 bug: it was ignored).
	if v, ok := sm.Get("NODA_ONLY_PROCESS"); !ok || v != "present" {
		t.Errorf("process env not honored: got %q ok=%v", v, ok)
	}
	// Process env overrides .env.
	if v, _ := sm.Get("NODA_TEST_KEY"); v != "from_process" {
		t.Errorf("process env should override .env, got %q", v)
	}
}
