package api

// ErrorData is the standardized error output shape used in workflow error responses.
type ErrorData struct {
	Code     string
	Message  string
	NodeID   string
	NodeType string
	Details  any
}
