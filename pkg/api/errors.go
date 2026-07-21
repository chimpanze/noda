package api

import (
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
