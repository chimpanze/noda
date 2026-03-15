package wasm

import "time"

// ModuleConfig configures a Wasm module runtime.
type ModuleConfig struct {
	Name        string         // Module name (key in wasm_runtimes)
	ModulePath  string         // Path to .wasm binary
	TickRate    int            // Ticks per second (Hz), 1-120
	Encoding    string         // "json" (default) or "msgpack"
	Services    []string       // Allowed service instance names
	Connections []string       // Allowed connection endpoint names
	AllowHTTP   []string       // Whitelisted HTTP hosts
	AllowWS     []string       // Whitelisted WebSocket hosts
	Config      map[string]any // Opaque config passed to initialize
	MemoryPages uint32         // Max memory pages (0 = default)
}

// TickInput is the data passed to a module's tick export.
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

// ClientMessage is a message from a client connected to a Noda endpoint.
type ClientMessage struct {
	Endpoint string `json:"endpoint" msgpack:"endpoint"`
	Channel  string `json:"channel" msgpack:"channel"`
	UserID   string `json:"user_id" msgpack:"user_id"`
	Data     any    `json:"data" msgpack:"data"`
}

// IncomingWSMsg is a message from an outbound WebSocket connection.
type IncomingWSMsg struct {
	Connection string `json:"connection" msgpack:"connection"`
	Data       any    `json:"data" msgpack:"data"`
}

// ConnectionEvent represents a connect/disconnect/reconnect event.
type ConnectionEvent struct {
	Endpoint   string `json:"endpoint,omitempty" msgpack:"endpoint,omitempty"`
	Channel    string `json:"channel,omitempty" msgpack:"channel,omitempty"`
	UserID     string `json:"user_id,omitempty" msgpack:"user_id,omitempty"`
	Connection string `json:"connection,omitempty" msgpack:"connection,omitempty"`
	Event      string `json:"event" msgpack:"event"`
	Reason     string `json:"reason,omitempty" msgpack:"reason,omitempty"`
}

// Command is data sent by workflows via wasm.send.
type Command struct {
	Source string `json:"source" msgpack:"source"`
	Data   any    `json:"data" msgpack:"data"`
}

// AsyncResponse is the result of a noda_call_async call.
type AsyncResponse struct {
	Status string      `json:"status" msgpack:"status"` // "ok" or "error"
	Data   any         `json:"data,omitempty" msgpack:"data,omitempty"`
	Error  *AsyncError `json:"error,omitempty" msgpack:"error,omitempty"`
}

// AsyncError is an error from an async call.
type AsyncError struct {
	Code      string `json:"code" msgpack:"code"`
	Message   string `json:"message" msgpack:"message"`
	Operation string `json:"operation,omitempty" msgpack:"operation,omitempty"`
}

// HostCallRequest is the input to noda_call / noda_call_async.
type HostCallRequest struct {
	Service   string `json:"service" msgpack:"service"`
	Operation string `json:"operation" msgpack:"operation"`
	Payload   any    `json:"payload" msgpack:"payload"`
	Label     string `json:"label,omitempty" msgpack:"label,omitempty"` // async only
}

// InitializeInput is passed to the module's initialize export.
type InitializeInput struct {
	Encoding string                     `json:"encoding" msgpack:"encoding"`
	Config   map[string]any             `json:"config" msgpack:"config"`
	Services map[string]ServiceManifest `json:"services" msgpack:"services"`
}

// ServiceManifest describes a service available to the module.
type ServiceManifest struct {
	Type       string   `json:"type" msgpack:"type"`
	Operations []string `json:"operations,omitempty" msgpack:"operations,omitempty"`
}

// GatewayConfig configures an outbound WebSocket connection.
type GatewayConfig struct {
	ID                string            `json:"id"`
	URL               string            `json:"url"`
	Headers           map[string]string `json:"headers,omitempty"`
	HeartbeatInterval time.Duration     `json:"heartbeat_interval,omitempty"`
	HeartbeatPayload  any               `json:"heartbeat_payload,omitempty"`
	Reconnect         *ReconnectConfig  `json:"reconnect,omitempty"`
}

// ReconnectConfig configures reconnection behavior.
type ReconnectConfig struct {
	Enabled      bool          `json:"enabled"`
	MaxAttempts  int           `json:"max_attempts"`
	Backoff      string        `json:"backoff"` // "exponential" or "linear"
	InitialDelay time.Duration `json:"initial_delay"`
}
