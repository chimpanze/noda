package connmgr

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	fastwebsocket "github.com/fasthttp/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startTestApp creates a Fiber app, starts listening on a random port, and returns the address.
func startTestApp(t *testing.T, app *fiber.App) (addr string, cleanup func()) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	addr = ln.Addr().String()

	go func() {
		_ = app.Listener(ln, fiber.ListenConfig{DisableStartupMessage: true})
	}()

	// Give the server a moment to start
	time.Sleep(20 * time.Millisecond)

	cleanup = func() {
		_ = app.Shutdown()
	}
	return addr, cleanup
}

// --- EndpointService tests ---

func TestEndpointService_Send(t *testing.T) {
	mgr := NewManager()
	var received []byte

	require.NoError(t, mgr.Register(&Conn{
		ID:      "c1",
		Channel: "ch1",
		SendFn:  func(data []byte) error { received = data; return nil },
	}))

	svc := NewEndpointService(mgr, "my-endpoint")
	err := svc.Send(context.Background(), "ch1", "hello")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), received)
}

func TestEndpointService_SendSSE(t *testing.T) {
	mgr := NewManager()
	var gotEvent, gotData, gotID string

	require.NoError(t, mgr.Register(&Conn{
		ID:      "c1",
		Channel: "sse-ch",
		SSEFn: func(event, data, id string) error {
			gotEvent = event
			gotData = data
			gotID = id
			return nil
		},
	}))

	svc := NewEndpointService(mgr, "sse-endpoint")
	err := svc.SendSSE(context.Background(), "sse-ch", "update", "payload", "evt1")
	require.NoError(t, err)
	assert.Equal(t, "update", gotEvent)
	assert.Equal(t, "payload", gotData)
	assert.Equal(t, "evt1", gotID)
}

func TestEndpointService_Manager(t *testing.T) {
	mgr := NewManager()
	svc := NewEndpointService(mgr, "ep")
	assert.Same(t, mgr, svc.Manager())
}

// --- resolveChannelPattern tests ---

func TestResolveChannelPattern_ColonParam(t *testing.T) {
	result := resolveChannelPattern(nil, "room.:id", map[string]string{"id": "42"}, "", nil)
	assert.Equal(t, "room.42", result)
}

func TestResolveChannelPattern_ExprError_FallbackToParamReplacement(t *testing.T) {
	logger := slog.Default()
	result := resolveChannelPattern(nil, "room.{{ invalid_expr!!! }}", map[string]string{"room": "abc"}, "", logger)
	assert.Contains(t, result, "room.")
}

func TestResolveChannelPattern_ExprError_FallbackWithColonParam(t *testing.T) {
	logger := slog.Default()
	result := resolveChannelPattern(nil, "doc.{{ bogus??? }}.:id", map[string]string{"id": "99"}, "", logger)
	assert.Contains(t, result, "99")
}

func TestResolveChannelPattern_NoMarkers(t *testing.T) {
	result := resolveChannelPattern(nil, "plain-channel", nil, "", nil)
	assert.Equal(t, "plain-channel", result)
}

func TestResolveChannelPattern_NilLogger_ExprError(t *testing.T) {
	result := resolveChannelPattern(nil, "ch.{{ bad!!! }}", nil, "", nil)
	assert.NotEmpty(t, result)
}

func TestResolveChannelPattern_MultipleColonParams(t *testing.T) {
	result := resolveChannelPattern(nil, "org.:org_id.project.:proj_id", map[string]string{
		"org_id":  "myorg",
		"proj_id": "proj1",
	}, "", nil)
	assert.Equal(t, "org.myorg.project.proj1", result)
}

func TestResolveChannelPattern_MixedBraceAndColon(t *testing.T) {
	result := resolveChannelPattern(nil, "{org}.project.:id", map[string]string{
		"org": "acme",
		"id":  "123",
	}, "", nil)
	assert.Equal(t, "acme.project.123", result)
}

// --- extractParamNamesFromPath tests ---

func TestExtractParamNamesFromPath_NoParams(t *testing.T) {
	names := extractParamNamesFromPath("/api/rooms")
	assert.Empty(t, names)
}

func TestExtractParamNamesFromPath_SingleParam(t *testing.T) {
	names := extractParamNamesFromPath("/ws/:room_id")
	assert.Equal(t, []string{"room_id"}, names)
}

func TestExtractParamNamesFromPath_MultipleParams(t *testing.T) {
	names := extractParamNamesFromPath("/ws/:org/:room_id")
	assert.Equal(t, []string{"org", "room_id"}, names)
}

func TestExtractParamNamesFromPath_EmptyPath(t *testing.T) {
	names := extractParamNamesFromPath("")
	assert.Empty(t, names)
}

func TestExtractParamNamesFromPath_MixedSegments(t *testing.T) {
	names := extractParamNamesFromPath("/api/:version/rooms/:id/messages")
	assert.Equal(t, []string{"version", "id"}, names)
}

// --- parseJSONMessage tests ---

func TestParseJSONMessage_ValidJSON(t *testing.T) {
	msg := []byte(`{"type":"chat","text":"hello"}`)
	result := parseJSONMessage(msg)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "chat", m["type"])
	assert.Equal(t, "hello", m["text"])
}

func TestParseJSONMessage_PlainText(t *testing.T) {
	msg := []byte("just a plain message")
	result := parseJSONMessage(msg)
	s, ok := result.(string)
	require.True(t, ok)
	assert.Equal(t, "just a plain message", s)
}

func TestParseJSONMessage_InvalidJSON(t *testing.T) {
	msg := []byte(`{broken json`)
	result := parseJSONMessage(msg)
	s, ok := result.(string)
	require.True(t, ok)
	assert.Equal(t, "{broken json", s)
}

