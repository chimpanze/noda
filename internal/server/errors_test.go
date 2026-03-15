package server

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
)

func TestMapErrorToHTTP_ValidationError(t *testing.T) {
	err := &api.ValidationError{Field: "email", Message: "must be valid email", Value: "bad"}
	status, resp := MapErrorToHTTP(err, "trace-123", false)
	assert.Equal(t, 422, status)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	assert.Equal(t, "trace-123", resp.Error.TraceID)
}

func TestMapErrorToHTTP_NotFoundError(t *testing.T) {
	err := &api.NotFoundError{Resource: "task", ID: "42"}
	status, resp := MapErrorToHTTP(err, "trace-456", false)
	assert.Equal(t, 404, status)
	assert.Equal(t, "NOT_FOUND", resp.Error.Code)
}

func TestMapErrorToHTTP_ConflictError(t *testing.T) {
	err := &api.ConflictError{Resource: "user", Reason: "duplicate email"}
	status, resp := MapErrorToHTTP(err, "", false)
	assert.Equal(t, 409, status)
	assert.Equal(t, "CONFLICT", resp.Error.Code)
}

func TestMapErrorToHTTP_ServiceUnavailableError(t *testing.T) {
	err := &api.ServiceUnavailableError{Service: "db", Cause: fmt.Errorf("connection refused")}
	status, resp := MapErrorToHTTP(err, "", false)
	assert.Equal(t, 503, status)
	assert.Equal(t, "SERVICE_UNAVAILABLE", resp.Error.Code)
}

func TestMapErrorToHTTP_TimeoutError(t *testing.T) {
	err := &api.TimeoutError{Duration: 5 * time.Second, Operation: "db query"}
	status, resp := MapErrorToHTTP(err, "", false)
	assert.Equal(t, 504, status)
	assert.Equal(t, "TIMEOUT", resp.Error.Code)
}

func TestMapErrorToHTTP_UnknownError(t *testing.T) {
	err := fmt.Errorf("something unexpected")
	status, resp := MapErrorToHTTP(err, "trace-789", false)
	assert.Equal(t, 500, status)
	assert.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
	assert.Equal(t, "trace-789", resp.Error.TraceID)
	assert.Equal(t, "Internal server error", resp.Error.Message)
}

func TestMapErrorToHTTP_DevMode_NodeExecutionError(t *testing.T) {
	inner := fmt.Errorf("resolve %q: evaluation error", "{{ nodes.generate_token.token }}")
	nodeErr := &engine.NodeExecutionError{
		NodeID:         "respond",
		NodeType:       "response.json",
		Err:            inner,
		AvailableNodes: []string{"create_player", "hash_password"},
	}

	status, resp := MapErrorToHTTP(nodeErr, "trace-dev", true)
	assert.Equal(t, 500, status)
	assert.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
	assert.Contains(t, resp.Error.Message, "evaluation error")
	assert.Equal(t, "respond", resp.Error.NodeID)
	assert.Equal(t, "response.json", resp.Error.NodeType)
	details := resp.Error.Details.(map[string]any)
	assert.Equal(t, []string{"create_player", "hash_password"}, details["available_nodes"])
}

func TestMapErrorToHTTP_DevMode_GenericError(t *testing.T) {
	err := fmt.Errorf("something broke internally")
	status, resp := MapErrorToHTTP(err, "trace-dev2", true)
	assert.Equal(t, 500, status)
	assert.Equal(t, "something broke internally", resp.Error.Message)
	assert.Empty(t, resp.Error.NodeID)
}

func TestMapErrorToHTTP_ProdMode_NodeExecutionError(t *testing.T) {
	nodeErr := &engine.NodeExecutionError{
		NodeID:         "respond",
		NodeType:       "response.json",
		Err:            fmt.Errorf("some failure"),
		AvailableNodes: []string{"step1"},
	}

	status, resp := MapErrorToHTTP(nodeErr, "trace-prod", false)
	assert.Equal(t, 500, status)
	assert.Equal(t, "Internal server error", resp.Error.Message)
	assert.Empty(t, resp.Error.NodeID)
	assert.Nil(t, resp.Error.Details)
}

func TestMapErrorToHTTP_DevMode_WrappedTypedError(t *testing.T) {
	valErr := &api.ValidationError{Field: "name", Message: "required", Value: ""}
	nodeErr := &engine.NodeExecutionError{
		NodeID:         "validate_input",
		NodeType:       "transform.validate",
		Err:            valErr,
		AvailableNodes: []string{"fetch_user"},
	}

	status, resp := MapErrorToHTTP(nodeErr, "trace-wrapped", true)
	// Should still match the ValidationError case
	assert.Equal(t, 422, status)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	// Dev mode should enrich with node context
	assert.Equal(t, "validate_input", resp.Error.NodeID)
	assert.Equal(t, "transform.validate", resp.Error.NodeType)
	details := resp.Error.Details.(map[string]any)
	assert.Equal(t, []string{"fetch_user"}, details["available_nodes"])
}

func TestMapErrorToHTTP_DevMode_WrappedTypedError_NoNodeCtx(t *testing.T) {
	// A typed error without NodeExecutionError wrapping — dev mode shouldn't add node fields
	valErr := &api.ValidationError{Field: "email", Message: "invalid", Value: "bad"}
	status, resp := MapErrorToHTTP(valErr, "trace-plain", true)
	assert.Equal(t, 422, status)
	assert.Empty(t, resp.Error.NodeID)

	// errors.As should still find ValidationError through NodeExecutionError.Unwrap
	nodeWrapped := &engine.NodeExecutionError{
		NodeID: "n1", NodeType: "t1", Err: valErr, AvailableNodes: nil,
	}
	assert.True(t, errors.As(nodeWrapped, &valErr))
}
