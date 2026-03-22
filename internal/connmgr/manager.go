package connmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/chimpanze/noda/internal/metrics"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Conn represents a single client connection (WebSocket or SSE).
type Conn struct {
	ID       string
	Channel  string
	Endpoint string
	UserID   string
	Metadata map[string]any
	SendFn   func(data []byte) error            // for WebSocket
	SSEFn    func(event, data, id string) error // for SSE
	CloseFn  func() error                       // called during graceful shutdown
}

// ErrMaxConnectionsReached is returned when the total connection limit is exceeded.
var ErrMaxConnectionsReached = fmt.Errorf("maximum total connections reached")

// ErrMaxChannelConnectionsReached is returned when the per-channel connection limit is exceeded.
var ErrMaxChannelConnectionsReached = fmt.Errorf("maximum connections per channel reached")

// ManagerConfig configures connection limits for a Manager.
type ManagerConfig struct {
	MaxTotalConnections      int // 0 = unlimited
	MaxConnectionsPerChannel int // 0 = unlimited
}

// Manager tracks open connections and provides channel-based delivery.
type Manager struct {
	mu          sync.RWMutex
	connections map[string]*Conn           // connID → Conn
	channels    map[string]map[string]bool // channel → set of connIDs
	connCount   atomic.Int64
	config      ManagerConfig
	metrics     *metrics.Metrics // optional application metrics
	connType    string           // "ws" or "sse", set via SetMetrics
}

// NewManager creates a new connection manager with optional limits.
func NewManager(configs ...ManagerConfig) *Manager {
	var cfg ManagerConfig
	if len(configs) > 0 {
		cfg = configs[0]
	}
	return &Manager{
		connections: make(map[string]*Conn),
		channels:    make(map[string]map[string]bool),
		config:      cfg,
	}
}

// SetMetrics sets the application metrics and connection type label for this manager.
func (m *Manager) SetMetrics(met *metrics.Metrics, connType string) {
	m.metrics = met
	m.connType = connType
}

// Register adds a connection to the manager. Returns an error if connection
// limits would be exceeded.
func (m *Manager) Register(conn *Conn) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check total connection limit
	if m.config.MaxTotalConnections > 0 && len(m.connections) >= m.config.MaxTotalConnections {
		return ErrMaxConnectionsReached
	}

	// Check per-channel limit
	if m.config.MaxConnectionsPerChannel > 0 {
		if channelConns, ok := m.channels[conn.Channel]; ok {
			if len(channelConns) >= m.config.MaxConnectionsPerChannel {
				return ErrMaxChannelConnectionsReached
			}
		}
	}

	m.connections[conn.ID] = conn
	if _, ok := m.channels[conn.Channel]; !ok {
		m.channels[conn.Channel] = make(map[string]bool)
	}
	m.channels[conn.Channel][conn.ID] = true
	m.connCount.Add(1)

	if m.metrics != nil {
		m.metrics.ActiveConns.Add(context.Background(), 1,
			metric.WithAttributes(attribute.String("type", m.connType)),
		)
	}

	return nil
}

// Unregister removes a connection from the manager.
func (m *Manager) Unregister(connID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, ok := m.connections[connID]
	if !ok {
		return
	}

	delete(m.connections, connID)
	if subs, ok := m.channels[conn.Channel]; ok {
		delete(subs, connID)
		if len(subs) == 0 {
			delete(m.channels, conn.Channel)
		}
	}
	m.connCount.Add(-1)

	if m.metrics != nil {
		m.metrics.ActiveConns.Add(context.Background(), -1,
			metric.WithAttributes(attribute.String("type", m.connType)),
		)
	}
}

// Count returns the number of active connections.
func (m *Manager) Count() int64 {
	return m.connCount.Load()
}

// ChannelCount returns the number of connections on a channel.
func (m *Manager) ChannelCount(channel string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.channels[channel])
}

