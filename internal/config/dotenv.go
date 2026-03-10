package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// LoadDotEnv loads .env files into the process environment.
// Existing environment variables are never overwritten.
//
// Loading order (later files override earlier ones for unset vars):
//  1. {configDir}/.env
//  2. {cwd}/.env (only if cwd differs from configDir)
//  3. {configDir}/.env.{env} (environment-specific override)
func LoadDotEnv(configDir, env string) []string {
	var loaded []string

	absConfig, _ := filepath.Abs(configDir)
	cwd, _ := os.Getwd()

	// 1. .env in config directory
	if f := filepath.Join(absConfig, ".env"); fileExists(f) {
		if err := godotenv.Load(f); err == nil {
			loaded = append(loaded, f)
		}
	}

	// 2. .env in working directory (if different)
	if cwd != absConfig {
		if f := filepath.Join(cwd, ".env"); fileExists(f) {
			if err := godotenv.Load(f); err == nil {
				loaded = append(loaded, f)
			}
		}
	}

	// 3. .env.{environment} in config directory
	if env != "" {
		if f := filepath.Join(absConfig, fmt.Sprintf(".env.%s", env)); fileExists(f) {
			if err := godotenv.Load(f); err == nil {
				loaded = append(loaded, f)
			}
		}
	}

	return loaded
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
