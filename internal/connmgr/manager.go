package connmgr

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
)

// Conn represents a single client connection (WebSocket or SSE).
type Conn struct {
	ID       string
	Channel  string
	Endpoint string
	UserID   string
	Metadata map[string]any
	SendFn   func(data []byte) error              // for WebSocket
	SSEFn    func(event, data, id string) error    // for SSE
}

// Manager tracks open connections and provides channel-based delivery.
type Manager struct {
	mu          sync.RWMutex
	connections map[string]*Conn            // connID → Conn
	channels    map[string]map[string]bool  // channel → set of connIDs
	connCount   atomic.Int64
}

// NewManager creates a new connection manager.
func NewManager() *Manager {
	return &Manager{
		connections: make(map[string]*Conn),
		channels:    make(map[string]map[string]bool),
	}
}

// Register adds a connection to the manager.
func (m *Manager) Register(conn *Conn) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.connections[conn.ID] = conn
	if _, ok := m.channels[conn.Channel]; !ok {
		m.channels[conn.Channel] = make(map[string]bool)
	}
	m.channels[conn.Channel][conn.ID] = true
	m.connCount.Add(1)
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
			// Best effort, non-blocking
			_ = conn.SendFn(payload)
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
			_ = conn.SSEFn(event, dataStr, id)
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
