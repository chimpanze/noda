package server

import (
	"fmt"
	"testing"
	"time"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestMapErrorToHTTP_ValidationError(t *testing.T) {
	err := &api.ValidationError{Field: "email", Message: "must be valid email", Value: "bad"}
	status, resp := MapErrorToHTTP(err, "trace-123")
	assert.Equal(t, 422, status)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	assert.Equal(t, "trace-123", resp.Error.TraceID)
}

func TestMapErrorToHTTP_NotFoundError(t *testing.T) {
	err := &api.NotFoundError{Resource: "task", ID: "42"}
	status, resp := MapErrorToHTTP(err, "trace-456")
	assert.Equal(t, 404, status)
	assert.Equal(t, "NOT_FOUND", resp.Error.Code)
}

func TestMapErrorToHTTP_ConflictError(t *testing.T) {
	err := &api.ConflictError{Resource: "user", Reason: "duplicate email"}
	status, resp := MapErrorToHTTP(err, "")
	assert.Equal(t, 409, status)
	assert.Equal(t, "CONFLICT", resp.Error.Code)
}

func TestMapErrorToHTTP_ServiceUnavailableError(t *testing.T) {
	err := &api.ServiceUnavailableError{Service: "db", Cause: fmt.Errorf("connection refused")}
	status, resp := MapErrorToHTTP(err, "")
	assert.Equal(t, 503, status)
	assert.Equal(t, "SERVICE_UNAVAILABLE", resp.Error.Code)
}

func TestMapErrorToHTTP_TimeoutError(t *testing.T) {
	err := &api.TimeoutError{Duration: 5 * time.Second, Operation: "db query"}
	status, resp := MapErrorToHTTP(err, "")
	assert.Equal(t, 504, status)
	assert.Equal(t, "TIMEOUT", resp.Error.Code)
}

func TestMapErrorToHTTP_UnknownError(t *testing.T) {
	err := fmt.Errorf("something unexpected")
	status, resp := MapErrorToHTTP(err, "trace-789")
	assert.Equal(t, 500, status)
	assert.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
	assert.Equal(t, "trace-789", resp.Error.TraceID)
}