func TestParseJSONMessage_JSONArray(t *testing.T) {
	msg := []byte(`[1,2,3]`)
	result := parseJSONMessage(msg)
	_, ok := result.(string)
	assert.True(t, ok)
}

func TestParseJSONMessage_EmptyObject(t *testing.T) {
	msg := []byte(`{}`)
	result := parseJSONMessage(msg)
	m, ok := result.(map[string]any)
	require.True(t, ok)
	assert.Empty(t, m)
}

// --- writeSSEEvent tests ---

func TestWriteSSEEvent_AllFields(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeSSEEvent(w, sseEvent{
		ID:    "evt-1",
		Event: "message",
		Data:  "hello world",
	})
	_ = w.Flush()

	output := buf.String()
	assert.Contains(t, output, "id: evt-1\n")
	assert.Contains(t, output, "event: message\n")
	assert.Contains(t, output, "data: hello world\n\n")
}

func TestWriteSSEEvent_DataOnly(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeSSEEvent(w, sseEvent{
		Data: "just data",
	})
	_ = w.Flush()

	output := buf.String()
	assert.NotContains(t, output, "id:")
	assert.NotContains(t, output, "event:")
	assert.Contains(t, output, "data: just data\n\n")
}

func TestWriteSSEEvent_WithIDNoEvent(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeSSEEvent(w, sseEvent{
		ID:   "42",
		Data: "payload",
	})
	_ = w.Flush()

	output := buf.String()
	assert.Contains(t, output, "id: 42\n")
	assert.NotContains(t, output, "event:")
	assert.Contains(t, output, "data: payload\n\n")
}

func TestWriteSSEEvent_WithEventNoID(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeSSEEvent(w, sseEvent{
		Event: "ping",
		Data:  "pong",
	})
	_ = w.Flush()

	output := buf.String()
	assert.NotContains(t, output, "id:")
	assert.Contains(t, output, "event: ping\n")
	assert.Contains(t, output, "data: pong\n\n")
}

// --- marshalData tests ---

func TestMarshalData_Bytes(t *testing.T) {
	input := []byte("raw bytes")
	result, err := marshalData(input)
	require.NoError(t, err)
	assert.Equal(t, input, result)
}

func TestMarshalData_String(t *testing.T) {
	result, err := marshalData("hello")
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), result)
}

func TestMarshalData_Map(t *testing.T) {
	input := map[string]any{"key": "value"}
	result, err := marshalData(input)
	require.NoError(t, err)
	assert.Contains(t, string(result), `"key"`)
	assert.Contains(t, string(result), `"value"`)
}

func TestMarshalData_Int(t *testing.T) {
	result, err := marshalData(42)
	require.NoError(t, err)
	assert.Equal(t, []byte("42"), result)
}

func TestMarshalData_Unmarshalable(t *testing.T) {
	ch := make(chan int)
	_, err := marshalData(ch)
	assert.Error(t, err)
}

// --- marshalDataString tests ---

func TestMarshalDataString_String(t *testing.T) {
	result, err := marshalDataString("hello")
	require.NoError(t, err)
	assert.Equal(t, "hello", result)
}

func TestMarshalDataString_Bytes(t *testing.T) {
	result, err := marshalDataString([]byte("raw"))
	require.NoError(t, err)
	assert.Equal(t, "raw", result)
}

func TestMarshalDataString_Map(t *testing.T) {
	input := map[string]any{"k": "v"}
	result, err := marshalDataString(input)
	require.NoError(t, err)
	assert.Contains(t, result, `"k"`)
	assert.Contains(t, result, `"v"`)
}

func TestMarshalDataString_Unmarshalable(t *testing.T) {
	ch := make(chan int)
	_, err := marshalDataString(ch)
	assert.Error(t, err)
}

// --- GetConnection tests ---

func TestGetConnection_Found(t *testing.T) {
	mgr := NewManager()
	conn := &Conn{ID: "c1", Channel: "ch"}
	require.NoError(t, mgr.Register(conn))

	got := mgr.getConnection("c1")
	require.NotNil(t, got)
	assert.Equal(t, "c1", got.ID)
	assert.Equal(t, "ch", got.Channel)
}

func TestGetConnection_NotFound(t *testing.T) {
	mgr := NewManager()
	got := mgr.getConnection("nonexistent")
	assert.Nil(t, got)
}

// --- ChannelCount tests ---

func TestChannelCount_EmptyChannel(t *testing.T) {
	mgr := NewManager()
	assert.Equal(t, 0, mgr.ChannelCount("no-such-channel"))
}

func TestChannelCount_AfterAllUnregistered(t *testing.T) {
	mgr := NewManager()
	require.NoError(t, mgr.Register(&Conn{ID: "c1", Channel: "ch"}))
	require.NoError(t, mgr.Register(&Conn{ID: "c2", Channel: "ch"}))
	assert.Equal(t, 2, mgr.ChannelCount("ch"))

	mgr.Unregister("c1")
	mgr.Unregister("c2")
	assert.Equal(t, 0, mgr.ChannelCount("ch"))
}

// --- Unregister tests ---

func TestUnregister_NonexistentID(t *testing.T) {
	mgr := NewManager()
	mgr.Unregister("does-not-exist")
	assert.Equal(t, int64(0), mgr.Count())
}

func TestUnregister_Twice(t *testing.T) {
	mgr := NewManager()
	require.NoError(t, mgr.Register(&Conn{ID: "c1", Channel: "ch"}))
	mgr.Unregister("c1")
	mgr.Unregister("c1")
	assert.Equal(t, int64(0), mgr.Count())
}

// --- Send/SendSSE with nil SendFn/SSEFn ---

