package plugin

import (
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

// GetService extracts a typed service from the resolved services map.
// It returns a ServiceUnavailableError if the slot is missing or the type assertion fails.
func GetService[T any](services map[string]any, slot string) (T, error) {
	var zero T
	svc, ok := services[slot]
	if !ok {
		return zero, &api.ServiceUnavailableError{Service: slot, Cause: fmt.Errorf("service not configured")}
	}
	typed, ok := svc.(T)
	if !ok {
		return zero, &api.ServiceUnavailableError{Service: slot, Cause: fmt.Errorf("service does not implement expected type")}
	}
	return typed, nil
}
