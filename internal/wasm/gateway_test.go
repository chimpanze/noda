package wasm

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wasm-pdk-8: Connect must reject a duplicate connection id rather than
// silently overwriting the existing gatewayConn, which would orphan its
// readLoop (it keeps running under the same id and can never be closed).
func TestGatewayConnect_RejectsDuplicateID(t *testing.T) {
	g := NewGateway(&Module{Name: "m", Codec: &jsonCodec{}, Config: ModuleConfig{AllowWS: []string{"example"}}}, testLogger())
	g.conns["c1"] = &gatewayConn{id: "c1", stopCh: make(chan struct{})} // simulate a live conn
	_, err := g.Connect(context.Background(), map[string]any{"id": "c1", "url": "ws://example/x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already in use")
	require.NotNil(t, g.conns["c1"], "existing connection must be left intact")
}

// #265: permission is checked before connection-id state — a guest probing
// with a non-whitelisted URL learns nothing about existing connection ids.
func TestGatewayConnect_WhitelistCheckedBeforeDupID(t *testing.T) {
	g := NewGateway(&Module{Name: "m", Codec: &jsonCodec{}}, testLogger()) // empty AllowWS: nothing whitelisted
	g.conns["c1"] = &gatewayConn{id: "c1", stopCh: make(chan struct{})}
	_, err := g.Connect(context.Background(), map[string]any{"id": "c1", "url": "ws://evil/x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "PERMISSION_DENIED")
	require.NotContains(t, err.Error(), "already in use")
}

// wasm-3 (sibling): heartbeatLoop must not race reconnectLoop's reassignment of
// gc.stopCh. Run under -race; without the fix the unlocked select-read of
// gc.stopCh conflicts with the locked write in reconnectLoop.
func TestHeartbeatLoop_NoRaceOnStopChReassign(t *testing.T) {
	g := &Gateway{module: &Module{Name: "t"}, logger: slog.Default()}
	gc := &gatewayConn{
		id:     "c",
		stopCh: make(chan struct{}),
		// nil HeartbeatPayload → loop never touches gc.ws; fast interval makes
		// it re-enter the select (and re-read gc.stopCh) frequently.
		config: GatewayConfig{HeartbeatInterval: 50 * time.Microsecond},
	}

	ctx, cancel := context.WithCancel(context.Background())
	hbDone := make(chan struct{})
	go func() { g.heartbeatLoop(ctx, gc); close(hbDone) }()

	// Concurrently reassign stopCh under lock, mimicking reconnectLoop.
	for i := 0; i < 200000; i++ {
		gc.mu.Lock()
		gc.stopCh = make(chan struct{})
		gc.mu.Unlock()
	}

	cancel()
	<-hbDone
}

// wasm-3: a reconnect already in its backoff window when the connection/module
// is closed must NOT re-dial and resurrect a torn-down connection.
func TestReconnectLoop_DoesNotResurrectAfterClose(t *testing.T) {
	var up websocket.Upgrader
	var srvWG sync.WaitGroup
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		srvWG.Add(1)
		go func() {
			defer srvWG.Done()
			for {
				if _, _, err := c.ReadMessage(); err != nil {
					_ = c.Close()
					return
				}
			}
		}()
	}))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	g := &Gateway{
		module: &Module{Name: "test"},
		logger: slog.Default(),
		conns:  map[string]*gatewayConn{},
	}
	gc := &gatewayConn{
		id:     "c1",
		url:    wsURL,
		stopCh: make(chan struct{}),
		config: GatewayConfig{
			Reconnect: &ReconnectConfig{Enabled: true, MaxAttempts: 3, InitialDelay: 80 * time.Millisecond},
		},
	}

	done := make(chan struct{})
	go func() { g.reconnectLoop(gc); close(done) }()

	// Simulate CloseConn/CloseAll racing in during the initial backoff sleep.
	time.Sleep(20 * time.Millisecond)
	gc.mu.Lock()
	gc.closed = true
	close(gc.stopCh)
	gc.mu.Unlock()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("reconnectLoop did not return after close")
	}

	gc.mu.Lock()
	defer gc.mu.Unlock()
	assert.True(t, gc.closed, "reconnectLoop must not resurrect a closed connection (closed flipped to false)")
	assert.False(t, gc.readLoopRunning.Load(), "reconnectLoop must not spawn a new readLoop after close")
}

// parseReconnectConfig must accept msgpack-decoded numerics: msgpack picks
// the narrowest integer type (int8/int64), not float64 like JSON. With raw
// .(float64) assertions the values silently coerce to zero and reconnection
// is disabled (#264).
func TestParseReconnectConfig_MsgpackIntegerWidths(t *testing.T) {
	rc := parseReconnectConfig(map[string]any{
		"enabled":       true,
		"max_attempts":  int8(5),
		"backoff":       "exponential",
		"initial_delay": int64(250),
	})
	assert.True(t, rc.Enabled)
	assert.Equal(t, 5, rc.MaxAttempts)
	assert.Equal(t, "exponential", rc.Backoff)
	assert.Equal(t, 250*time.Millisecond, rc.InitialDelay)
}

func TestParseReconnectConfig_JSONFloats(t *testing.T) {
	rc := parseReconnectConfig(map[string]any{
		"enabled":       true,
		"max_attempts":  float64(3),
		"initial_delay": float64(100),
	})
	assert.Equal(t, 3, rc.MaxAttempts)
	assert.Equal(t, 100*time.Millisecond, rc.InitialDelay)
}