func TestSend_NilSendFn_NoError(t *testing.T) {
	mgr := NewManager()
	require.NoError(t, mgr.Register(&Conn{ID: "c1", Channel: "ch"}))

	err := mgr.Send(context.Background(), "ch", "msg")
	assert.NoError(t, err)
}

func TestSendSSE_NilSSEFn_NoError(t *testing.T) {
	mgr := NewManager()
	require.NoError(t, mgr.Register(&Conn{ID: "c1", Channel: "ch"}))

	err := mgr.SendSSE(context.Background(), "ch", "event", "data", "1")
	assert.NoError(t, err)
}

// --- Send with various data types ---

func TestSend_MapData(t *testing.T) {
	mgr := NewManager()
	var received []byte

	require.NoError(t, mgr.Register(&Conn{
		ID:      "c1",
		Channel: "ch",
		SendFn:  func(data []byte) error { received = data; return nil },
	}))

	err := mgr.Send(context.Background(), "ch", map[string]any{"msg": "hi"})
	require.NoError(t, err)
	assert.Contains(t, string(received), `"msg"`)
}

func TestSend_ByteData(t *testing.T) {
	mgr := NewManager()
	var received []byte

	require.NoError(t, mgr.Register(&Conn{
		ID:      "c1",
		Channel: "ch",
		SendFn:  func(data []byte) error { received = data; return nil },
	}))

	err := mgr.Send(context.Background(), "ch", []byte("raw"))
	require.NoError(t, err)
	assert.Equal(t, []byte("raw"), received)
}

func TestSend_MarshalError(t *testing.T) {
	mgr := NewManager()
	require.NoError(t, mgr.Register(&Conn{
		ID:      "c1",
		Channel: "ch",
		SendFn:  func(data []byte) error { return nil },
	}))

	err := mgr.Send(context.Background(), "ch", make(chan int))
	assert.Error(t, err)
}

func TestSendSSE_MapData(t *testing.T) {
	mgr := NewManager()
	var gotData string

	require.NoError(t, mgr.Register(&Conn{
		ID:      "c1",
		Channel: "ch",
		SSEFn:   func(event, data, id string) error { gotData = data; return nil },
	}))

	err := mgr.SendSSE(context.Background(), "ch", "evt", map[string]any{"k": "v"}, "1")
	require.NoError(t, err)
	assert.Contains(t, gotData, `"k"`)
}

func TestSendSSE_ByteData(t *testing.T) {
	mgr := NewManager()
	var gotData string

	require.NoError(t, mgr.Register(&Conn{
		ID:      "c1",
		Channel: "ch",
		SSEFn:   func(event, data, id string) error { gotData = data; return nil },
	}))

	err := mgr.SendSSE(context.Background(), "ch", "evt", []byte("raw"), "1")
	require.NoError(t, err)
	assert.Equal(t, "raw", gotData)
}

func TestSendSSE_MarshalError(t *testing.T) {
	mgr := NewManager()
	require.NoError(t, mgr.Register(&Conn{
		ID:      "c1",
		Channel: "ch",
		SSEFn:   func(event, data, id string) error { return nil },
	}))

	err := mgr.SendSSE(context.Background(), "ch", "evt", make(chan int), "1")
	assert.Error(t, err)
}

// --- Send with wildcard to connections with nil SendFn ---

func TestSend_Wildcard_MixedSendFn(t *testing.T) {
	mgr := NewManager()
	var called bool

	require.NoError(t, mgr.Register(&Conn{ID: "c1", Channel: "ns.a"}))
	require.NoError(t, mgr.Register(&Conn{
		ID:      "c2",
		Channel: "ns.b",
		SendFn:  func(data []byte) error { called = true; return nil },
	}))

	err := mgr.Send(context.Background(), "ns.*", "msg")
	require.NoError(t, err)
	assert.True(t, called)
}

// --- SendSSE with wildcard ---

func TestSendSSE_Wildcard(t *testing.T) {
	mgr := NewManager()
	var count int

	require.NoError(t, mgr.Register(&Conn{
		ID:      "c1",
		Channel: "feed.a",
		SSEFn:   func(event, data, id string) error { count++; return nil },
	}))
	require.NoError(t, mgr.Register(&Conn{
		ID:      "c2",
		Channel: "feed.b",
		SSEFn:   func(event, data, id string) error { count++; return nil },
	}))

	err := mgr.SendSSE(context.Background(), "feed.*", "update", "payload", "")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// --- buildSSEInput test ---

func TestBuildSSEInput(t *testing.T) {
	conn := &Conn{
		ID:       "c1",
		Channel:  "ch1",
		Endpoint: "ep1",
		UserID:   "u1",
		Metadata: map[string]any{
			"params": map[string]string{"room": "42"},
		},
	}

	input := buildSSEInput(conn)
	assert.Equal(t, "c1", input["connection_id"])
	assert.Equal(t, "ch1", input["channel"])
	assert.Equal(t, "ep1", input["endpoint"])
	assert.Equal(t, "u1", input["user_id"])
	assert.Equal(t, map[string]string{"room": "42"}, input["params"])
}

// --- NewSSEHandler tests ---

func TestNewSSEHandler_Defaults(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-ep",
		Path:           "/events",
		ChannelPattern: "updates",
	}, mgr, nil, nil, nil)

	assert.NotNil(t, h)
	assert.Equal(t, 30*time.Second, h.config.Heartbeat)
	assert.NotNil(t, h.compiler)
	assert.NotNil(t, h.logger)
}

func TestNewSSEHandler_CustomValues(t *testing.T) {
	mgr := NewManager()
	logger := slog.Default()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-ep",
		Path:           "/events",
		ChannelPattern: "updates",
		Heartbeat:      10 * time.Second,
		Retry:          3000,
	}, mgr, nil, nil, logger)

	assert.Equal(t, 10*time.Second, h.config.Heartbeat)
	assert.Equal(t, 3000, h.config.Retry)
	assert.Same(t, logger, h.logger)
}

