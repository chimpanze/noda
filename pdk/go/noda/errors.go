package noda

import "fmt"

// HostError is a structured error returned by a Noda host call.
type HostError struct {
	Code    string
	Message string
}

func (e *HostError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
