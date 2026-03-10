package trace

import (
	"log/slog"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
)

// RegisterTraceWebSocket registers the /ws/trace endpoint on the Fiber app.
// It subscribes each connected client to the EventHub and streams execution
// events as JSON text messages. Used in dev mode only.
func RegisterTraceWebSocket(app *fiber.App, hub *EventHub, logger *slog.Logger) {
	app.Get("/ws/trace", websocket.New(func(c *websocket.Conn) {
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
				// Drop message if buffer full (slow client)
			}
		})
		defer unsubscribe()

		// Write loop — single goroutine owns all writes to the connection.
		go func() {
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
		logger.Info("trace websocket client disconnected", "remote", c.RemoteAddr().String())
	}))
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
