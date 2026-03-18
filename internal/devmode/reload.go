package devmode

import (
	"log/slog"
	"sync"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/trace"
)

// Reloader handles hot-reloading of config when files change.
type Reloader struct {
	configDir string
	envFlag   string
	logger    *slog.Logger
	hub       *trace.EventHub

	mu     sync.RWMutex
	config *config.ResolvedConfig

	onReload func(rc *config.ResolvedConfig)
}

// NewReloader creates a new config hot-reloader.
func NewReloader(configDir, envFlag string, initial *config.ResolvedConfig, hub *trace.EventHub, logger *slog.Logger) *Reloader {
	return &Reloader{
		configDir: configDir,
		envFlag:   envFlag,
		config:    initial,
		hub:       hub,
		logger:    logger,
	}
}

// OnReload sets a callback invoked when config is successfully reloaded.
func (r *Reloader) OnReload(fn func(rc *config.ResolvedConfig)) {
	r.onReload = fn
}

// Config returns the current active config.
func (r *Reloader) Config() *config.ResolvedConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.config
}

// HandleChange processes a file change event. It re-validates the full config.
// On success, it swaps the config atomically and calls the reload callback.
// On failure, it keeps the old config and emits an error event via the trace hub.
func (r *Reloader) HandleChange(path string) {
	r.logger.Info("reloading config", "trigger", path)

	sm, smErr := config.NewSecretsManager(r.configDir, r.envFlag)
	if smErr != nil {
		r.logger.Warn("secrets loading failed during reload", "error", smErr.Error())
		return
	}
	rc, errs := config.ValidateAll(r.configDir, r.envFlag, sm)
	if len(errs) > 0 {
		r.logger.Warn("config reload failed — keeping previous config",
			"errors", len(errs),
			"trigger", path,
		)
		for _, e := range errs {
			r.logger.Warn("validation error",
				"file", e.FilePath,
				"message", e.Message,
			)
		}

		// Emit error to trace WebSocket so editor knows
		if r.hub != nil {
			r.hub.Emit(trace.Event{
				Type:  "file:error",
				Error: config.FormatErrors(errs),
				Data: map[string]any{
					"file":  path,
					"count": len(errs),
				},
			})
		}
		return
	}

	r.mu.Lock()
	r.config = rc
	r.mu.Unlock()

	r.logger.Info("config reloaded successfully", "files", rc.FileCount)

	if r.hub != nil {
		r.hub.Emit(trace.Event{
			Type: "config:reloaded",
			Data: map[string]any{
				"files":   rc.FileCount,
				"trigger": path,
			},
		})
	}

	if r.onReload != nil {
		r.onReload(rc)
	}
}