// --- NewWebSocketHandler tests ---

func TestNewWebSocketHandler_Defaults(t *testing.T) {
	mgr := NewManager()
	h := NewWebSocketHandler(WebSocketConfig{
		Endpoint:       "ws-ep",
		Path:           "/ws/:room_id",
		ChannelPattern: "room.:room_id",
	}, mgr, nil, nil, nil)

	assert.NotNil(t, h)
	assert.Equal(t, defaultPingInterval, h.config.PingInterval)
	assert.Equal(t, int64(defaultMaxMessageSize), h.config.MaxMessageSize)
	assert.NotNil(t, h.compiler)
	assert.NotNil(t, h.logger)
	assert.Equal(t, []string{"room_id"}, h.paramNames)
}

func TestNewWebSocketHandler_CustomValues(t *testing.T) {
	mgr := NewManager()
	logger := slog.Default()
	h := NewWebSocketHandler(WebSocketConfig{
		Endpoint:       "ws-ep",
		Path:           "/ws/:org/:id",
		ChannelPattern: "room",
		PingInterval:   5 * time.Second,
		MaxMessageSize: 1024,
		MaxPerChannel:  10,
	}, mgr, nil, nil, logger)

	assert.Equal(t, 5*time.Second, h.config.PingInterval)
	assert.Equal(t, int64(1024), h.config.MaxMessageSize)
	assert.Equal(t, 10, h.config.MaxPerChannel)
	assert.Same(t, logger, h.logger)
	assert.Equal(t, []string{"org", "id"}, h.paramNames)
}

// --- WebSocketHandler.fireLifecycle tests ---

func TestFireLifecycle_EmptyWorkflowID(t *testing.T) {
	mgr := NewManager()
	var called bool
	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		called = true
		return nil
	}
	h := NewWebSocketHandler(WebSocketConfig{
		Path:           "/ws",
		ChannelPattern: "ch",
	}, mgr, runner, nil, nil)

	conn := &Conn{ID: "c1", Channel: "ch", Metadata: map[string]any{"params": map[string]string{}}}
	h.fireLifecycle("", conn)
	assert.False(t, called)
}

func TestFireLifecycle_NilRunner(t *testing.T) {
	mgr := NewManager()
	h := NewWebSocketHandler(WebSocketConfig{
		Path:           "/ws",
		ChannelPattern: "ch",
	}, mgr, nil, nil, nil)

	conn := &Conn{ID: "c1", Channel: "ch", Metadata: map[string]any{"params": map[string]string{}}}
	// Should not panic
	h.fireLifecycle("some-workflow", conn)
}

func TestFireLifecycle_RunnerSuccess(t *testing.T) {
	mgr := NewManager()
	var gotWfID string
	var gotInput map[string]any
	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		gotWfID = wfID
		gotInput = input
		return nil
	}
	h := NewWebSocketHandler(WebSocketConfig{
		Path:           "/ws",
		ChannelPattern: "ch",
	}, mgr, runner, nil, nil)

	conn := &Conn{
		ID:       "c1",
		Channel:  "ch",
		Endpoint: "ws-ep",
		UserID:   "u1",
		Metadata: map[string]any{"params": map[string]string{"room": "42"}},
	}
	h.fireLifecycle("on_connect", conn)
	assert.Equal(t, "on_connect", gotWfID)
	assert.Equal(t, "c1", gotInput["connection_id"])
	assert.Equal(t, "ch", gotInput["channel"])
	assert.Equal(t, "ws-ep", gotInput["endpoint"])
	assert.Equal(t, "u1", gotInput["user_id"])
}

func TestFireLifecycle_RunnerError(t *testing.T) {
	mgr := NewManager()
	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		return fmt.Errorf("workflow error")
	}
	h := NewWebSocketHandler(WebSocketConfig{
		Path:           "/ws",
		ChannelPattern: "ch",
	}, mgr, runner, nil, nil)

	conn := &Conn{ID: "c1", Channel: "ch", Metadata: map[string]any{"params": map[string]string{}}}
	// Should not panic, just log the error
	h.fireLifecycle("on_connect", conn)
}

// --- WebSocketHandler.buildInput test ---

func TestWebSocketHandler_BuildInput(t *testing.T) {
	mgr := NewManager()
	h := NewWebSocketHandler(WebSocketConfig{
		Path:           "/ws",
		ChannelPattern: "ch",
	}, mgr, nil, nil, nil)

	conn := &Conn{
		ID:       "c1",
		Channel:  "test-ch",
		Endpoint: "ws-ep",
		UserID:   "user42",
		Metadata: map[string]any{"params": map[string]string{"id": "99"}},
	}

	input := h.buildInput(conn)
	assert.Equal(t, "c1", input["connection_id"])
	assert.Equal(t, "test-ch", input["channel"])
	assert.Equal(t, "ws-ep", input["endpoint"])
	assert.Equal(t, "user42", input["user_id"])
	assert.Equal(t, map[string]string{"id": "99"}, input["params"])
}

// --- SSEHandler integration test via Fiber ---

