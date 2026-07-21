package connmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/chimpanze/noda/internal/bounded"
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
	Endpoint              string
	Path                  string
	ChannelPattern        string // e.g., "doc.{{ request.params.doc_id }}"
	PingInterval          time.Duration
	MaxMessageSize        int64
	MaxPerChannel         int
	MaxConcurrentMessages int // max concurrent onMessage goroutines (default 100)

	OnConnect    string // workflow ID
	OnMessage    string // workflow ID
	OnDisconnect string // workflow ID
}

const (
	defaultMaxConcurrentMessages = 100
	lifecycleTimeout             = 30 * time.Second
)

const (
	// wsOutboundBuffer bounds per-connection queued outbound frames before
	// drop-newest kicks in (mirrors the SSE design).
	wsOutboundBuffer = 64
	// wsWriteTimeout bounds a single socket write so a stuck client cannot
	// block the writer goroutine indefinitely.
	wsWriteTimeout = 5 * time.Second
)

// wsWriter serializes outbound writes for one WebSocket connection through a
// bounded queue drained by a single writer goroutine. send() is non-blocking
// and drops on overflow, so one slow/non-reading client never blocks delivery
// to the other clients on a channel (the head-of-line-blocking fix).
type wsWriter struct {
	q      *bounded.Queue[[]byte]
	done   chan struct{}
	popCtx context.Context
	cancel context.CancelFunc
	once   sync.Once
}

// newWSWriter starts the writer goroutine. write performs the actual socket
// write (callers serialize control frames via the same mutex); onErr, if set,
// is invoked once when a write fails (used to close the socket and unblock the
// read loop).
func newWSWriter(write func([]byte) error, onErr func(), logger *slog.Logger, connID string) *wsWriter {
	w := &wsWriter{
		q:    bounded.New[[]byte](wsOutboundBuffer, bounded.DropNewest),
		done: make(chan struct{}),
	}
	w.popCtx, w.cancel = context.WithCancel(context.Background()) //nolint:gosec // cancel is stored in w.cancel and invoked by stop()
	go func() {
		for {
			data, ok := w.q.Pop(w.popCtx)
			if !ok {
				return
			}
			if err := write(data); err != nil {
				if logger != nil {
					logger.Debug("websocket write failed; closing connection", "conn", connID, "error", err)
				}
				w.stop()
				if onErr != nil {
					onErr()
				}
				return
			}
		}
	}()
	return w
}

// send enqueues data for delivery. It never blocks: on a full buffer the newest
// frame is dropped and an error is returned.
func (w *wsWriter) send(data []byte) error {
	select {
	case <-w.done:
		return fmt.Errorf("connection closed")
	default:
	}
	if !w.q.Push(data) {
		select {
		case <-w.done:
			return fmt.Errorf("connection closed")
		default:
			return fmt.Errorf("ws outbound buffer full")
		}
	}
	return nil
}

// stop halts the writer goroutine and rejects further sends. Safe to call more
// than once.
func (w *wsWriter) stop() {
	w.once.Do(func() {
		close(w.done)
		w.q.Close()
		w.cancel()
	})
}

// WebSocketHandler manages a single WebSocket endpoint.
type WebSocketHandler struct {
	config     WebSocketConfig
	manager    *Manager
	runner     api.WorkflowRunner
	compiler   *expr.Compiler
	logger     *slog.Logger
	paramNames []string      // route param names extracted from path pattern
	msgSem     chan struct{} // bounds concurrent onMessage goroutines
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
	maxMsg := cfg.MaxConcurrentMessages
	if maxMsg <= 0 {
		maxMsg = defaultMaxConcurrentMessages
	}
	return &WebSocketHandler{
		config:     cfg,
		manager:    mgr,
		runner:     runner,
		compiler:   compiler,
		logger:     logger,
		paramNames: extractParamNamesFromPath(cfg.Path),
		msgSem:     make(chan struct{}, maxMsg),
	}
}

