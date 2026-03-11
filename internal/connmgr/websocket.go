package connmgr

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

const (
	defaultPingInterval   = 30 * time.Second
	defaultMaxMessageSize = 64 * 1024 // 64KB
)

// WebSocketConfig holds configuration for a WebSocket endpoint.
type WebSocketConfig struct {
	Endpoint       string
	Path           string
	ChannelPattern string // e.g., "doc.{{ request.params.doc_id }}"
	PingInterval   time.Duration
	MaxMessageSize int64
	MaxPerChannel  int

	OnConnect    string // workflow ID
	OnMessage    string // workflow ID
	OnDisconnect string // workflow ID
}

// WebSocketHandler manages a single WebSocket endpoint.
type WebSocketHandler struct {
	config     WebSocketConfig
	manager    *Manager
	runner     api.WorkflowRunner
	compiler   *expr.Compiler
	logger     *slog.Logger
	paramNames []string // route param names extracted from path pattern
}

// NewWebSocketHandler creates a handler for a WebSocket endpoint.
func NewWebSocketHandler(cfg WebSocketConfig, mgr *Manager, runner api.WorkflowRunner, compiler *expr.Compiler, logger *slog.Logger) *WebSocketHandler {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.PingInterval == 0 {
		cfg.PingInterval = defaultPingInterval
	}
	if cfg.MaxMessageSize == 0 {
		cfg.MaxMessageSize = defaultMaxMessageSize
	}
	if compiler == nil {
		compiler = expr.NewCompiler()
	}
	return &WebSocketHandler{
		config:     cfg,
		manager:    mgr,
		runner:     runner,
		compiler:   compiler,
		logger:     logger,
		paramNames: extractParamNamesFromPath(cfg.Path),
	}
}

// Register sets up the WebSocket route on the Fiber app.
// Middleware handlers (e.g., auth) run before the WebSocket upgrade.
func (h *WebSocketHandler) Register(app *fiber.App, middleware ...fiber.Handler) {
	wsHandler := websocket.New(h.handleConnection, websocket.Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	})

	// Build handler chain: middleware first, then WebSocket handler
	handlers := make([]any, 0, len(middleware)+1)
	for _, mw := range middleware {
		handlers = append(handlers, mw)
	}
	handlers = append(handlers, wsHandler)

	app.Get(h.config.Path, handlers[0], handlers[1:]...)
}

// handleConnection is the Fiber WebSocket handler callback.
func (h *WebSocketHandler) handleConnection(ws *websocket.Conn) {
	defer func() { _ = ws.Close() }()

	connID := uuid.New().String()

	// Extract params from the path
	params := extractFiberParams(ws, h.paramNames)
	userID := fiberLocal[string](ws, "jwt_user_id")

	channel := resolveChannelPattern(h.compiler, h.config.ChannelPattern, params, userID, h.logger)

	// Check max per channel
	if h.config.MaxPerChannel > 0 && h.manager.ChannelCount(channel) >= h.config.MaxPerChannel {
		h.logger.Warn("channel full", "channel", channel, "max", h.config.MaxPerChannel)
		return
	}

	conn := &Conn{
		ID:       connID,
		Channel:  channel,
		Endpoint: h.config.Endpoint,
		UserID:   userID,
		Metadata: map[string]any{
			"params": params,
		},
		SendFn: func(data []byte) error {
			return ws.WriteMessage(websocket.TextMessage, data)
		},
	}

	h.manager.Register(conn)
	defer func() {
		h.manager.Unregister(connID)
		h.fireLifecycle(h.config.OnDisconnect, conn)
	}()

	// Fire on_connect
	h.fireLifecycle(h.config.OnConnect, conn)

	// Set read limits and pong handler
	ws.SetReadLimit(h.config.MaxMessageSize)
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(h.config.PingInterval * 2))
	})

	// Start ping goroutine
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(h.config.PingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()
	defer close(done)

	// Message loop
	for {
		_, msg, err := ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				h.logger.Debug("websocket read error", "conn", connID, "error", err)
			}
			break
		}

		if h.config.OnMessage != "" && h.runner != nil {
			input := h.buildInput(conn)
			input["data"] = parseJSONMessage(msg)
			go func() {
				if err := h.runner(context.Background(), h.config.OnMessage, input); err != nil {
					h.logger.Error("on_message workflow failed", "workflow", h.config.OnMessage, "error", err)
				}
			}()
		}
	}
}

func (h *WebSocketHandler) fireLifecycle(workflowID string, conn *Conn) {
	if workflowID == "" || h.runner == nil {
		return
	}
	input := h.buildInput(conn)
	if err := h.runner(context.Background(), workflowID, input); err != nil {
		h.logger.Error("lifecycle workflow failed", "workflow", workflowID, "error", err)
	}
}

func (h *WebSocketHandler) buildInput(conn *Conn) map[string]any {
	return map[string]any{
		"connection_id": conn.ID,
		"channel":       conn.Channel,
		"endpoint":      conn.Endpoint,
		"user_id":       conn.UserID,
		"params":        conn.Metadata["params"],
	}
}

// extractFiberParams extracts route params from a Fiber WebSocket connection
// using the param names parsed from the route path pattern.
func extractFiberParams(ws *websocket.Conn, paramNames []string) map[string]string {
	params := make(map[string]string)
	if p := ws.Params("*"); p != "" {
		params["*"] = p
	}
	for _, name := range paramNames {
		if v := ws.Params(name); v != "" {
			params[name] = v
		}
	}
	return params
}

// extractParamNamesFromPath parses :paramName segments from a Fiber route path.
func extractParamNamesFromPath(path string) []string {
	var names []string
	for _, part := range strings.Split(path, "/") {
		if strings.HasPrefix(part, ":") {
			names = append(names, strings.TrimPrefix(part, ":"))
		}
	}
	return names
}

// parseJSONMessage attempts to parse a WebSocket message as JSON.
// Returns the parsed map if valid JSON, otherwise returns the raw string.
func parseJSONMessage(msg []byte) any {
	var parsed map[string]any
	if err := json.Unmarshal(msg, &parsed); err == nil {
		return parsed
	}
	return string(msg)
}

// fiberLocal extracts a local value from the WebSocket connection.
func fiberLocal[T any](ws *websocket.Conn, key string) T {
	val, ok := ws.Locals(key).(T)
	if !ok {
		var zero T
		return zero
	}
	return val
}
