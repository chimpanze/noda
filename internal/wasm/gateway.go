package wasm

import (
	"context"
	"fmt"
	"log/slog"
	neturl "net/url"
	"sync"
	"time"

	"github.com/fasthttp/websocket"
)

// Gateway manages outbound WebSocket connections for a Wasm module.
// Concurrency model: readLoop is the sole reader per connection, writes are
// protected by the per-connection mutex (gatewayConn.mu), and CloseConn
// causes readLoop to exit via the underlying connection close.
type Gateway struct {
	mu     sync.RWMutex
	module *Module
	conns  map[string]*gatewayConn
	logger *slog.Logger
}

type gatewayConn struct {
	id     string
	url    string
	ws     *websocket.Conn
	config GatewayConfig

	mu     sync.Mutex
	stopCh chan struct{}
	closed bool
}

// NewGateway creates a new gateway manager.
func NewGateway(module *Module, logger *slog.Logger) *Gateway {
	return &Gateway{
		module: module,
		conns:  make(map[string]*gatewayConn),
		logger: logger,
	}
}

// Connect establishes a new outbound WebSocket connection.
func (g *Gateway) Connect(ctx context.Context, payload map[string]any) (any, error) {
	id, err := requireString(payload, "id")
	if err != nil {
		return nil, err
	}
	wsURL, err := requireString(payload, "url")
	if err != nil {
		return nil, err
	}

	// Check whitelist
	if !g.isAllowed(wsURL) {
		return nil, fmt.Errorf("PERMISSION_DENIED: host not in allow_outbound.ws whitelist")
	}

	headers := make(map[string]string)
	if h, ok := payload["headers"].(map[string]any); ok {
		for k, v := range h {
			headers[k] = fmt.Sprintf("%v", v)
		}
	}

	// Build WebSocket dialer headers
	dialer := websocket.DefaultDialer
	httpHeaders := make(map[string][]string)
	for k, v := range headers {
		httpHeaders[k] = []string{v}
	}

	conn, _, err := dialer.DialContext(ctx, wsURL, httpHeaders)
	if err != nil {
		return nil, fmt.Errorf("SERVICE_UNAVAILABLE: %w", err)
	}

	gc := &gatewayConn{
		id:     id,
		url:    wsURL,
		ws:     conn,
		stopCh: make(chan struct{}),
	}

	g.mu.Lock()
	g.conns[id] = gc
	g.mu.Unlock()

	// Start reading messages
	go g.readLoop(gc)

	g.logger.Debug("gateway connected", "module", g.module.Name, "id", id, "url", wsURL)
	return map[string]any{"status": "connected"}, nil
}

// Send sends a message on an outbound WebSocket connection.
func (g *Gateway) Send(payload map[string]any) (any, error) {
	id, err := requireString(payload, "id")
	if err != nil {
		return nil, err
	}
	data := payload["data"]

	g.mu.RLock()
	gc, ok := g.conns[id]
	g.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("NOT_FOUND: connection %q not found", id)
	}

	msgBytes, err := g.module.Codec.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("INTERNAL_ERROR: marshal message: %w", err)
	}

	gc.mu.Lock()
	defer gc.mu.Unlock()
	if gc.closed {
		return nil, fmt.Errorf("SERVICE_UNAVAILABLE: connection %q is closed", id)
	}

	if err := gc.ws.WriteMessage(websocket.TextMessage, msgBytes); err != nil {
		return nil, fmt.Errorf("INTERNAL_ERROR: write: %w", err)
	}

	return nil, nil
}

