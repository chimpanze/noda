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
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on field %q: %s", e.Field, e.Message)
}

// TimeoutError indicates an operation exceeded its time limit.
type TimeoutError struct {
	Duration  time.Duration
	Operation string
}

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("timeout after %s: %s", e.Duration, e.Operation)
}

// NotFoundError indicates a requested resource was not found.
type NotFoundError struct {
	Resource string
	ID       string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
}

// ConflictError indicates a conflict with the current state.
type ConflictError struct {
	Resource string
	Reason   string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("conflict on %s: %s", e.Resource, e.Reason)
}
