package trace

import (
	"log/slog"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
)

// RegisterTraceWebSocket registers the /ws/trace endpoint on the Fiber app.
func RegisterTraceWebSocket(app *fiber.App, hub *EventHub, logger *slog.Logger) {
	app.Get("/ws/trace", websocket.New(func(c *websocket.Conn) {
		logger.Info("trace websocket client connected", "remote", c.RemoteAddr().String())

		// Subscribe to events
		done := make(chan struct{})
		unsubscribe := hub.Subscribe(func(data []byte) {
			select {
			case <-done:
				return
			default:
			}
			if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
				// Connection closed, will be cleaned up by read loop
				return
			}
		})
		defer unsubscribe()

		// Read loop — keeps connection alive, detects close
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				break
			}
		}
		close(done)
		logger.Info("trace websocket client disconnected", "remote", c.RemoteAddr().String())
	}))
}
