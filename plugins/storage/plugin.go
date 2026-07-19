package storage

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/spf13/afero"
)

// Plugin implements the Afero-based storage plugin.
type Plugin struct{}

func (p *Plugin) Name() string   { return "storage" }
func (p *Plugin) Prefix() string { return "storage" }

func (p *Plugin) HasServices() bool { return true }

func (p *Plugin) Nodes() []api.NodeRegistration { return nil }

func (p *Plugin) CreateService(config map[string]any) (any, error) {
	backend, _ := config["backend"].(string)
	if backend == "" {
		backend = "local"
	}

	switch backend {
	case "local":
		path, _ := config["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("storage: local backend requires 'path'")
		}
		if err := os.MkdirAll(path, 0755); err != nil {
			return nil, fmt.Errorf("storage: create path %q: %w", path, err)
		}
		fi, err := os.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("storage: stat base path %q: %w", path, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			real, _ := filepath.EvalSymlinks(path)
			return nil, fmt.Errorf("storage: base path %q must not be a symlink (resolved to %q)", path, real)
		}
		fs := afero.NewBasePathFs(afero.NewOsFs(), path)
		return &Service{fs: fs, backend: "local"}, nil

	case "memory":
		fs := afero.NewMemMapFs()
		return &Service{fs: fs, backend: "memory"}, nil

	default:
		return nil, fmt.Errorf("storage: unknown backend %q (supported: local, memory)", backend)
	}
}

// ServiceConfigSchema documents the storage service `config` block.
// additionalProperties is false: unknown keys are silently ignored by
// CreateService. "path" is not marked schema-required because it is only
// required for the "local" backend (default) — CreateService remains the
// arbiter of that conditional requirement.
func (p *Plugin) ServiceConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"backend": map[string]any{
				"type": "string",
				// "" is accepted because CreateService treats empty as the
				// default (local) — schema and parser must agree (#386).
				"enum":        []any{"local", "memory", ""},
				"description": "Storage backend (default local; empty string means default)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Base directory on disk (required when backend is local; ignored for memory)",
			},
		},
		"required":             []any{},
		"additionalProperties": false,
	}
}

func (p *Plugin) HealthCheck(service any) error {
	_, ok := service.(*Service)
	if !ok {
		return fmt.Errorf("storage: invalid service type")
	}
	return nil
}

func (p *Plugin) Shutdown(_ any) error { return nil }