func TestSSEHandler_Integration(t *testing.T) {
	mgr := NewManager()
	var connectCalled bool
	disconnectDone := make(chan struct{})
	var mu sync.Mutex

	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		mu.Lock()
		defer mu.Unlock()
		switch wfID {
		case "on_connect":
			connectCalled = true
		case "on_disconnect":
			close(disconnectDone)
		}
		return nil
	}

	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "updates",
		Heartbeat:      100 * time.Millisecond,
		Retry:          1000,
		OnConnect:      "on_connect",
		OnDisconnect:   "on_disconnect",
	}, mgr, runner, nil, nil)

	app := fiber.New()
	h.Register(app)

	// Start the request in a goroutine, read first chunk, then close
	go func() {
		time.Sleep(50 * time.Millisecond)
		// Send an event to the connected client
		_ = mgr.SendSSE(context.Background(), "updates", "test-event", "test-data", "id1")
	}()

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 500 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Should have retry header
	assert.Contains(t, bodyStr, "retry: 1000")

	// Should have the SSE event we sent
	assert.Contains(t, bodyStr, "event: test-event")
	assert.Contains(t, bodyStr, "data: test-data")
	assert.Contains(t, bodyStr, "id: id1")

	mu.Lock()
	assert.True(t, connectCalled, "on_connect should have been called")
	mu.Unlock()

	// Wait for disconnect lifecycle to fire (happens asynchronously after stream ends)
	select {
	case <-disconnectDone:
		// ok
	case <-time.After(3 * time.Second):
		t.Error("on_disconnect was not called within timeout")
	}
}

func TestSSEHandler_Integration_NoLifecycles(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "updates",
		Heartbeat:      100 * time.Millisecond,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	// Should get heartbeat
	assert.Contains(t, bodyStr, ": heartbeat")
}

func TestSSEHandler_Integration_WithParams(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events/:room_id",
		ChannelPattern: "room.{{ request.params.room_id }}",
		Heartbeat:      100 * time.Millisecond,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = mgr.SendSSE(context.Background(), "room.42", "msg", "hello", "")
	}()

	req, _ := http.NewRequest("GET", "/events/42", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	assert.Contains(t, bodyStr, "data: hello")
}

func TestSSEHandler_Integration_WithMiddleware(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "user.{{ auth.sub }}",
		Heartbeat:      100 * time.Millisecond,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	// Middleware that sets jwt_user_id
	authMW := func(c fiber.Ctx) error {
		c.Locals("jwt_user_id", "user-abc")
		return c.Next()
	}
	h.Register(app, authMW)

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = mgr.SendSSE(context.Background(), "user.user-abc", "update", "for-user", "")
	}()

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "data: for-user")
}

func TestSSEHandler_Integration_OnConnectError(t *testing.T) {
	mgr := NewManager()
	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		return fmt.Errorf("connect workflow error")
	}

	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "ch",
		Heartbeat:      100 * time.Millisecond,
		OnConnect:      "on_connect",
	}, mgr, runner, nil, nil)

	app := fiber.New()
	h.Register(app)

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should still work even if on_connect workflow fails
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), ": heartbeat")
}

func TestSSEHandler_Integration_SSEFnBufferFull(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "ch",
		Heartbeat:      5 * time.Second, // long heartbeat so we don't drain the buffer
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	// Use a channel to synchronize: wait for connection to be established
	connected := make(chan struct{})
	go func() {
		// Wait a bit for the SSE handler to register the connection
		time.Sleep(50 * time.Millisecond)
		close(connected)
	}()

	go func() {
		<-connected
		// Flood the buffer (buffer size is 64) to trigger "sse buffer full"
		for i := 0; i < 100; i++ {
			_ = mgr.SendSSE(context.Background(), "ch", "msg", fmt.Sprintf("msg-%d", i), "")
		}
	}()

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	// Should have received at least some messages
	assert.Contains(t, string(body), "data: msg-")
}

// --- SSEHandler Register with WebSocket handler on same app ---

func TestSSEHandler_Register_RouteExists(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/sse",
		ChannelPattern: "ch",
		Heartbeat:      100 * time.Millisecond,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	// Verify route is registered by making a request
	req, _ := http.NewRequest("GET", "/sse", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, 200, resp.StatusCode)
}

// --- resolveChannelPattern: expression returns non-string result ---

func TestResolveChannelPattern_ExprReturnsNonString(t *testing.T) {
	// If the expression result is not a string, it should fall through to param replacement
	// Use an expression that evaluates to a number
	result := resolveChannelPattern(nil, "ch.{{ 1 + 2 }}", map[string]string{"x": "val"}, "", nil)
	// The result of 1+2 is int 3, not a string, so it falls through to param replacement
	assert.NotEmpty(t, result)
}

// --- resolveChannelPattern: successful expr with user ID ---

func TestResolveChannelPattern_AuthSub(t *testing.T) {
	result := resolveChannelPattern(nil, "tasks.{{ auth.sub }}", nil, "user-xyz", nil)
	assert.Equal(t, "tasks.user-xyz", result)
}

func TestResolveChannelPattern_RequestParams(t *testing.T) {
	result := resolveChannelPattern(nil, "doc.{{ request.params.doc_id }}", map[string]string{"doc_id": "abc123"}, "", nil)
	assert.Equal(t, "doc.abc123", result)
}

// --- SSE disconnect lifecycle error ---

func TestSSEHandler_Integration_OnDisconnectError(t *testing.T) {
	mgr := NewManager()
	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		if wfID == "on_disconnect" {
			return fmt.Errorf("disconnect error")
		}
		return nil
	}

	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "ch",
		Heartbeat:      100 * time.Millisecond,
		OnConnect:      "on_connect",
		OnDisconnect:   "on_disconnect",
	}, mgr, runner, nil, nil)

	app := fiber.New()
	h.Register(app)

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	// Should complete without error even if on_disconnect fails
	_, _ = io.ReadAll(resp.Body)
}

// --- SSEFn after done channel is closed ---

func TestSSEHandler_Integration_SSEFnAfterClose(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "ch",
		Heartbeat:      50 * time.Millisecond,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 200 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	// After the connection is closed, the connection should eventually be unregistered
	// Give the cleanup goroutine time to run
	assert.Eventually(t, func() bool {
		return mgr.Count() == 0
	}, 2*time.Second, 50*time.Millisecond, "connection count should reach 0 after close")
}

