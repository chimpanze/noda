package server

import (
	"errors"
	"fmt"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/pkg/api"
)

// ErrorResponse wraps api.ErrorData for JSON serialization.
type ErrorResponse struct {
	Error api.ErrorData `json:"error"`
}

// MapErrorToHTTP maps a workflow error to an HTTP status code and standardized error body.
// When devMode is true and the error wraps a NodeExecutionError, the response includes
// the full error message, node context, and available node IDs.
func MapErrorToHTTP(err error, traceID string, devMode bool) (int, ErrorResponse) {
	// Extract node context if available (used in dev mode)
	var nodeErr *engine.NodeExecutionError
	hasNodeCtx := errors.As(err, &nodeErr)

	var (
		valErr *api.ValidationError
		nfErr  *api.NotFoundError
		cfErr  *api.ConflictError
		suErr  *api.ServiceUnavailableError
		toErr  *api.TimeoutError
	)

	var status int
	var resp ErrorResponse

	code := api.ErrorCode(err)
	if code == "" {
		// Defensive: api.ErrorCode returns "" for a nil error. Both call
		// sites guard against nil, so this is unreachable, but a nil here
		// must not produce an empty code in the response body.
		code = "INTERNAL_ERROR"
	}

	switch {
	case errors.As(err, &valErr):
		status = 422
		resp = ErrorResponse{
			Error: api.ErrorData{
				Code:    code,
				Message: valErr.Message,
				Details: map[string]any{
					"field": valErr.Field,
					"value": valErr.Value,
				},
				TraceID: traceID,
			},
		}
	case errors.As(err, &nfErr):
		status = 404
		resp = ErrorResponse{
			Error: api.ErrorData{
				Code:    code,
				Message: nfErr.Error(),
				TraceID: traceID,
			},
		}
	case errors.As(err, &cfErr):
		status = 409
		msg := fmt.Sprintf("conflict on %s", cfErr.Resource)
		if devMode {
			msg = cfErr.Error()
		}
		resp = ErrorResponse{
			Error: api.ErrorData{
				Code:    code,
				Message: msg,
				TraceID: traceID,
			},
		}
	case errors.As(err, &suErr):
		status = 503
		msg := fmt.Sprintf("service unavailable: %s", suErr.Service)
		if devMode {
			msg = suErr.Error()
		}
		resp = ErrorResponse{
			Error: api.ErrorData{
				Code:    code,
				Message: msg,
				TraceID: traceID,
			},
		}
	case errors.As(err, &toErr):
		status = 504
		resp = ErrorResponse{
			Error: api.ErrorData{
				Code:    code,
				Message: toErr.Error(),
				TraceID: traceID,
			},
		}
	default:
		status = 500
		msg := "Internal server error"
		if devMode {
			msg = err.Error()
		}
		resp = ErrorResponse{
			Error: api.ErrorData{
				Code:    code,
				Message: msg,
				TraceID: traceID,
			},
		}
	}

	// In dev mode, enrich response with node execution context
	if devMode && hasNodeCtx {
		resp.Error.NodeID = nodeErr.NodeID
		resp.Error.NodeType = nodeErr.NodeType
		if resp.Error.Details == nil {
			resp.Error.Details = map[string]any{}
		}
		details, ok := resp.Error.Details.(map[string]any)
		if ok {
			details["available_nodes"] = nodeErr.AvailableNodes
			resp.Error.Details = details
		}
	}

	return status, resp
}
