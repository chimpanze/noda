package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/chimpanze/noda/internal/secrets"
)

// ResolvedConfig is the fully resolved, validated configuration structure.
type ResolvedConfig struct {
	Environment string
	Root        map[string]any
	Vars        map[string]string
	Schemas     map[string]map[string]any
	Routes      map[string]map[string]any
	Workflows   map[string]map[string]any
	Workers     map[string]map[string]any
	Schedules   map[string]map[string]any
	Connections map[string]map[string]any
	Tests       map[string]map[string]any
	Models      map[string]map[string]any
	FileCount   int
}

// ValidateAll runs the full config loading and validation pipeline.
// Steps: detect env → discover → load → merge → load secrets → resolve $env() → resolve $ref → validate schemas → validate cross-refs.
func ValidateAll(rootPath string, envFlag string, sm *secrets.Manager) (*ResolvedConfig, []ValidationError) {
	// 1. Detect environment
	env, err := DetectEnvironment(envFlag)
	if err != nil {
		return nil, []ValidationError{{Message: err.Error()}}
	}

	// 2. Discover files
	discovered, err := Discover(rootPath, env)
	if err != nil {
		return nil, []ValidationError{{Message: err.Error()}}
	}

	// 3. Load all JSON files
	raw, loadErrs := LoadAll(discovered)
	if len(loadErrs) > 0 {
		var valErrs []ValidationError
		for _, e := range loadErrs {
			valErrs = append(valErrs, ValidationError{Message: e.Error()})
		}
		return nil, valErrs
	}

	// 4. Merge overlay into root
	if raw.Overlay != nil {
		for _, w := range ValidateMergePreservedKeys(raw.Root, raw.Overlay) {
			slog.Warn("config overlay warning", "detail", w)
		}
		raw.Root = MergeOverlay(raw.Root, raw.Overlay)
	}

	// 5. Resolve $env() in root config using SecretsManager
	resolved, envErrs := sm.Resolve(raw.Root)
	if len(envErrs) > 0 {
		var valErrs []ValidationError
		for _, e := range envErrs {
			valErrs = append(valErrs, ValidationError{Message: e.Error()})
		}
		return nil, valErrs
	}
	raw.Root = resolved

	// 5.5 Resolve $var()
	if len(raw.Vars) > 0 {
		varErrs := resolveVarsAll(raw)
		if len(varErrs) > 0 {
			var valErrs []ValidationError
			for _, e := range varErrs {
				valErrs = append(valErrs, ValidationError{Message: e.Error()})
			}
			return nil, valErrs
		}
	}

	// 6. Resolve $ref
	refErrs := ResolveRefs(raw)
	if len(refErrs) > 0 {
		var valErrs []ValidationError
		for _, e := range refErrs {
			valErrs = append(valErrs, ValidationError{Message: e.Error()})
		}
		return nil, valErrs
	}

	// 7. Validate schemas
	schemaErrs := Validate(raw)
	if len(schemaErrs) > 0 {
		return nil, schemaErrs
	}

	// 8. Validate cross-file references
	crossRefErrs := ValidateCrossRefs(raw)
	if len(crossRefErrs) > 0 {
		return nil, crossRefErrs
	}

	// Count files
	fileCount := 1 // noda.json
	if discovered.Overlay != "" {
		fileCount++
	}
	if discovered.Vars != "" {
		fileCount++
	}
	for _, paths := range [][]string{
		discovered.Schemas, discovered.Routes, discovered.Workflows,
		discovered.Workers, discovered.Schedules, discovered.Connections,
		discovered.Tests,
		discovered.Models,
	} {
		fileCount += len(paths)
	}

	return &ResolvedConfig{
		Environment: env,
		Root:        raw.Root,
		Vars:        raw.Vars,
		Schemas:     raw.Schemas,
		Routes:      raw.Routes,
		Workflows:   raw.Workflows,
		Workers:     raw.Workers,
		Schedules:   raw.Schedules,
		Connections: raw.Connections,
		Tests:       raw.Tests,
		Models:      raw.Models,
		FileCount:   fileCount,
	}, nil
}

// ValidateAllVerbose returns additional info for verbose output.
type ValidateInfo struct {
	Environment string
	OverlayFile string
	FileCounts  map[string]int
}

// GetValidateInfo returns info about the validation for verbose output.
func GetValidateInfo(rootPath string, envFlag string) (*ValidateInfo, error) {
	env, err := DetectEnvironment(envFlag)
	if err != nil {
		return nil, err
	}

	discovered, err := Discover(rootPath, env)
	if err != nil {
		return nil, err
	}

	info := &ValidateInfo{
		Environment: env,
		OverlayFile: discovered.Overlay,
		FileCounts: map[string]int{
			"schemas":     len(discovered.Schemas),
			"routes":      len(discovered.Routes),
			"workflows":   len(discovered.Workflows),
			"workers":     len(discovered.Workers),
			"schedules":   len(discovered.Schedules),
			"connections": len(discovered.Connections),
			"tests":       len(discovered.Tests),
			"models":      len(discovered.Models),
		},
	}

	return info, nil
}

// NewSecretsManager creates and loads a SecretsManager by reading provider
// configuration from the raw root config (noda.json). This must run before
// $env() resolution since it provides the values $env() resolves against.
//
// If no "secrets" section exists, defaults to DotEnvProvider only (safe default).
func NewSecretsManager(configDir, envFlag string) (*secrets.Manager, error) {
	env, err := DetectEnvironment(envFlag)
	if err != nil {
		return nil, err
	}

	providers := parseSecretsProviders(configDir, env)
	sm := secrets.New(providers...)
	if err := sm.Load(context.Background()); err != nil {
		return nil, fmt.Errorf("loading secrets: %w", err)
	}
	return sm, nil
}

// parseSecretsProviders reads the raw noda.json to determine which secret
// providers to configure. Falls back to DotEnvProvider if no config found.
func parseSecretsProviders(configDir, env string) []secrets.Provider {
	absConfig, _ := filepath.Abs(configDir)
	rootFile := filepath.Join(absConfig, "noda.json")

	raw, err := os.ReadFile(rootFile)
	if err != nil {
		// No noda.json — use default provider
		return []secrets.Provider{&secrets.DotEnvProvider{ConfigDir: configDir, Env: env}}
	}

	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return []secrets.Provider{&secrets.DotEnvProvider{ConfigDir: configDir, Env: env}}
	}

	secretsCfg, ok := root["secrets"].(map[string]any)
	if !ok {
		return []secrets.Provider{&secrets.DotEnvProvider{ConfigDir: configDir, Env: env}}
	}

	providersList, ok := secretsCfg["providers"].([]any)
	if !ok || len(providersList) == 0 {
		return []secrets.Provider{&secrets.DotEnvProvider{ConfigDir: configDir, Env: env}}
	}

	var providers []secrets.Provider
	for _, item := range providersList {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		switch m["type"] {
		case "dotenv":
			providers = append(providers, &secrets.DotEnvProvider{ConfigDir: configDir, Env: env})
		case "env":
			providers = append(providers, &secrets.ProcessEnvProvider{})
		}
	}

	if len(providers) == 0 {
		return []secrets.Provider{&secrets.DotEnvProvider{ConfigDir: configDir, Env: env}}
	}
	return providers
}
