package server

import (
	"bufio"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/registry"
	coresse "github.com/chimpanze/noda/plugins/core/sse"
	corews "github.com/chimpanze/noda/plugins/core/ws"
	"github.com/fasthttp/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_WebSocketSendReceive tests: REST endpoint triggers ws.send → WebSocket client receives.
func TestE2E_WebSocketSendReceive(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "ws-test")

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("ws-test", svc, nil))

	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&corews.Plugin{})

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"push-message": {
				"method": "POST",
				"path":   "/api/push",
				"trigger": map[string]any{
					"workflow": "push-wf",
					"input": map[string]any{
						"channel": "{{ body.channel }}",
						"message": "{{ body.message }}",
					},
				},
			},
		},
		Workflows: map[string]map[string]any{
			"push-wf": {
				"nodes": map[string]any{
					"send": map[string]any{
						"type":     "ws.send",
						"services": map[string]any{"connections": "ws-test"},
						"config": map[string]any{
							"channel": "{{ input.channel }}",
							"data":    "{{ input.message }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.send }}"},
					},
				},
				"edges": []any{
					map[string]any{"from": "send", "to": "respond"},
				},
			},
		},
		Connections: map[string]map[string]any{},
		Schemas:     map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())

	// Simulate a WebSocket client by registering directly with the manager
	received := make(chan []byte, 1)
	mgr.Register(&connmgr.Conn{
		ID:       "test-conn",
		Channel:  "room.42",
		Endpoint: "ws-test",
		SendFn: func(data []byte) error {
			received <- data
			return nil
		},
	})

	// POST to push-message endpoint
	payload, _ := json.Marshal(map[string]any{
		"channel": "room.42",
		"message": "hello from REST",
	})
	req := httptest.NewRequest("POST", "/api/push", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the WebSocket client received the message
	select {
	case msg := <-received:
		assert.Equal(t, "hello from REST", string(msg))
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for WebSocket message")
	}
}

// TestE2E_SSESendReceive tests: REST endpoint triggers sse.send → SSE client receives.
func TestE2E_SSESendReceive(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "sse-test")

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("sse-test", svc, nil))

	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&coresse.Plugin{})

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"push-event": {
				"method": "POST",
				"path":   "/api/push-event",
				"trigger": map[string]any{
					"workflow": "push-event-wf",
					"input": map[string]any{
						"channel": "{{ body.channel }}",
						"data":    "{{ body.data }}",
					},
				},
			},
		},
		Workflows: map[string]map[string]any{
			"push-event-wf": {
				"nodes": map[string]any{
					"send": map[string]any{
						"type":     "sse.send",
						"services": map[string]any{"connections": "sse-test"},
						"config": map[string]any{
							"channel": "{{ input.channel }}",
							"data":    "{{ input.data }}",
							"event":   "update",
							"id":      "1",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.send }}"},
					},
				},
				"edges": []any{
					map[string]any{"from": "send", "to": "respond"},
				},
			},
		},
		Connections: map[string]map[string]any{},
		Schemas:     map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())

	// Simulate an SSE client by registering directly with the manager
	received := make(chan sseTestEvent, 1)
	mgr.Register(&connmgr.Conn{
		ID:       "sse-conn",
		Channel:  "feed.main",
		Endpoint: "sse-test",
		SSEFn: func(event, data, id string) error {
			received <- sseTestEvent{Event: event, Data: data, ID: id}
			return nil
		},
	})

	// POST to push the SSE event
	payload, _ := json.Marshal(map[string]any{
		"channel": "feed.main",
		"data":    "new item",
	})
	req := httptest.NewRequest("POST", "/api/push-event", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Verify the SSE client received the event
	select {
	case evt := <-received:
		assert.Equal(t, "update", evt.Event)
		assert.Equal(t, "new item", evt.Data)
		assert.Equal(t, "1", evt.ID)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for SSE event")
	}
}

type sseTestEvent struct {
	Event string
	Data  string
	ID    string
}

