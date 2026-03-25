package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// MessageContext provides context about the message being processed.
type MessageContext struct {
	WorkerID  string
	MessageID string
	TraceID   string
	Topic     string
	Group     string
	Logger    *slog.Logger
}

// Handler is a function that processes a message.
type Handler func(ctx context.Context) error

// Middleware wraps a handler with cross-cutting concerns.
type Middleware interface {
	Name() string
	Wrap(next Handler, msgCtx *MessageContext) Handler
}

// LogMiddleware logs message processing start, completion, and timing.
type LogMiddleware struct{}

func (m *LogMiddleware) Name() string { return "worker.log" }

func (m *LogMiddleware) Wrap(next Handler, msgCtx *MessageContext) Handler {
	return func(ctx context.Context) error {
		start := time.Now()
		msgCtx.Logger.Info("worker.log: processing started",
			"worker_id", msgCtx.WorkerID,
			"message_id", msgCtx.MessageID,
			"trace_id", msgCtx.TraceID,
			"topic", msgCtx.Topic,
		)

		err := next(ctx)
		duration := time.Since(start)

		if err != nil {
			msgCtx.Logger.Error("worker.log: processing failed",
				"worker_id", msgCtx.WorkerID,
				"message_id", msgCtx.MessageID,
				"trace_id", msgCtx.TraceID,
				"duration", duration.String(),
				"error", err.Error(),
			)
		} else {
			msgCtx.Logger.Info("worker.log: processing completed",
				"worker_id", msgCtx.WorkerID,
				"message_id", msgCtx.MessageID,
				"trace_id", msgCtx.TraceID,
				"duration", duration.String(),
			)
		}
		return err
	}
}

// TimeoutMiddleware enforces a deadline on message processing.
type TimeoutMiddleware struct {
	Timeout time.Duration
}

func (m *TimeoutMiddleware) Name() string { return "worker.timeout" }

func (m *TimeoutMiddleware) Wrap(next Handler, msgCtx *MessageContext) Handler {
	return func(ctx context.Context) error {
		timeout := m.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		done := make(chan error, 1)
		go func() {
			done <- next(ctx)
		}()

		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			msgCtx.Logger.Warn("worker.timeout: processing timed out",
				"worker_id", msgCtx.WorkerID,
				"message_id", msgCtx.MessageID,
				"trace_id", msgCtx.TraceID,
				"timeout", timeout.String(),
			)
			return fmt.Errorf("worker.timeout: processing exceeded %s", timeout)
		}
	}
}

// RecoverMiddleware catches panics during message processing.
type RecoverMiddleware struct{}

func (m *RecoverMiddleware) Name() string { return "worker.recover" }

func (m *RecoverMiddleware) Wrap(next Handler, msgCtx *MessageContext) Handler {
	return func(ctx context.Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				msgCtx.Logger.Error("worker.recover: panic caught",
					"worker_id", msgCtx.WorkerID,
					"message_id", msgCtx.MessageID,
					"trace_id", msgCtx.TraceID,
					"panic", fmt.Sprintf("%v", r),
				)
				err = fmt.Errorf("worker.recover: panic: %v", r)
			}
		}()
		return next(ctx)
	}
}

// DefaultMiddleware returns the standard set of worker middleware.
func DefaultMiddleware(timeout time.Duration) []Middleware {
	return []Middleware{
		&RecoverMiddleware{},
		&LogMiddleware{},
		&TimeoutMiddleware{Timeout: timeout},
	}
}

// ResolveMiddleware resolves middleware names to implementations.
func ResolveMiddleware(names []string, timeout time.Duration) []Middleware {
	var result []Middleware
	for _, name := range names {
		switch name {
		case "worker.log":
			result = append(result, &LogMiddleware{})
		case "worker.timeout":
			result = append(result, &TimeoutMiddleware{Timeout: timeout})
		case "worker.recover":
			result = append(result, &RecoverMiddleware{})
		default:
			slog.Warn("unknown worker middleware, skipping", "name", name)
		}
	}
	return result
}