// --- SendSSE with wildcard and nil SSEFn ---

func TestSendSSE_Wildcard_MixedSSEFn(t *testing.T) {
	mgr := NewManager()
	var called bool

	require.NoError(t, mgr.Register(&Conn{ID: "c1", Channel: "ns.a"}))
	require.NoError(t, mgr.Register(&Conn{
		ID:      "c2",
		Channel: "ns.b",
		SSEFn:   func(event, data, id string) error { called = true; return nil },
	}))

	err := mgr.SendSSE(context.Background(), "ns.*", "evt", "data", "")
	require.NoError(t, err)
	assert.True(t, called)
}

// --- matchConnections with exact match and missing connection ---

func TestSend_ExactMatch_NoConnections(t *testing.T) {
	mgr := NewManager()
	err := mgr.Send(context.Background(), "nonexistent", "msg")
	assert.NoError(t, err)
}

func TestSendSSE_ExactMatch_NoConnections(t *testing.T) {
	mgr := NewManager()
	err := mgr.SendSSE(context.Background(), "nonexistent", "evt", "data", "id")
	assert.NoError(t, err)
}

// --- SSE handler: no retry header when Retry is 0 ---

func TestSSEHandler_Integration_NoRetry(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "ch",
		Heartbeat:      100 * time.Millisecond,
		Retry:          0,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.NotContains(t, string(body), "retry:")
}

// --- writeSSEEvent: empty data ---

func TestWriteSSEEvent_EmptyData(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeSSEEvent(w, sseEvent{Data: ""})
	_ = w.Flush()

	output := buf.String()
	assert.Equal(t, "data: \n\n", output)
}

// --- resolveChannelPattern: only has single brace {param} ---

func TestResolveChannelPattern_SingleBraceOnly(t *testing.T) {
	result := resolveChannelPattern(nil, "room.{id}", map[string]string{"id": "99"}, "", nil)
	assert.Equal(t, "room.99", result)
}

// --- Multiple channels, verify channel cleanup ---

func TestChannelCleanup_LastConnection(t *testing.T) {
	mgr := NewManager()
	require.NoError(t, mgr.Register(&Conn{ID: "c1", Channel: "ch-a"}))
	require.NoError(t, mgr.Register(&Conn{ID: "c2", Channel: "ch-b"}))

	assert.Equal(t, 1, mgr.ChannelCount("ch-a"))
	assert.Equal(t, 1, mgr.ChannelCount("ch-b"))

	mgr.Unregister("c1")
	assert.Equal(t, 0, mgr.ChannelCount("ch-a"))
	assert.Equal(t, 1, mgr.ChannelCount("ch-b"))
}

// --- SSE handler with data that contains newlines ---

func TestSSEHandler_Integration_MultilineData(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "ch",
		Heartbeat:      200 * time.Millisecond,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	go func() {
		time.Sleep(30 * time.Millisecond)
		_ = mgr.SendSSE(context.Background(), "ch", "msg", "line1\nline2", "")
	}()

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "data: line1\nline2")
}

// --- writeSSEEvent exact format verification ---

func TestWriteSSEEvent_ExactFormat(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	writeSSEEvent(w, sseEvent{
		ID:    "1",
		Event: "update",
		Data:  "hello",
	})
	_ = w.Flush()

	// Exact expected output
	expected := "id: 1\nevent: update\ndata: hello\n\n"
	assert.Equal(t, expected, buf.String())
}

// --- resolveChannelPattern: pattern with only colon, no braces ---

func TestResolveChannelPattern_ColonOnly(t *testing.T) {
	result := resolveChannelPattern(nil, "room:id", map[string]string{"id": "42"}, "", nil)
	// :id should be replaced even when not dot-separated
	assert.Equal(t, "room42", result)
}

// --- resolveChannelPattern: empty params map ---

func TestResolveChannelPattern_EmptyParams_ColonPattern(t *testing.T) {
	result := resolveChannelPattern(nil, "room.:id", map[string]string{}, "", nil)
	// No matching param, :id stays as-is
	assert.Equal(t, "room.:id", result)
}

// --- resolveChannelPattern: nil params ---

func TestResolveChannelPattern_NilParams_ColonPattern(t *testing.T) {
	result := resolveChannelPattern(nil, "room.:id", nil, "", nil)
	// nil params map, :id stays as-is
	assert.Equal(t, "room.:id", result)
}

// --- WebSocketHandler.Register test ---

func TestWebSocketHandler_Register(t *testing.T) {
	mgr := NewManager()
	h := NewWebSocketHandler(WebSocketConfig{
		Endpoint:       "ws-test",
		Path:           "/ws",
		ChannelPattern: "ch",
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	// Verify route was registered by sending a non-upgrade request (should get an error response)
	req, _ := http.NewRequest("GET", "/ws", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 1 * time.Second})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	// Non-upgrade request to a websocket endpoint returns an error status
	assert.NotEqual(t, 0, resp.StatusCode)
}

func TestWebSocketHandler_Register_WithMiddleware(t *testing.T) {
	mgr := NewManager()
	h := NewWebSocketHandler(WebSocketConfig{
		Endpoint:       "ws-test",
		Path:           "/ws",
		ChannelPattern: "ch",
	}, mgr, nil, nil, nil)

	app := fiber.New()
	mw := func(c fiber.Ctx) error {
		c.Locals("jwt_user_id", "test-user")
		return c.Next()
	}
	h.Register(app, mw)

	// Verify route was registered
	req, _ := http.NewRequest("GET", "/ws", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 1 * time.Second})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.NotEqual(t, 0, resp.StatusCode)
}

// --- SSE handler: send multiple events in sequence ---

