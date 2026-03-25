package secrets

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// DotEnvProvider reads .env files without polluting os.Environ().
type DotEnvProvider struct {
	ConfigDir string
	Env       string // e.g., "production"
}

func (p *DotEnvProvider) Name() string { return "dotenv" }

// Load reads .env files and returns their values as a map.
// Loading order (later files override earlier ones for the same key):
//  1. {configDir}/.env
//  2. {cwd}/.env (only if cwd differs from configDir)
//  3. {configDir}/.env.{environment}
func (p *DotEnvProvider) Load(_ context.Context) (map[string]string, error) {
	merged := make(map[string]string)

	absConfig, err := filepath.Abs(p.ConfigDir)
	if err != nil {
		absConfig = p.ConfigDir
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}

	// 1. .env in config directory
	if f := filepath.Join(absConfig, ".env"); fileExists(f) {
		vals, err := godotenv.Read(f)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", f, err)
		}
		for k, v := range vals {
			merged[k] = v
		}
	}

	// 2. .env in working directory (if different)
	if cwd != absConfig {
		if f := filepath.Join(cwd, ".env"); fileExists(f) {
			vals, err := godotenv.Read(f)
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", f, err)
			}
			for k, v := range vals {
				merged[k] = v
			}
		}
	}

	// 3. .env.{environment} in config directory
	if p.Env != "" {
		if f := filepath.Join(absConfig, fmt.Sprintf(".env.%s", p.Env)); fileExists(f) {
			vals, err := godotenv.Read(f)
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", f, err)
			}
			for k, v := range vals {
				merged[k] = v
			}
		}
	}

	return merged, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// ProcessEnvProvider reads all current process environment variables.
// This is opt-in — only used when explicitly configured.
type ProcessEnvProvider struct{}

func (p *ProcessEnvProvider) Name() string { return "env" }

func (p *ProcessEnvProvider) Load(_ context.Context) (map[string]string, error) {
	environ := os.Environ()
	m := make(map[string]string, len(environ))
	for _, entry := range environ {
		if k, v, ok := strings.Cut(entry, "="); ok {
			m[k] = v
		}
	}
	return m, nil
}
