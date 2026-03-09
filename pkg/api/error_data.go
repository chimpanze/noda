package api

// ErrorData is the standardized error output shape used in workflow error
// responses and HTTP error bodies.
type ErrorData struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	NodeID   string `json:"node_id,omitempty"`
	NodeType string `json:"node_type,omitempty"`
	TraceID  string `json:"trace_id,omitempty"`
	Details  any    `json:"details,omitempty"`
}