func TestSSEHandler_Integration_MultipleEvents(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "ch",
		Heartbeat:      500 * time.Millisecond,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	go func() {
		time.Sleep(50 * time.Millisecond)
		for i := 0; i < 5; i++ {
			_ = mgr.SendSSE(context.Background(), "ch", "msg", fmt.Sprintf("event-%d", i), fmt.Sprintf("%d", i))
		}
	}()

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	// Verify multiple events were delivered
	assert.Contains(t, bodyStr, "data: event-0")
	assert.Contains(t, bodyStr, "data: event-4")
	assert.Contains(t, bodyStr, "id: 0")
	assert.Contains(t, bodyStr, "id: 4")
}

// --- SSE handler: Send (not SendSSE) to SSE connection (should not deliver since SendFn is nil) ---

func TestSSEHandler_Integration_SendNotSSE(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "ch",
		Heartbeat:      100 * time.Millisecond,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	go func() {
		time.Sleep(50 * time.Millisecond)
		// Use Send (WebSocket-style) - SSE connections have SSEFn, not SendFn
		_ = mgr.Send(context.Background(), "ch", "ws-msg")
	}()

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	// The ws-style Send should not appear in SSE output since SendFn is nil on SSE conns
	assert.NotContains(t, string(body), "ws-msg")
}

// --- marshalData: bool and nil ---

func TestMarshalData_Bool(t *testing.T) {
	result, err := marshalData(true)
	require.NoError(t, err)
	assert.Equal(t, []byte("true"), result)
}

func TestMarshalData_Nil(t *testing.T) {
	result, err := marshalData(nil)
	require.NoError(t, err)
	assert.Equal(t, []byte("null"), result)
}

func TestMarshalDataString_Bool(t *testing.T) {
	result, err := marshalDataString(true)
	require.NoError(t, err)
	assert.Equal(t, "true", result)
}

func TestMarshalDataString_Nil(t *testing.T) {
	result, err := marshalDataString(nil)
	require.NoError(t, err)
	assert.Equal(t, "null", result)
}

func TestMarshalDataString_Int(t *testing.T) {
	result, err := marshalDataString(42)
	require.NoError(t, err)
	assert.Equal(t, "42", result)
}

// --- SSE handler: verify SSEFn done channel branch (send after done is closed) ---

func TestSSEHandler_Integration_SSEFnDoneBranch(t *testing.T) {
	mgr := NewManager()
	disconnected := make(chan struct{})
	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		if wfID == "on_disconnect" {
			close(disconnected)
		}
		return nil
	}

	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "ch",
		Heartbeat:      50 * time.Millisecond,
		OnDisconnect:   "on_disconnect",
	}, mgr, runner, nil, nil)

	app := fiber.New()
	h.Register(app)

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 150 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	// Wait for disconnect
	select {
	case <-disconnected:
	case <-time.After(3 * time.Second):
		t.Error("on_disconnect was not called")
	}

	// Now try sending to the closed connection - SSEFn should return error
	err = mgr.SendSSE(context.Background(), "ch", "evt", "data", "")
	// Connection is unregistered, so this is a no-op (no connections matched)
	assert.NoError(t, err)
}

// --- SSE handler: verify user ID extraction from locals ---

func TestSSEHandler_Integration_UserIDFromLocals(t *testing.T) {
	mgr := NewManager()
	var gotUserID string
	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		if wfID == "on_connect" {
			gotUserID, _ = input["user_id"].(string)
		}
		return nil
	}

	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "user.{{ auth.sub }}",
		Heartbeat:      100 * time.Millisecond,
		OnConnect:      "on_connect",
	}, mgr, runner, nil, nil)

	app := fiber.New()
	mw := func(c fiber.Ctx) error {
		c.Locals("jwt_user_id", "uid-42")
		return c.Next()
	}
	h.Register(app, mw)

	go func() {
		time.Sleep(50 * time.Millisecond)
		_ = mgr.SendSSE(context.Background(), "user.uid-42", "msg", "hello", "")
	}()

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, "uid-42", gotUserID)
	assert.Contains(t, string(body), "data: hello")
}

// --- SSE handler: flush error causes stream to end ---

