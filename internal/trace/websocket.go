package trace

import (
	"log/slog"
	neturl "net/url"
	"strings"
	"sync"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
)

// RegisterTraceWebSocket registers the /ws/trace endpoint on the Fiber app.
// It subscribes each connected client to the EventHub and streams execution
// events as JSON text messages. Used in dev mode only (dev mode binds
// loopback, but the browser still sends a same-origin Origin header, so we
// still guard against a malicious page on another origin driving a
// cross-site WebSocket upgrade against a developer's local instance).
func RegisterTraceWebSocket(app *fiber.App, hub *EventHub, logger *slog.Logger) {
	app.Get("/ws/trace", traceOriginGuard, websocket.New(func(c *websocket.Conn) {
		logger.Info("trace websocket client connected", "remote", c.RemoteAddr().String())

		// Buffered channel serializes writes — hub subscribers may be
		// called from concurrent goroutines (parallel node execution),
		// but websocket.Conn.WriteMessage is not goroutine-safe.
		writeCh := make(chan []byte, 256)
		done := make(chan struct{})

		unsubscribe := hub.Subscribe(func(data []byte) {
			select {
			case <-done:
			case writeCh <- data:
			default:
				logger.Warn("trace event dropped: buffer full", "remote", c.RemoteAddr().String())
			}
		})
		defer unsubscribe()

		// Write loop — single goroutine owns all writes to the connection.
		var writeWg sync.WaitGroup
		writeWg.Add(1)
		go func() {
			defer writeWg.Done()
			for {
				select {
				case <-done:
					return
				case msg := <-writeCh:
					if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
						return
					}
				}
			}
		}()

		// Read loop — keeps connection alive, detects close.
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				break
			}
		}
		close(done)
		writeWg.Wait() // ensure write goroutine exits before handler returns
		logger.Info("trace websocket client disconnected", "remote", c.RemoteAddr().String())
	}))
}

// traceOriginGuard rejects cross-origin WebSocket upgrades to the dev trace
// stream (which carries workflow inputs, DB rows, and secrets). An empty
// Origin (non-browser clients like the CLI) is allowed; browser origins must
// be same-host or localhost.
func traceOriginGuard(c fiber.Ctx) error {
	origin := c.Get("Origin")
	if origin == "" || originAllowed(origin, c.Hostname()) {
		return c.Next()
	}
	return c.SendStatus(fiber.StatusForbidden)
}

func originAllowed(origin, host string) bool {
	u, err := neturl.Parse(origin)
	if err != nil {
		return false
	}
	oh := u.Hostname()
	// Hostnames are case-insensitive (RFC 4343).
	if strings.EqualFold(oh, host) {
		return true
	}
	return strings.EqualFold(oh, "localhost") || oh == "127.0.0.1" || oh == "::1"
}

// RegisterNoOpTraceWebSocket registers a /ws/trace endpoint that accepts
// WebSocket connections but never sends events. Used in production mode so
// the editor can connect without errors.
func RegisterNoOpTraceWebSocket(app *fiber.App) {
	app.Get("/ws/trace", websocket.New(func(c *websocket.Conn) {
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
}
