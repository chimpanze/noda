package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	// Create a temp directory with a .env file
	dir := t.TempDir()
	envContent := "TEST_DOTENV_VAR=hello\nTEST_DOTENV_OTHER=world\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Ensure vars are not set
	os.Unsetenv("TEST_DOTENV_VAR")
	os.Unsetenv("TEST_DOTENV_OTHER")

	loaded := LoadDotEnv(dir, "")
	if len(loaded) != 1 {
		t.Fatalf("expected 1 file loaded, got %d", len(loaded))
	}

	if v := os.Getenv("TEST_DOTENV_VAR"); v != "hello" {
		t.Errorf("expected TEST_DOTENV_VAR=hello, got %q", v)
	}
	if v := os.Getenv("TEST_DOTENV_OTHER"); v != "world" {
		t.Errorf("expected TEST_DOTENV_OTHER=world, got %q", v)
	}

	// Clean up
	os.Unsetenv("TEST_DOTENV_VAR")
	os.Unsetenv("TEST_DOTENV_OTHER")
}

func TestLoadDotEnvDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TEST_NOOVERWRITE=from_file\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set the var before loading
	os.Setenv("TEST_NOOVERWRITE", "from_env")
	defer os.Unsetenv("TEST_NOOVERWRITE")

	LoadDotEnv(dir, "")

	if v := os.Getenv("TEST_NOOVERWRITE"); v != "from_env" {
		t.Errorf("expected existing env var to be preserved, got %q", v)
	}
}

func TestLoadDotEnvWithEnvironment(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("TEST_ENV_BASE=base\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.production"), []byte("TEST_ENV_PROD=prod\n"), 0644); err != nil {
		t.Fatal(err)
	}

	os.Unsetenv("TEST_ENV_BASE")
	os.Unsetenv("TEST_ENV_PROD")

	loaded := LoadDotEnv(dir, "production")
	if len(loaded) != 2 {
		t.Fatalf("expected 2 files loaded, got %d: %v", len(loaded), loaded)
	}

	if v := os.Getenv("TEST_ENV_BASE"); v != "base" {
		t.Errorf("expected TEST_ENV_BASE=base, got %q", v)
	}
	if v := os.Getenv("TEST_ENV_PROD"); v != "prod" {
		t.Errorf("expected TEST_ENV_PROD=prod, got %q", v)
	}

	os.Unsetenv("TEST_ENV_BASE")
	os.Unsetenv("TEST_ENV_PROD")
}

func TestLoadDotEnvMissingFile(t *testing.T) {
	dir := t.TempDir()
	// No .env file — should return empty, no error
	loaded := LoadDotEnv(dir, "")
	if len(loaded) != 0 {
		t.Fatalf("expected 0 files loaded, got %d", len(loaded))
	}
}
