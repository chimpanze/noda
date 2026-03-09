package server

import (
	"errors"

	"github.com/chimpanze/noda/pkg/api"
)

// ErrorResponse wraps api.ErrorData for JSON serialization.
type ErrorResponse struct {
	Error api.ErrorData `json:"error"`
}

// MapErrorToHTTP maps a workflow error to an HTTP status code and standardized error body.
func MapErrorToHTTP(err error, traceID string) (int, ErrorResponse) {
	var (
		valErr *api.ValidationError
		nfErr  *api.NotFoundError
		cfErr  *api.ConflictError
		suErr  *api.ServiceUnavailableError
		toErr  *api.TimeoutError
	)

	switch {
	case errors.As(err, &valErr):
		return 422, ErrorResponse{
			Error: api.ErrorData{
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
			Error: api.ErrorData{
				Code:    "NOT_FOUND",
				Message: nfErr.Error(),
				TraceID: traceID,
			},
		}
	case errors.As(err, &cfErr):
		return 409, ErrorResponse{
			Error: api.ErrorData{
				Code:    "CONFLICT",
				Message: cfErr.Error(),
				TraceID: traceID,
			},
		}
	case errors.As(err, &suErr):
		return 503, ErrorResponse{
			Error: api.ErrorData{
				Code:    "SERVICE_UNAVAILABLE",
				Message: suErr.Error(),
				TraceID: traceID,
			},
		}
	case errors.As(err, &toErr):
		return 504, ErrorResponse{
			Error: api.ErrorData{
				Code:    "TIMEOUT",
				Message: toErr.Error(),
				TraceID: traceID,
			},
		}
	default:
		return 500, ErrorResponse{
			Error: api.ErrorData{
				Code:    "INTERNAL_ERROR",
				Message: "Internal server error",
				TraceID: traceID,
			},
		}
	}
}
