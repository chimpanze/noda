package noda

import "encoding/json"

// InitInput is the input provided to the initialize export.
type InitInput struct {
	Encoding string                     `json:"encoding" msgpack:"encoding"`
	Config   map[string]any             `json:"config" msgpack:"config"`
	Services map[string]ServiceManifest `json:"services,omitempty" msgpack:"services,omitempty"`
}

// ServiceManifest describes an available service and its operations.
type ServiceManifest struct {
	Operations []string `json:"operations" msgpack:"operations"`
}

// TickInput is the input provided to the tick export.
type TickInput struct {
	DT               int64                     `json:"dt" msgpack:"dt"`
	Timestamp        int64                     `json:"timestamp" msgpack:"timestamp"`
	ClientMessages   []ClientMessage           `json:"client_messages,omitempty" msgpack:"client_messages,omitempty"`
	IncomingWS       []IncomingWSMsg           `json:"incoming_ws,omitempty" msgpack:"incoming_ws,omitempty"`
	ConnectionEvents []ConnectionEvent         `json:"connection_events,omitempty" msgpack:"connection_events,omitempty"`
	Commands         []Command                 `json:"commands,omitempty" msgpack:"commands,omitempty"`
	Responses        map[string]*AsyncResponse `json:"responses,omitempty" msgpack:"responses,omitempty"`
	Timers           []string                  `json:"timers,omitempty" msgpack:"timers,omitempty"`
}

// ClientMessage represents a message from a connected client.
type ClientMessage struct {
	ClientID string          `json:"client_id" msgpack:"client_id"`
	Data     json.RawMessage `json:"data" msgpack:"data"`
}

// IncomingWSMsg represents a message received on a managed WebSocket connection.
type IncomingWSMsg struct {
	Connection string          `json:"connection" msgpack:"connection"`
	Data       json.RawMessage `json:"data" msgpack:"data"`
}

// ConnectionEvent represents a WebSocket connection lifecycle event.
type ConnectionEvent struct {
	Connection string `json:"connection" msgpack:"connection"`
	Event      string `json:"event" msgpack:"event"` // "connected", "disconnected", "error"
	Error      string `json:"error,omitempty" msgpack:"error,omitempty"`
}

// Command represents a command sent to the module via wasm.send.
type Command struct {
	Data json.RawMessage `json:"data" msgpack:"data"`
}

// AsyncResponse is the result of a prior CallAsync, keyed by label.
type AsyncResponse struct {
	Status string      `json:"status" msgpack:"status"`
	Data   any         `json:"data,omitempty" msgpack:"data,omitempty"`
	Error  *AsyncError `json:"error,omitempty" msgpack:"error,omitempty"`
}

// OK returns true if the async response completed successfully.
func (r *AsyncResponse) OK() bool {
	return r.Status == "ok"
}

// AsyncError contains error details from a failed async call.
type AsyncError struct {
	Code    string `json:"code" msgpack:"code"`
	Message string `json:"message" msgpack:"message"`
}

// hostCallRequest is the wire format for noda_call and noda_call_async.
type hostCallRequest struct {
	Service   string `json:"service" msgpack:"service"`
	Operation string `json:"operation" msgpack:"operation"`
	Payload   any    `json:"payload" msgpack:"payload"`
	Label     string `json:"label,omitempty" msgpack:"label,omitempty"`
}