// Send delivers data to all connections matching the channel pattern.
// Supports wildcards: "user.*" matches "user.123", "*" matches all.
func (m *Manager) Send(_ context.Context, channel string, data any) error {
	payload, err := marshalData(data)
	if err != nil {
		return err
	}

	m.mu.RLock()
	conns := m.matchConnections(channel)
	m.mu.RUnlock()

	for _, conn := range conns {
		if conn.SendFn != nil {
			if err := conn.SendFn(payload); err != nil {
				slog.Debug("ws send failed", "channel", channel, "conn", conn.ID, "error", err)
			}
		}
	}
	return nil
}

// SendSSE delivers an SSE event to all connections matching the channel pattern.
func (m *Manager) SendSSE(_ context.Context, channel string, event string, data any, id string) error {
	dataStr, err := marshalDataString(data)
	if err != nil {
		return err
	}

	m.mu.RLock()
	conns := m.matchConnections(channel)
	m.mu.RUnlock()

	for _, conn := range conns {
		if conn.SSEFn != nil {
			if err := conn.SSEFn(event, dataStr, id); err != nil {
				slog.Debug("sse send failed", "channel", channel, "conn", conn.ID, "error", err)
			}
		}
	}
	return nil
}

// matchConnections returns connections matching a channel pattern.
// Must be called with at least a read lock held.
func (m *Manager) matchConnections(pattern string) []*Conn {
	var result []*Conn

	if !strings.Contains(pattern, "*") {
		// Exact match
		if connIDs, ok := m.channels[pattern]; ok {
			for id := range connIDs {
				if conn, ok := m.connections[id]; ok {
					result = append(result, conn)
				}
			}
		}
		return result
	}

	// Wildcard matching
	for channel, connIDs := range m.channels {
		if matchWildcard(pattern, channel) {
			for id := range connIDs {
				if conn, ok := m.connections[id]; ok {
					result = append(result, conn)
				}
			}
		}
	}
	return result
}

// matchWildcard checks if a channel matches a wildcard pattern.
// "user.*" matches "user.123", "user.abc" but not "user.a.b".
// "*" matches everything.
func matchWildcard(pattern, channel string) bool {
	if pattern == "*" {
		return true
	}

	// Split on segments
	patParts := strings.Split(pattern, ".")
	chanParts := strings.Split(channel, ".")

	if len(patParts) != len(chanParts) {
		return false
	}

	for i, pp := range patParts {
		if pp == "*" {
			continue
		}
		if pp != chanParts[i] {
			return false
		}
	}
	return true
}

// Stop gracefully closes and unregisters all connections.
func (m *Manager) Stop(_ context.Context) error {
	m.mu.Lock()
	conns := make([]*Conn, 0, len(m.connections))
	for _, conn := range m.connections {
		conns = append(conns, conn)
	}
	m.mu.Unlock()

	for _, conn := range conns {
		if conn.CloseFn != nil {
			if err := conn.CloseFn(); err != nil {
				slog.Debug("connection close failed", "conn", conn.ID, "error", err)
			}
		}
		m.Unregister(conn.ID)
	}
	return nil
}

// ManagerGroup collects multiple Managers and stops them together.
type ManagerGroup struct {
	mu       sync.Mutex
	managers []*Manager
}

// NewManagerGroup creates a new ManagerGroup.
func NewManagerGroup() *ManagerGroup {
	return &ManagerGroup{}
}

// Add registers a Manager to be stopped on shutdown.
func (g *ManagerGroup) Add(m *Manager) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.managers = append(g.managers, m)
}

// Stop calls Stop on all registered managers.
func (g *ManagerGroup) Stop(ctx context.Context) error {
	g.mu.Lock()
	managers := g.managers
	g.mu.Unlock()

	for _, m := range managers {
		if err := m.Stop(ctx); err != nil {
			return err
		}
	}
	return nil
}

// GetConnection returns a connection by ID.
func (m *Manager) GetConnection(connID string) *Conn {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connections[connID]
}

func marshalData(data any) ([]byte, error) {
	switch v := data.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	default:
		return json.Marshal(data)
	}
}

func marshalDataString(data any) (string, error) {
	switch v := data.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		b, err := json.Marshal(data)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}
