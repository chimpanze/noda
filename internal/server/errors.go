package server

import (
	"errors"

	"github.com/chimpanze/noda/pkg/api"
)

// ErrorData is the standardized error body.
type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
}

// ErrorResponse wraps ErrorData for JSON serialization.
type ErrorResponse struct {
	Error ErrorData `json:"error"`
}

// MapErrorToHTTP maps a workflow error to an HTTP status code and standardized error body.
func MapErrorToHTTP(err error, traceID string) (int, ErrorResponse) {
	var (
		valErr  *api.ValidationError
		nfErr   *api.NotFoundError
		cfErr   *api.ConflictError
		suErr   *api.ServiceUnavailableError
		toErr   *api.TimeoutError
	)

	switch {
	case errors.As(err, &valErr):
		return 422, ErrorResponse{
			Error: ErrorData{
				Code:    "VALIDATION_ERROR",
				Message: valErr.Message,
				Details: map[string]any{
					"field": valErr.Field,
					"value": valErr.Value,
				},
				TraceID: traceID,
			},
		}
	case errors.As(err, &nfErr):
		return 404, ErrorResponse{
			Error: ErrorData{
				Code:    "NOT_FOUND",
				Message: nfErr.Error(),
				TraceID: traceID,
			},
		}
	case errors.As(err, &cfErr):
		return 409, ErrorResponse{
			Error: ErrorData{
				Code:    "CONFLICT",
				Message: cfErr.Error(),
				TraceID: traceID,
			},
		}
	case errors.As(err, &suErr):
		return 503, ErrorResponse{
			Error: ErrorData{
				Code:    "SERVICE_UNAVAILABLE",
				Message: suErr.Error(),
				TraceID: traceID,
			},
		}
	case errors.As(err, &toErr):
		return 504, ErrorResponse{
			Error: ErrorData{
				Code:    "TIMEOUT",
				Message: toErr.Error(),
				TraceID: traceID,
			},
		}
	default:
		return 500, ErrorResponse{
			Error: ErrorData{
				Code:    "INTERNAL_ERROR",
				Message: "Internal server error",
				TraceID: traceID,
			},
		}
	}
}
