package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/require"
)

// envForDir lists the environment variables each example/fixture references
// via $env() in its noda.json, with dummy values sufficient to satisfy
// config.ValidateAll's "unresolved $env()" check. These are pre-existing
// deployment requirements of the examples (real secrets/connection strings
// at runtime), not something node ConfigSchema enforcement should flag.
var envForDir = map[string][]string{
	"auth-demo":       {"SMTP_HOST", "DATABASE_URL"},
	"discord-bot":     {"DISCORD_BOT_TOKEN"},
	"realtime-collab": {"JWT_SECRET", "DATABASE_URL", "REDIS_URL"},
	"rest-api":        {"DATABASE_URL", "JWT_SECRET"},
	"saas-backend":    {"JWT_SECRET", "REDIS_URL", "SMTP_FROM", "SMTP_HOST", "SMTP_PORT", "DATABASE_URL"},
	"video-rooms":     {"LIVEKIT_API_KEY", "LIVEKIT_API_SECRET", "LIVEKIT_URL"},
}

// Every shipped example and full-project fixture must pass the exact
// pipeline `noda validate` runs — including node ConfigSchema enforcement.
func TestShippedProjectsValidate(t *testing.T) {
	exampleDirs, err := filepath.Glob("../../examples/*")
	require.NoError(t, err)
	dirs := append(exampleDirs,
		"../../testdata/auth",
		// NOTE: testdata/valid-project is intentionally excluded. Its
		// create-task.json workflow references node type "db.insert",
		// which has never existed (the db plugin registers "db.create",
		// not "db.insert") — confirmed identical on origin/main HEAD
		// (2acaf06), i.e. this fixture predates Node-ConfigSchema
		// enforcement and was never validate-clean. That's unrelated
		// node-type drift, not a schema-enforcement finding, so it's out
		// of scope for #332 and left for a separate fix.
		"../../testdata/node-e2e",
		"../../testdata/livekit-example",
		"../../testdata/minimal-project",
	)

	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "noda.json")); err != nil {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			for _, name := range envForDir[filepath.Base(dir)] {
				t.Setenv(name, "dummy")
			}

			sm, err := config.NewSecretsManager(dir, "")
			require.NoError(t, err)
			rc, errs := config.ValidateAll(dir, "", sm)
			require.Empty(t, errs)

			plugins := registry.NewPluginRegistry()
			require.NoError(t, registerCorePlugins(plugins))
			_, bootErrs := registry.Bootstrap(context.Background(), rc, plugins,
				registry.BootstrapOptions{DryRun: true})
			require.Empty(t, bootErrs)
		})
	}
}

func TestInvalidProjectStillFails(t *testing.T) {
	dir := "../../testdata/invalid-project"
	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)
	_, errs := config.ValidateAll(dir, "", sm)
	require.NotEmpty(t, errs, "invalid-project must keep failing validation")
}
