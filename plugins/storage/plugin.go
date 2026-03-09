package storage

import (
	"fmt"
	"os"

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
		fs := afero.NewBasePathFs(afero.NewOsFs(), path)
		return &Service{fs: fs, backend: "local"}, nil

	case "memory":
		fs := afero.NewMemMapFs()
		return &Service{fs: fs, backend: "memory"}, nil

	default:
		return nil, fmt.Errorf("storage: unknown backend %q (supported: local, memory)", backend)
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
