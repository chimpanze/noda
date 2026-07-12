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

// PanicShieldMiddleware runs the handler in a child goroutine and converts
// panics to errors (recover() cannot cross goroutines, so the outer
// RecoverMiddleware can't catch them). It no longer applies its own
// timeout: processMessage's context owns the per-message deadline (#285).
type PanicShieldMiddleware struct{}

func (m *PanicShieldMiddleware) Name() string { return "worker.timeout" } // config-name compat

func (m *PanicShieldMiddleware) Wrap(next Handler, msgCtx *MessageContext) Handler {
	return func(ctx context.Context) error {
		done := make(chan error, 1)
		go func() {
			// recover() only catches panics on its own goroutine, so the outer
			// RecoverMiddleware (parent goroutine) cannot catch a panic raised
			// here. Convert it to an error to avoid crashing the worker process.
			defer func() {
				if r := recover(); r != nil {
					done <- fmt.Errorf("worker.timeout: panic: %v", r)
				}
			}()
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
			)
			return fmt.Errorf("worker.timeout: processing exceeded deadline: %w", ctx.Err())
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
func DefaultMiddleware() []Middleware {
	return []Middleware{
		&RecoverMiddleware{},
		&LogMiddleware{},
		&PanicShieldMiddleware{},
	}
}

// ResolveMiddleware resolves middleware names to implementations.
func ResolveMiddleware(names []string) []Middleware {
	var result []Middleware
	for _, name := range names {
		switch name {
		case "worker.log":
			result = append(result, &LogMiddleware{})
		case "worker.timeout":
			result = append(result, &PanicShieldMiddleware{})
		case "worker.recover":
			result = append(result, &RecoverMiddleware{})
		default:
			slog.Warn("unknown worker middleware, skipping", "name", name)
		}
	}
	return result
}
