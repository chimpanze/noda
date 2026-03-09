package connmgr

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
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

// WorkflowRunner executes a workflow given its ID and input data.
type WorkflowRunner func(ctx context.Context, workflowID string, input map[string]any) error

// WebSocketHandler manages a single WebSocket endpoint.
type WebSocketHandler struct {
	config  WebSocketConfig
	manager *Manager
	runner  WorkflowRunner
	logger  *slog.Logger
}

// NewWebSocketHandler creates a handler for a WebSocket endpoint.
func NewWebSocketHandler(cfg WebSocketConfig, mgr *Manager, runner WorkflowRunner, logger *slog.Logger) *WebSocketHandler {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 30 * time.Second
	}
	if cfg.MaxMessageSize == 0 {
		cfg.MaxMessageSize = 64 * 1024 // 64KB default
	}
	return &WebSocketHandler{
		config:  cfg,
		manager: mgr,
		runner:  runner,
		logger:  logger,
	}
}

// Register sets up the WebSocket route on the Fiber app.
func (h *WebSocketHandler) Register(app *fiber.App) {
	app.Get(h.config.Path, websocket.New(h.handleConnection, websocket.Config{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}))
}

// handleConnection is the Fiber WebSocket handler callback.
func (h *WebSocketHandler) handleConnection(ws *websocket.Conn) {
	defer ws.Close()

	connID := uuid.New().String()

	// Extract params from the path
	params := extractFiberParams(ws)
	userID := fiberLocal[string](ws, "jwt_user_id")

	channel := resolveChannelPattern(h.config.ChannelPattern, params, userID)

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
			input["message"] = string(msg)
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

// resolveChannelPattern replaces placeholders in channel pattern.
func resolveChannelPattern(pattern string, params map[string]string, userID string) string {
	result := pattern
	result = strings.ReplaceAll(result, "{{ auth.sub }}", userID)
	result = strings.ReplaceAll(result, "{{auth.sub}}", userID)
	for k, v := range params {
		result = strings.ReplaceAll(result, "{"+k+"}", v)
		result = strings.ReplaceAll(result, "{{ request.params."+k+" }}", v)
		result = strings.ReplaceAll(result, "{{request.params."+k+"}}", v)
		result = strings.ReplaceAll(result, ":"+k, v)
	}
	return result
}

// extractFiberParams extracts route params from a Fiber WebSocket connection.
func extractFiberParams(ws *websocket.Conn) map[string]string {
	params := make(map[string]string)
	if p := ws.Params("*"); p != "" {
		params["*"] = p
	}
	// Common param names — Fiber WebSocket exposes Params() method
	for _, name := range []string{"id", "channel", "room", "doc_id", "user_id"} {
		if v := ws.Params(name); v != "" {
			params[name] = v
		}
	}
	return params
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