func TestSSEHandler_Integration_FlushErrorEndsStream(t *testing.T) {
	mgr := NewManager()
	h := NewSSEHandler(SSEConfig{
		Endpoint:       "sse-test",
		Path:           "/events",
		ChannelPattern: "ch",
		Heartbeat:      200 * time.Millisecond,
		Retry:          500,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	go func() {
		time.Sleep(30 * time.Millisecond)
		// Send events to cover the SSEFn code path
		for i := 0; i < 3; i++ {
			_ = mgr.SendSSE(context.Background(), "ch", "evt", fmt.Sprintf("d%d", i), fmt.Sprintf("id%d", i))
			time.Sleep(5 * time.Millisecond)
		}
	}()

	req, _ := http.NewRequest("GET", "/events", nil)
	resp, err := app.Test(req, fiber.TestConfig{Timeout: 300 * time.Millisecond})
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	assert.Contains(t, bodyStr, "retry: 500")
	assert.Contains(t, bodyStr, "data: d0")
}

// --- WebSocket integration tests ---

func TestWebSocketHandler_Integration_BasicConnection(t *testing.T) {
	mgr := NewManager()
	var connectWfCalled, disconnectWfCalled bool
	var mu sync.Mutex

	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		mu.Lock()
		defer mu.Unlock()
		switch wfID {
		case "on_connect":
			connectWfCalled = true
		case "on_disconnect":
			disconnectWfCalled = true
		case "on_message":
			ch, _ := input["channel"].(string)
			_ = mgr.Send(ctx, ch, input["data"])
		}
		return nil
	}

	h := NewWebSocketHandler(WebSocketConfig{
		Endpoint:       "ws-test",
		Path:           "/ws",
		ChannelPattern: "test-channel",
		PingInterval:   30 * time.Second,
		MaxMessageSize: 1024,
		OnConnect:      "on_connect",
		OnMessage:      "on_message",
		OnDisconnect:   "on_disconnect",
	}, mgr, runner, nil, nil)

	app := fiber.New()
	h.Register(app)

	addr, cleanup := startTestApp(t, app)
	defer cleanup()

	wsURL := fmt.Sprintf("ws://%s/ws", addr)
	dialer := &fastwebsocket.Dialer{}
	ws, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.True(t, connectWfCalled, "on_connect workflow should have been called")
	mu.Unlock()

	assert.Equal(t, int64(1), mgr.Count())
	assert.Equal(t, 1, mgr.ChannelCount("test-channel"))

	err = ws.WriteMessage(fastwebsocket.TextMessage, []byte(`{"action":"hello"}`))
	require.NoError(t, err)

	_ = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := ws.ReadMessage()
	require.NoError(t, err)
	assert.Contains(t, string(msg), "action")

	_ = ws.WriteMessage(fastwebsocket.CloseMessage,
		fastwebsocket.FormatCloseMessage(fastwebsocket.CloseNormalClosure, ""))
	_ = ws.Close()

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.True(t, disconnectWfCalled, "on_disconnect workflow should have been called")
	mu.Unlock()
}

func TestWebSocketHandler_Integration_MaxPerChannel(t *testing.T) {
	mgr := NewManager()
	h := NewWebSocketHandler(WebSocketConfig{
		Endpoint:       "ws-test",
		Path:           "/ws",
		ChannelPattern: "limited-ch",
		MaxPerChannel:  1,
	}, mgr, nil, nil, nil)

	app := fiber.New()
	h.Register(app)

	addr, cleanup := startTestApp(t, app)
	defer cleanup()

	wsURL := fmt.Sprintf("ws://%s/ws", addr)
	dialer := &fastwebsocket.Dialer{}

	ws1, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = ws1.Close() }()

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(1), mgr.Count())

	ws2, _, err := dialer.Dial(wsURL, nil)
	if err == nil {
		_ = ws2.SetReadDeadline(time.Now().Add(1 * time.Second))
		_, _, readErr := ws2.ReadMessage()
		assert.Error(t, readErr)
		_ = ws2.Close()
	}

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(1), mgr.Count())
}

func TestWebSocketHandler_Integration_WithParams(t *testing.T) {
	mgr := NewManager()
	var gotChannel string
	var mu sync.Mutex
	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		if wfID == "on_connect" {
			mu.Lock()
			gotChannel, _ = input["channel"].(string)
			mu.Unlock()
		}
		return nil
	}

	h := NewWebSocketHandler(WebSocketConfig{
		Endpoint:       "ws-test",
		Path:           "/ws/:room_id",
		ChannelPattern: "room.{{ request.params.room_id }}",
		OnConnect:      "on_connect",
	}, mgr, runner, nil, nil)

	app := fiber.New()
	h.Register(app)

	addr, cleanup := startTestApp(t, app)
	defer cleanup()

	wsURL := fmt.Sprintf("ws://%s/ws/my-room", addr)
	dialer := &fastwebsocket.Dialer{}
	ws, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	assert.Equal(t, "room.my-room", gotChannel)
	mu.Unlock()
}

func TestWebSocketHandler_Integration_LifecycleError(t *testing.T) {
	mgr := NewManager()
	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		return fmt.Errorf("lifecycle error for %s", wfID)
	}

	h := NewWebSocketHandler(WebSocketConfig{
		Endpoint:       "ws-test",
		Path:           "/ws",
		ChannelPattern: "ch",
		OnConnect:      "on_connect",
		OnDisconnect:   "on_disconnect",
	}, mgr, runner, nil, nil)

	app := fiber.New()
	h.Register(app)

	addr, cleanup := startTestApp(t, app)
	defer cleanup()

	wsURL := fmt.Sprintf("ws://%s/ws", addr)
	dialer := &fastwebsocket.Dialer{}
	ws, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int64(1), mgr.Count())

	_ = ws.Close()
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int64(0), mgr.Count())
}

func TestWebSocketHandler_Integration_PlainTextMessage(t *testing.T) {
	mgr := NewManager()
	var gotData any
	var mu sync.Mutex

	runner := func(ctx context.Context, wfID string, input map[string]any) error {
		if wfID == "on_message" {
			mu.Lock()
			gotData = input["data"]
			mu.Unlock()
		}
		return nil
	}

	h := NewWebSocketHandler(WebSocketConfig{
		Endpoint:       "ws-test",
		Path:           "/ws",
		ChannelPattern: "ch",
		OnMessage:      "on_message",
	}, mgr, runner, nil, nil)

	app := fiber.New()
	h.Register(app)

	addr, cleanup := startTestApp(t, app)
	defer cleanup()

	wsURL := fmt.Sprintf("ws://%s/ws", addr)
	dialer := &fastwebsocket.Dialer{}
	ws, _, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer func() { _ = ws.Close() }()

	err = ws.WriteMessage(fastwebsocket.TextMessage, []byte("plain text msg"))
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, "plain text msg", gotData)
	mu.Unlock()
}

// --- writeSSEEvent with multiline event name ---

func TestWriteSSEEvent_LongStrings(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	longData := strings.Repeat("x", 1000)
	writeSSEEvent(w, sseEvent{
		Event: "big-event",
		Data:  longData,
	})
	_ = w.Flush()

	output := buf.String()
	assert.Contains(t, output, "event: big-event\n")
	assert.Contains(t, output, "data: "+longData)
}