// Register sets up the WebSocket route on the Fiber app.
// Middleware handlers (e.g., auth) run before the WebSocket upgrade.
func (h *WebSocketHandler) Register(app *fiber.App, middleware ...fiber.Handler) {
	if len(middleware) == 0 {
		h.logger.Warn("websocket endpoint registered without auth middleware", "path", h.config.Path)
	}
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
	userID := fiberLocal[string](ws, api.LocalJWTUserID)

	channel := resolveChannelPattern(h.compiler, h.config.ChannelPattern, params, userID, h.logger)

	// Mutex to synchronize all writes to the websocket connection.
	// gorilla/websocket's WriteMessage is not safe for concurrent use.
	var wsMu sync.Mutex

	// Per-connection bounded outbound writer: data frames go through a queue
	// drained by a single goroutine that sets a write deadline before each
	// write, so a slow/non-reading client drops its own frames instead of
	// blocking delivery to the rest of the channel (head-of-line blocking).
	writer := newWSWriter(func(data []byte) error {
		wsMu.Lock()
		defer wsMu.Unlock()
		_ = ws.SetWriteDeadline(time.Now().Add(wsWriteTimeout))
		err := ws.WriteMessage(websocket.TextMessage, data)
		_ = ws.SetWriteDeadline(time.Time{}) // clear for subsequent control frames
		return err
	}, func() { _ = ws.Close() }, h.logger, connID)

	conn := &Conn{
		ID:       connID,
		Channel:  channel,
		Endpoint: h.config.Endpoint,
		UserID:   userID,
		Metadata: map[string]any{
			"params": params,
		},
		SendFn: writer.send,
		CloseFn: func() error {
			wsMu.Lock()
			defer wsMu.Unlock()
			return ws.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown"),
				time.Now().Add(5*time.Second),
			)
		},
	}

	if err := h.manager.Register(conn); err != nil {
		h.logger.Warn("connection rejected", "channel", channel, "error", err)
		writer.stop()
		return
	}
	defer func() {
		writer.stop()
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
	go func() {
		ticker := time.NewTicker(h.config.PingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				wsMu.Lock()
				err := ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
				wsMu.Unlock()
				if err != nil {
					return
				}
			case <-writer.done:
				return
			}
		}
	}()

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
			select {
			case h.msgSem <- struct{}{}:
				go func() {
					defer func() { <-h.msgSem }()
					msgCtx, msgCancel := context.WithTimeout(context.Background(), lifecycleTimeout)
					defer msgCancel()
					if err := h.runner(msgCtx, h.config.OnMessage, input); err != nil {
						h.logger.Error("on_message workflow failed", "workflow", h.config.OnMessage, "error", err)
					}
				}()
			default:
				h.logger.Warn("on_message dropped: concurrency limit reached", "conn", connID)
			}
		}
	}
}

func (h *WebSocketHandler) fireLifecycle(workflowID string, conn *Conn) {
	if workflowID == "" || h.runner == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), lifecycleTimeout)
	defer cancel()
	input := h.buildInput(conn)
	if err := h.runner(ctx, workflowID, input); err != nil {
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

// extractFiberParamsFromCtx extracts route params from a Fiber context
// using the param names parsed from the route path pattern.
// This is the shared helper used by both SSE and WebSocket handlers.
func extractFiberParamsFromCtx(c fiber.Ctx, paramNames []string) map[string]string {
	params := make(map[string]string)
	if p := c.Params("*"); p != "" {
		params["*"] = p
	}
	for _, name := range paramNames {
		if v := c.Params(name); v != "" {
			params[name] = v
		}
	}
	return params
}

// extractParamNamesFromPath parses :paramName segments from a Fiber route path.
func extractParamNamesFromPath(path string) []string {
	var names []string
	for part := range strings.SplitSeq(path, "/") {
		if after, ok := strings.CutPrefix(part, ":"); ok {
			names = append(names, after)
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
