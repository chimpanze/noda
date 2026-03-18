// Package lifecycle manages ordered startup and reverse-order shutdown of runtime components.
package lifecycle

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Component represents something with a start/stop lifecycle.
type Component interface {
	Name() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

// Lifecycle manages ordered startup and reverse-order shutdown of components.
type Lifecycle struct {
	mu               sync.Mutex
	components       []Component
	started          int // number of successfully started components
	logger           *slog.Logger
	rollbackDeadline time.Duration // deadline for rollback on startup failure (default 30s)
}

// New creates a new Lifecycle manager.
func New(logger *slog.Logger) *Lifecycle {
	return &Lifecycle{logger: logger, rollbackDeadline: 30 * time.Second}
}

// SetRollbackDeadline sets the deadline for rolling back started components
// when a startup failure occurs. Defaults to 30s if not set.
func (l *Lifecycle) SetRollbackDeadline(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rollbackDeadline = d
}

// Register adds a component. Start order = registration order.
// Shutdown order is reverse of registration order.
func (l *Lifecycle) Register(c Component) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.components = append(l.components, c)
}

// StartAll starts all components in registration order.
// If a component fails, already-started components are stopped (rollback).
func (l *Lifecycle) StartAll(ctx context.Context) error {
	l.mu.Lock()
	n := len(l.components)
	components := make([]Component, n)
	copy(components, l.components)
	l.mu.Unlock()

	for i, c := range components {
		l.logger.Info("starting component", "name", c.Name())
		if err := c.Start(ctx); err != nil {
			l.logger.Error("component start failed", "name", c.Name(), "error", err)
			l.mu.Lock()
			l.started = i
			l.mu.Unlock()
			l.StopAll(l.rollbackDeadline)
			return fmt.Errorf("starting %s: %w", c.Name(), err)
		}
	}

	l.mu.Lock()
	l.started = n
	l.mu.Unlock()
	return nil
}

// StopAll stops all started components in reverse registration order.
// The deadline is divided evenly across components, with unused budget
// rolling forward to the next component.
func (l *Lifecycle) StopAll(deadline time.Duration) {
	l.mu.Lock()
	started := l.started
	components := make([]Component, started)
	copy(components, l.components[:started])
	l.started = 0
	l.mu.Unlock()

	if started == 0 {
		return
	}

	remaining := deadline
	perComponent := deadline / time.Duration(started)

	for i := started - 1; i >= 0; i-- {
		c := components[i]
		l.logger.Info("stopping component", "name", c.Name())

		budget := min(perComponent, remaining)

		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), budget)
		if err := c.Stop(ctx); err != nil {
			l.logger.Error("component stop failed", "name", c.Name(), "error", err)
		}
		cancel()

		elapsed := time.Since(start)
		if elapsed < budget {
			remaining -= elapsed
		} else {
			remaining -= budget
		}
	}

	l.logger.Info("shutdown complete")
}
