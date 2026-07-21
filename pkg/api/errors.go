package api

import (
	"errors"
	"fmt"
	"time"
)

// ServiceUnavailableError indicates a required service is not reachable.
type ServiceUnavailableError struct {
	Service string
	Cause   error
}

func (e *ServiceUnavailableError) Error() string {
	return fmt.Sprintf("service unavailable: %s: %v", e.Service, e.Cause)
}

func (e *ServiceUnavailableError) Unwrap() error {
	return e.Cause
}

// ValidationError indicates invalid input data.
type ValidationError struct {
	Field   string
	Message string
	Value   any
	// Cause is the underlying error, when this was derived from one.
	// Message must stay free of driver text: internal/server renders it
	// to prod clients unconditionally.
	Cause error
}

func (e *ValidationError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("validation error on field %q: %s: %v", e.Field, e.Message, e.Cause)
	}
	return fmt.Sprintf("validation error on field %q: %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error { return e.Cause }

// TimeoutError indicates an operation exceeded its time limit.
type TimeoutError struct {
	Duration  time.Duration
	Operation string
	// Cause is deliberately not rendered by Error(): internal/server
	// sends TimeoutError.Error() to prod clients unconditionally.
	// Recover it with errors.As / errors.Unwrap instead.
	Cause error
}

func (e *TimeoutError) Error() string {
	if e.Duration <= 0 {
		return fmt.Sprintf("timeout: %s", e.Operation)
	}
	return fmt.Sprintf("timeout after %s: %s", e.Duration, e.Operation)
}

func (e *TimeoutError) Unwrap() error { return e.Cause }

// NotFoundError indicates a requested resource was not found.
type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	if e.ID == "" {
		return fmt.Sprintf("%s not found", e.Resource)
	}
	return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
}

// ConflictError indicates a conflict with the current state.
type ConflictError struct {
	Resource string
	Reason   string
	Cause    error
}

func (e *ConflictError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("conflict on %s: %s: %v", e.Resource, e.Reason, e.Cause)
	}
	return fmt.Sprintf("conflict on %s: %s", e.Resource, e.Reason)
}

func (e *ConflictError) Unwrap() error { return e.Cause }

// ErrorCode returns the stable symbolic code for err, matching the `code`
// field of the HTTP error body and of a node's error-edge output.
//
// It is the shared source for those two surfaces: internal/server's
// workflow-error HTTP body and internal/engine's node error-edge payload
// both derive their code from here, so those two cannot drift from each
// other. It is not the only vocabulary the server emits — errorHandler in
// internal/server/server.go and literals in internal/server/routes.go mint
// additional codes (e.g. METHOD_NOT_ALLOWED, RATE_LIMITED) for failures
// that never reach a typed error, and internal/wasm/hostapi.go has its own
// vocabulary for the Wasm host-call boundary.
//
// Evaluation order matches MapErrorToHTTP's switch. No error chain
// currently matches two of these types, so the order is not load-bearing,
// but it is preserved rather than depended upon.
//
// A nil error returns the empty string, never INTERNAL_ERROR.
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if _, ok := errors.AsType[*ValidationError](err); ok {
		return "VALIDATION_ERROR"
	}
	if _, ok := errors.AsType[*NotFoundError](err); ok {
		return "NOT_FOUND"
	}
	if _, ok := errors.AsType[*ConflictError](err); ok {
		return "CONFLICT"
	}
	if _, ok := errors.AsType[*ServiceUnavailableError](err); ok {
		return "SERVICE_UNAVAILABLE"
	}
	if _, ok := errors.AsType[*TimeoutError](err); ok {
		return "TIMEOUT"
	}
	return "INTERNAL_ERROR"
}