// CloseConn closes an outbound WebSocket connection.
func (g *Gateway) CloseConn(payload map[string]any) (any, error) {
	id, err := requireString(payload, "id")
	if err != nil {
		return nil, err
	}
	code := 1000
	reason := optionalString(payload, "reason")
	if v, ok := payload["code"].(float64); ok {
		code = int(v)
	}

	g.mu.Lock()
	gc, ok := g.conns[id]
	if ok {
		delete(g.conns, id)
	}
	g.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("NOT_FOUND: connection %q not found", id)
	}

	gc.mu.Lock()
	gc.closed = true
	close(gc.stopCh)
	gc.mu.Unlock()

	msg := websocket.FormatCloseMessage(code, reason)
	_ = gc.ws.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
	_ = gc.ws.Close()

	g.logger.Debug("gateway disconnected", "module", g.module.Name, "id", id)
	return nil, nil
}

// Configure updates connection settings (heartbeat, reconnection).
func (g *Gateway) Configure(payload map[string]any) (any, error) {
	id, err := requireString(payload, "id")
	if err != nil {
		return nil, err
	}

	g.mu.RLock()
	gc, ok := g.conns[id]
	g.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("NOT_FOUND: connection %q not found", id)
	}

	if v, ok := payload["heartbeat_interval"].(float64); ok && v > 0 {
		gc.config.HeartbeatInterval = time.Duration(v) * time.Millisecond
		gc.config.HeartbeatPayload = payload["heartbeat_payload"]
		go g.heartbeatLoop(gc)
	}

	return nil, nil
}

// CloseAll closes all gateway connections.
func (g *Gateway) CloseAll() {
	g.mu.Lock()
	conns := make(map[string]*gatewayConn, len(g.conns))
	for k, v := range g.conns {
		conns[k] = v
	}
	g.conns = make(map[string]*gatewayConn)
	g.mu.Unlock()

	for _, gc := range conns {
		gc.mu.Lock()
		gc.closed = true
		close(gc.stopCh)
		gc.mu.Unlock()
		_ = gc.ws.Close()
	}
}

// readLoop reads messages from an outbound WebSocket and buffers them for tick delivery.
func (g *Gateway) readLoop(gc *gatewayConn) {
	defer func() {
		gc.mu.Lock()
		wasClosed := gc.closed
		gc.closed = true
		gc.mu.Unlock()

		if !wasClosed {
			g.module.AddConnectionEvent(ConnectionEvent{
				Connection: gc.id,
				Event:      "disconnected",
			})
		}
	}()

	for {
		_, msg, err := gc.ws.ReadMessage()
		if err != nil {
			return
		}

		var data any
		if err := g.module.Codec.Unmarshal(msg, &data); err != nil {
			data = string(msg)
		}

		g.module.AddIncomingWS(IncomingWSMsg{
			Connection: gc.id,
			Data:       data,
		})
	}
}

// heartbeatLoop sends periodic heartbeat messages.
func (g *Gateway) heartbeatLoop(gc *gatewayConn) {
	if gc.config.HeartbeatInterval <= 0 {
		return
	}

	ticker := time.NewTicker(gc.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-gc.stopCh:
			return
		case <-ticker.C:
			gc.mu.Lock()
			if gc.closed {
				gc.mu.Unlock()
				return
			}
			if gc.config.HeartbeatPayload != nil {
				data, _ := g.module.Codec.Marshal(gc.config.HeartbeatPayload)
				_ = gc.ws.WriteMessage(websocket.TextMessage, data)
			}
			gc.mu.Unlock()
		}
	}
}

// isAllowed checks if a URL host is in the module's whitelist.
func (g *Gateway) isAllowed(url string) bool {
	for _, allowed := range g.module.Config.AllowWS {
		if containsHost(url, allowed) {
			return true
		}
	}
	return false
}

// containsHost checks if a URL's hostname matches the allowed host.
// allowedHost may be a bare hostname or a host:port pair.
func containsHost(rawURL, allowedHost string) bool {
	if rawURL == "" || allowedHost == "" {
		return false
	}
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return false
	}
	// Compare against the full Host (includes port if present) and bare Hostname.
	return parsed.Host == allowedHost || parsed.Hostname() == allowedHost
}
