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
	started          int  // number of successfully started components
	shuttingDown     bool // set once StopAll has been called; aborts an in-flight StartAll
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
// If StopAll is called concurrently while StartAll is still booting (e.g. a
// SIGTERM during startup), StartAll aborts as soon as it observes the
// shutdown request — checked before each component and once more after the
// loop to close the window on the last component — and returns an error
// instead of finishing the boot with everything left running.
func (l *Lifecycle) StartAll(ctx context.Context) error {
	l.mu.Lock()
	n := len(l.components)
	components := make([]Component, n)
	copy(components, l.components)
	l.mu.Unlock()

	for _, c := range components {
		l.mu.Lock()
		down := l.shuttingDown
		l.mu.Unlock()
		if down {
			l.rollback()
			return fmt.Errorf("startup aborted: shutdown requested")
		}

		l.logger.Info("starting component", "name", c.Name())
		err := c.Start(ctx)

		// Decide atomically, under the same lock a concurrent StopAll takes,
		// whether this success counts. If a concurrent StopAll already fired
		// while c.Start was in flight, l.started was reset to 0 and the
		// components it stopped no longer correspond to a `components[:started]`
		// prefix that includes c — so we must NOT bump the counter here (that
		// would desync the counter from actual component identity and cause
		// StopAll to stop the wrong component next time). Instead we stop c
		// ourselves below.
		l.mu.Lock()
		down = l.shuttingDown
		if err == nil && !down {
			l.started++
		}
		l.mu.Unlock()

		if err != nil {
			l.logger.Error("component start failed", "name", c.Name(), "error", err)
			l.rollback()
			return fmt.Errorf("starting %s: %w", c.Name(), err)
		}
		if down {
			l.logger.Info("stopping component started during concurrent shutdown", "name", c.Name())
			stopCtx, cancel := context.WithTimeout(context.Background(), l.rollbackDeadline)
			if err := c.Stop(stopCtx); err != nil {
				l.logger.Error("component stop failed", "name", c.Name(), "error", err)
			}
			cancel()
			l.rollback() // defensive: stop anything else StopAll had already tracked
			return fmt.Errorf("startup aborted: shutdown requested")
		}
	}

	// Re-check after the loop: closes the window where shutdown is requested
	// while (or just after) the last component finishes starting.
	l.mu.Lock()
	down := l.shuttingDown
	l.mu.Unlock()
	if down {
		l.rollback()
		return fmt.Errorf("startup aborted: shutdown requested")
	}
	return nil
}

// rollback stops whatever has started so far, bounded by rollbackDeadline.
func (l *Lifecycle) rollback() {
	rbCtx, rbCancel := context.WithTimeout(context.Background(), l.rollbackDeadline)
	defer rbCancel()
	l.StopAll(rbCtx)
}

// StopAll stops all started components in reverse registration order.
// The parent ctx's deadline (or rollbackDeadline if it has none) is
// divided across components, with unused budget rolling forward to
// the next component. Parent cancellation propagates to each component's
// Stop call.
func (l *Lifecycle) StopAll(parent context.Context) {
	l.mu.Lock()
	l.shuttingDown = true
	started := l.started
	components := make([]Component, started)
	copy(components, l.components[:started])
	l.started = 0
	l.mu.Unlock()

	if started == 0 {
		return
	}

	var totalBudget time.Duration
	if deadline, ok := parent.Deadline(); ok {
		totalBudget = time.Until(deadline)
		if totalBudget < 0 {
			totalBudget = 0
		}
	} else {
		totalBudget = l.rollbackDeadline
	}

	perComponent := totalBudget / time.Duration(started)
	remaining := totalBudget

	for i := started - 1; i >= 0; i-- {
		c := components[i]
		l.logger.Info("stopping component", "name", c.Name())

		budget := perComponent
		if budget > remaining {
			budget = remaining
		}

		start := time.Now()
		ctx, cancel := context.WithTimeout(parent, budget)
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
