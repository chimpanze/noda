package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/dberr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.Equal(t, "conflict on user", resp.Error.Message)
}

func TestMapErrorToHTTP_ServiceUnavailableError(t *testing.T) {
	err := &api.ServiceUnavailableError{Service: "db", Cause: fmt.Errorf("connection refused")}
	status, resp := MapErrorToHTTP(err, "", false)
	assert.Equal(t, 503, status)
	assert.Equal(t, "SERVICE_UNAVAILABLE", resp.Error.Code)
	assert.Equal(t, "service unavailable: db", resp.Error.Message)
}

func TestMapErrorToHTTP_ConflictGatedOnDevMode(t *testing.T) {
	cf := &api.ConflictError{Resource: "users", Reason: `duplicate key value violates unique constraint "users_email_key" (email)=(a@b.com)`}
	// production: no raw driver detail
	status, resp := MapErrorToHTTP(cf, "trace-1", false)
	assert.Equal(t, 409, status)
	assert.NotContains(t, resp.Error.Message, "users_email_key")
	assert.NotContains(t, resp.Error.Message, "a@b.com")
	assert.Contains(t, resp.Error.Message, "users") // resource name is fine
	// dev: full detail
	_, devResp := MapErrorToHTTP(cf, "trace-1", true)
	assert.Contains(t, devResp.Error.Message, "users_email_key")
}

func TestMapErrorToHTTP_ServiceUnavailableGatedOnDevMode(t *testing.T) {
	su := &api.ServiceUnavailableError{Service: "db", Cause: errors.New("dial tcp 10.0.0.5:5432: connection refused")}
	_, resp := MapErrorToHTTP(su, "t", false)
	assert.NotContains(t, resp.Error.Message, "10.0.0.5")
	_, devResp := MapErrorToHTTP(su, "t", true)
	assert.Contains(t, devResp.Error.Message, "10.0.0.5")
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

func TestMapErrorToHTTP_NoErrorEdge_WrappedTypedError(t *testing.T) {
	// Mirrors the wrap produced by the no-error-edge path in executor.go
	// when a node's Execute returns a typed error but no error edge is
	// wired: the typed error must survive via %w so it maps to its own
	// HTTP status instead of a generic 500 (#361).
	err := fmt.Errorf("node %q failed with no error edge: %w", "u",
		&api.ValidationError{Field: "file", Message: "type"})
	status, resp := MapErrorToHTTP(err, "t", false)
	assert.Equal(t, 422, status)
	assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
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

// Every mapped driver error must produce a prod-mode body free of driver
// text. MapErrorToHTTP renders ValidationError.Message and
// TimeoutError.Error() unconditionally, so a careless Cause could leak.
func TestMappedDriverErrorsDoNotLeakInProd(t *testing.T) {
	const secret = "SECRET_DRIVER_DETAIL"

	codes := []string{
		"23505", "23503", "23P01", "23502", "23514",
		"22P02", "22003", "22007", "22008", "22001", "22023",
		"40001", "40P01", "53300", "08000", "08003", "08006", "57014",
	}

	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			driverErr := fmt.Errorf("db.find: %w", &pgconn.PgError{
				Code:           code,
				Message:        secret,
				Detail:         secret,
				ConstraintName: secret,
				TableName:      secret,
				SchemaName:     secret,
				Hint:           secret,
			})
			typed := dberr.Classify(driverErr, "users")
			require.NotNil(t, typed, "code %s should classify", code)

			status, resp := MapErrorToHTTP(typed, "trace-1", false)
			assert.NotEqual(t, 500, status, "mapped errors must not be 500")

			body, err := json.Marshal(resp)
			require.NoError(t, err)
			assert.NotContains(t, string(body), secret,
				"prod response leaked driver text for %s: %s", code, body)
		})
	}
}

// Dev mode is expected to expose the cause — that is the point of Unwrap.
func TestDevModeSurfacesCause(t *testing.T) {
	driverErr := fmt.Errorf("db.create: %w", &pgconn.PgError{Code: "23505", Message: "VISIBLE"})
	typed := dberr.Classify(driverErr, "users")
	_, resp := MapErrorToHTTP(typed, "trace-1", true)
	body, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(body), "VISIBLE")
}
