package devmode

import (
	"context"
	"log/slog"
	"time"
)

// Stoppable is something that can be stopped gracefully.
type Stoppable interface {
	Stop()
}

// ContextStoppable can be stopped with a context (e.g., Wasm runtime).
type ContextStoppable interface {
	StopAll(ctx context.Context)
}

// Shutdownable can be shut down (e.g., HTTP server).
type Shutdownable interface {
	Stop() error
}

// TraceShutdownable flushes and shuts down (e.g., OTel provider).
type TraceShutdownable interface {
	Shutdown(ctx context.Context) error
}

// ShutdownSequence performs ordered graceful shutdown of all components.
func ShutdownSequence(
	logger *slog.Logger,
	deadline time.Duration,
	server Shutdownable,
	scheduler Stoppable,
	wasm ContextStoppable,
	watcher *Watcher,
	tracer TraceShutdownable,
) {
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	logger.Info("shutdown: stopping HTTP server")
	if server != nil {
		if err := server.Stop(); err != nil {
			logger.Error("shutdown: server stop error", "error", err.Error())
		}
	}

	logger.Info("shutdown: stopping scheduler")
	if scheduler != nil {
		scheduler.Stop()
	}

	logger.Info("shutdown: stopping wasm runtime")
	if wasm != nil {
		wasm.StopAll(ctx)
	}

	logger.Info("shutdown: stopping file watcher")
	if watcher != nil {
		watcher.Stop()
	}

	logger.Info("shutdown: flushing telemetry")
	if tracer != nil {
		if err := tracer.Shutdown(ctx); err != nil {
			logger.Error("shutdown: tracer flush error", "error", err.Error())
		}
	}

	logger.Info("shutdown: complete")
}
