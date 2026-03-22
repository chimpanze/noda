package api

import "time"

// AuthData holds authentication information for the current request.
type AuthData struct {
	UserID string
	Roles  []string
	Claims map[string]any
}

// TriggerData describes what triggered the current workflow execution.
type TriggerData struct {
	Type      string // "http", "event", "schedule", "websocket", "wasm"
	Timestamp time.Time
	TraceID   string
	RequestID string // X-Request-ID from HTTP request (if present)
}

// ExecutionContext provides node executors with access to input data,
// authentication, expression resolution, and logging.
type ExecutionContext interface {
	Input() any
	Auth() *AuthData // nil if no auth
	Trigger() TriggerData
	Resolve(expression string) (any, error)
	ResolveWithVars(expression string, extraVars map[string]any) (any, error)
	Log(level string, message string, fields map[string]any)
}