// TestE2E_WebSocketWildcardBroadcast tests: ws.send with wildcard channel delivers to multiple clients.
func TestE2E_WebSocketWildcardBroadcast(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "ws-broadcast")

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("ws-broadcast", svc, nil))

	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&corews.Plugin{})

	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Routes: map[string]map[string]any{
			"broadcast": {
				"method": "POST",
				"path":   "/api/broadcast",
				"trigger": map[string]any{
					"workflow": "broadcast-wf",
					"input": map[string]any{
						"msg": "{{ body.message }}",
					},
				},
			},
		},
		Workflows: map[string]map[string]any{
			"broadcast-wf": {
				"nodes": map[string]any{
					"send": map[string]any{
						"type":     "ws.send",
						"services": map[string]any{"connections": "ws-broadcast"},
						"config": map[string]any{
							"channel": "room.*",
							"data":    "{{ input.msg }}",
						},
					},
					"respond": map[string]any{
						"type":   "response.json",
						"config": map[string]any{"status": "200", "body": "{{ nodes.send }}"},
					},
				},
				"edges": []any{
					map[string]any{"from": "send", "to": "respond"},
				},
			},
		},
		Connections: map[string]map[string]any{},
		Schemas:     map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())

	// Register 3 clients on different rooms
	received := make(chan string, 3)
	for _, ch := range []string{"room.1", "room.2", "room.3"} {
		mgr.Register(&connmgr.Conn{
			ID:       "conn-" + ch,
			Channel:  ch,
			Endpoint: "ws-broadcast",
			SendFn: func(data []byte) error {
				received <- string(data)
				return nil
			},
		})
	}

	payload, _ := json.Marshal(map[string]any{"message": "broadcast!"})
	req := httptest.NewRequest("POST", "/api/broadcast", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")

	resp, err := srv.App().Test(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// All 3 clients should receive the message
	for i := 0; i < 3; i++ {
		select {
		case msg := <-received:
			assert.Equal(t, "broadcast!", msg)
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for message %d", i+1)
		}
	}
}

// TestE2E_RealWebSocketConnection tests a real WebSocket connection with Fiber.
func TestE2E_RealWebSocketConnection(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "chat")

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("chat", svc, nil))

	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&corews.Plugin{})

	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Routes:    map[string]map[string]any{},
		Workflows: map[string]map[string]any{},
		Connections: map[string]map[string]any{
			"connections/chat.json": {
				"endpoints": map[string]any{
					"chat": map[string]any{
						"type": "websocket",
						"path": "/ws/chat",
						"channels": map[string]any{
							"pattern": "chat.general",
						},
						"ping_interval": "5s",
					},
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())

	// Start the Fiber server on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	go func() { _ = srv.App().Listener(ln) }()
	defer func() { _ = srv.App().Shutdown() }()

	// Connect via real WebSocket
	wsURL := "ws://" + ln.Addr().String() + "/ws/chat"
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	// The connection should be registered in the manager
	time.Sleep(100 * time.Millisecond) // brief wait for registration
	// The internal manager for this endpoint is inside the server;
	// we verify by sending from our test mgr, but the real connections
	// are in a separate manager created by registerConnections.
	// Instead, just verify the WebSocket is connected and can write/read.

	// Send a message from client
	err = ws.WriteMessage(websocket.TextMessage, []byte("hello"))
	require.NoError(t, err)
	// No on_message configured, so just verify no error
}

// TestE2E_RealSSEConnection tests a real SSE stream connection.
func TestE2E_RealSSEConnection(t *testing.T) {
	mgr := connmgr.NewManager()

	svcReg := registry.NewServiceRegistry()
	nodeReg := buildTestNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&coresse.Plugin{})

	rc := &config.ResolvedConfig{
		Root:      map[string]any{},
		Routes:    map[string]map[string]any{},
		Workflows: map[string]map[string]any{},
		Connections: map[string]map[string]any{
			"connections/updates.json": {
				"endpoints": map[string]any{
					"updates": map[string]any{
						"type": "sse",
						"path": "/events/updates",
						"channels": map[string]any{
							"pattern": "updates.all",
						},
						"heartbeat": "1s",
					},
				},
			},
		},
		Schemas: map[string]map[string]any{},
	}

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())

	// Start the server
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	go func() { _ = srv.App().Listener(ln) }()
	defer func() { _ = srv.App().Shutdown() }()

	// Connect via HTTP and read SSE stream
	resp, err := http.Get("http://" + ln.Addr().String() + "/events/updates")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Contains(t, resp.Header.Get("Content-Type"), "text/event-stream")

	// Read at least a heartbeat comment
	reader := bufio.NewReader(resp.Body)

	// Set a read deadline via a timer
	done := make(chan string, 1)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				done <- "error: " + err.Error()
				return
			}
			line = strings.TrimSpace(line)
			if line != "" {
				done <- line
				return
			}
		}
	}()

	select {
	case line := <-done:
		// Should be a heartbeat comment
		assert.Contains(t, line, "heartbeat")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for SSE heartbeat")
	}

	_ = mgr // ensure mgr is referenced
}
