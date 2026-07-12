package devmode

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/trace"
)

// Reloader handles hot-reloading of config when files change.
type Reloader struct {
	configDir    string
	envFlag      string
	logger       *slog.Logger
	hub          *trace.EventHub
	shuttingDown atomic.Bool

	mu     sync.RWMutex
	config *config.ResolvedConfig

	onReload func(rc *config.ResolvedConfig)

	reloadMu sync.Mutex // serializes the whole HandleChange (latest wins);
	// also the shutdown barrier — Shutdown drains it to await an in-flight reload
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
// Concurrent invocations of HandleChange (e.g. an editor save racing the
// watcher's debounce) are serialized via reloadMu, so the last reload to run
// always wins.
//
// Shutdown marks the reloader as shutting down and awaits the in-flight-reload
// barrier, bounded by ctx, so no onReload callback fires into a closing system.
//
// The flag is set before the barrier is taken: any HandleChange that acquires
// reloadMu after this point observes shuttingDown at the post-lock re-check and
// bails without firing onReload. Draining reloadMu (a reload holds it across the
// swap and onReload) guarantees any truly in-flight reload has fully completed
// before Shutdown returns the barrier.
//
// If ctx expires first (e.g. a reload is stuck in config.ValidateAll),
// Shutdown returns early without waiting for the barrier. This is still safe:
// the shuttingDown flag already made the in-flight reload's post-lock
// re-check bail before firing onReload, so nothing fires into the closing
// system — the in-flight reload just keeps running in the background (#287).
func (r *Reloader) Shutdown(ctx context.Context) {
	r.shuttingDown.Store(true)
	acquired := make(chan struct{})
	go func() {
		r.reloadMu.Lock()
		r.reloadMu.Unlock() //nolint:staticcheck // intentional barrier: drain to await in-flight reload
		close(acquired)
	}()
	select {
	case <-acquired:
	case <-ctx.Done():
		r.logger.Warn("devmode: shutdown proceeding without reload barrier (in-flight reload still running)")
	}
}

func (r *Reloader) HandleChange(path string) {
	if r.shuttingDown.Load() {
		return
	}

	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	// Re-check after acquiring the serialization lock (shutdown may have begun
	// while we queued behind another reload).
	if r.shuttingDown.Load() {
		return
	}

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

	// Re-check after validation (which is slow) before swapping / firing
	// onReload — shutdown may have begun while validation was running.
	if r.shuttingDown.Load() {
		return
	}

	// Hold mu across the swap and the onReload callback so that Config()
	// readers never observe the new config while onReload is still running
	// (see TestReloader_ConfigVisibleOnlyAfterOnReloadCompletes). reloadMu
	// already serializes concurrent HandleChange calls; mu here only guards
	// visibility for readers of Config().
	r.mu.Lock()
	defer r.mu.Unlock()

	r.config = rc
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
