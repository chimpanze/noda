package connmgr

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// SSEConfig holds configuration for an SSE endpoint.
type SSEConfig struct {
	Endpoint       string
	Path           string
	ChannelPattern string
	Heartbeat      time.Duration
	Retry          int // milliseconds

	OnConnect    string // workflow ID
	OnDisconnect string // workflow ID
}

// SSEHandler manages a single SSE endpoint.
type SSEHandler struct {
	config   SSEConfig
	manager  *Manager
	runner   api.WorkflowRunner
	compiler *expr.Compiler
	logger   *slog.Logger
}

// NewSSEHandler creates a handler for an SSE endpoint.
func NewSSEHandler(cfg SSEConfig, mgr *Manager, runner api.WorkflowRunner, compiler *expr.Compiler, logger *slog.Logger) *SSEHandler {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Heartbeat == 0 {
		cfg.Heartbeat = 30 * time.Second
	}
	if compiler == nil {
		compiler = expr.NewCompiler()
	}
	return &SSEHandler{
		config:   cfg,
		manager:  mgr,
		runner:   runner,
		compiler: compiler,
		logger:   logger,
	}
}

// Register sets up the SSE route on the Fiber app.
func (h *SSEHandler) Register(app *fiber.App) {
	app.Get(h.config.Path, h.handleConnection)
}

// handleConnection is the Fiber handler for SSE connections.
func (h *SSEHandler) handleConnection(c fiber.Ctx) error {
	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Set("Transfer-Encoding", "chunked")

	connID := uuid.New().String()

	// Extract params
	params := make(map[string]string)
	for _, p := range c.Route().Params {
		params[p] = c.Params(p)
	}
	userID := ""
	if uid, ok := c.Locals("jwt_user_id").(string); ok {
		userID = uid
	}

	channel := resolveChannelPattern(h.compiler, h.config.ChannelPattern, params, userID)

	// Event channel for pushing SSE events to the client
	const sseEventBuffer = 64
	events := make(chan sseEvent, sseEventBuffer)

	conn := &Conn{
		ID:       connID,
		Channel:  channel,
		Endpoint: h.config.Endpoint,
		UserID:   userID,
		Metadata: map[string]any{
			"params": params,
		},
		SSEFn: func(event, data, id string) error {
			select {
			case events <- sseEvent{Event: event, Data: data, ID: id}:
				return nil
			default:
				return fmt.Errorf("sse buffer full")
			}
		},
	}

	h.manager.Register(conn)

	// Fire on_connect lifecycle
	if h.config.OnConnect != "" && h.runner != nil {
		input := buildSSEInput(conn)
		if err := h.runner(context.Background(), h.config.OnConnect, input); err != nil {
			h.logger.Error("on_connect workflow failed", "workflow", h.config.OnConnect, "error", err)
		}
	}

	return c.SendStreamWriter(func(w *bufio.Writer) {
		defer func() {
			h.manager.Unregister(connID)
			if h.config.OnDisconnect != "" && h.runner != nil {
				input := buildSSEInput(conn)
				if err := h.runner(context.Background(), h.config.OnDisconnect, input); err != nil {
					h.logger.Error("on_disconnect workflow failed", "workflow", h.config.OnDisconnect, "error", err)
				}
			}
		}()

		// Send retry header
		if h.config.Retry > 0 {
			fmt.Fprintf(w, "retry: %d\n\n", h.config.Retry)
			w.Flush()
		}

		ticker := time.NewTicker(h.config.Heartbeat)
		defer ticker.Stop()

		for {
			select {
			case evt, ok := <-events:
				if !ok {
					return
				}
				writeSSEEvent(w, evt)
				if err := w.Flush(); err != nil {
					return
				}

			case <-ticker.C:
				fmt.Fprintf(w, ": heartbeat\n\n")
				if err := w.Flush(); err != nil {
					return
				}
			}
		}
	})
}

type sseEvent struct {
	Event string
	Data  string
	ID    string
}

func writeSSEEvent(w *bufio.Writer, evt sseEvent) {
	if evt.ID != "" {
		fmt.Fprintf(w, "id: %s\n", evt.ID)
	}
	if evt.Event != "" {
		fmt.Fprintf(w, "event: %s\n", evt.Event)
	}
	fmt.Fprintf(w, "data: %s\n\n", evt.Data)
}

func buildSSEInput(conn *Conn) map[string]any {
	return map[string]any{
		"connection_id": conn.ID,
		"channel":       conn.Channel,
		"endpoint":      conn.Endpoint,
		"user_id":       conn.UserID,
		"params":        conn.Metadata["params"],
	}
}
