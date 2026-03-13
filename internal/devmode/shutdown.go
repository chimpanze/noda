package devmode

import (
	"context"
	"log/slog"
	"time"
)

// Stoppable is something that can be stopped gracefully (e.g., scheduler).
type Stoppable interface {
	Stop(ctx context.Context) error
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

// ServiceShutdownable shuts down all services (e.g., DB, Redis connections).
type ServiceShutdownable interface {
	ShutdownAll() []error
}

// ShutdownSequence performs ordered graceful shutdown of all components.
// Order: server → workers → scheduler → wasm → watcher → services → tracer
func ShutdownSequence(
	logger *slog.Logger,
	deadline time.Duration,
	server Shutdownable,
	scheduler Stoppable,
	workers Stoppable,
	wasm ContextStoppable,
	watcher *Watcher,
	connMgr Stoppable,
	services ServiceShutdownable,
	tracer TraceShutdownable,
) {
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	// 1. Stop accepting new connections
	logger.Info("shutdown: stopping HTTP server")
	if server != nil {
		if err := server.Stop(); err != nil {
			logger.Error("shutdown: server stop error", "error", err.Error())
		}
	}

	// 2. Stop workers (drains in-flight messages)
	logger.Info("shutdown: stopping workers")
	if workers != nil {
		if err := workers.Stop(ctx); err != nil {
			logger.Error("shutdown: workers stop error", "error", err.Error())
		}
	}

	// 3. Stop scheduler
	logger.Info("shutdown: stopping scheduler")
	if scheduler != nil {
		if err := scheduler.Stop(ctx); err != nil {
			logger.Error("shutdown: scheduler stop error", "error", err.Error())
		}
	}

	// 4. Stop Wasm runtimes (calls shutdown on each module)
	logger.Info("shutdown: stopping wasm runtimes")
	if wasm != nil {
		wasm.StopAll(ctx)
	}

	// 5. Stop file watcher (dev mode only)
	if watcher != nil {
		logger.Info("shutdown: stopping file watcher")
		watcher.Stop()
	}

	// 6. Close WebSocket/SSE connections
	if connMgr != nil {
		logger.Info("shutdown: closing connections")
		if err := connMgr.Stop(ctx); err != nil {
			logger.Error("shutdown: connmgr stop error", "error", err.Error())
		}
	}

	// 7. Close service connections (DB, Redis, storage)
	if services != nil {
		logger.Info("shutdown: closing services")
		if errs := services.ShutdownAll(); len(errs) > 0 {
			for _, err := range errs {
				logger.Error("shutdown: service error", "error", err.Error())
			}
		}
	}

	// 8. Flush telemetry
	logger.Info("shutdown: flushing telemetry")
	if tracer != nil {
		if err := tracer.Shutdown(ctx); err != nil {
			logger.Error("shutdown: tracer flush error", "error", err.Error())
		}
	}

	logger.Info("shutdown: complete")
}
