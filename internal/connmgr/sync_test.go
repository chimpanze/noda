package connmgr

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeBus is an in-memory implementation of api.PubSubService that mimics
// plugins/pubsub/service.go's real Redis-backed Service: Publish JSON-marshals
// the payload to a string and fans it out to every subscriber on the channel;
// Subscribe JSON-unmarshals each string into `any` (so handlers observe the
// same map[string]any shape Redis delivers) and blocks until ctx is done. A
// handler error terminates that subscription, matching the real Service.
type fakeBus struct {
	mu          sync.Mutex
	subscribers map[string][]chan string

	failPublish bool // when true, Publish always fails

	subErrOnce     bool // when true, the next Subscribe call fails once, then works
	subErrConsumed bool
}

func newFakeBus() *fakeBus {
	return &fakeBus{subscribers: make(map[string][]chan string)}
}

func (b *fakeBus) Publish(_ context.Context, channel string, payload any) error {
	b.mu.Lock()
	if b.failPublish {
		b.mu.Unlock()
		return errors.New("fakeBus: publish failed")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		b.mu.Unlock()
		return err
	}
	subs := append([]chan string(nil), b.subscribers[channel]...)
	b.mu.Unlock()

	for _, ch := range subs {
		ch <- string(data)
	}
	return nil
}

func (b *fakeBus) Subscribe(ctx context.Context, channel string, handler func(payload any) error) error {
	b.mu.Lock()
	if b.subErrOnce && !b.subErrConsumed {
		b.subErrConsumed = true
		b.mu.Unlock()
		return errors.New("fakeBus: subscribe failed")
	}
	ch := make(chan string, 16)
	b.subscribers[channel] = append(b.subscribers[channel], ch)
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		subs := b.subscribers[channel]
		for i, c := range subs {
			if c == ch {
				b.subscribers[channel] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		b.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case raw := <-ch:
			var payload any
			if err := json.Unmarshal([]byte(raw), &payload); err != nil {
				payload = raw
			}
			if err := handler(payload); err != nil {
				return err
			}
		}
	}
}

// waitFor polls cond until it returns true or the timeout elapses, failing
// the test on timeout. Used instead of a bare sleep for eventually-style
// assertions in this file.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	require.Eventually(t, cond, timeout, time.Millisecond)
}

func TestSyncBridge_WSCrossDelivery(t *testing.T) {
	bus := newFakeBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgrA := NewManager()
	mgrB := NewManager()
	bridgeA := NewSyncBridge(bus, "instanceA", nil)
	bridgeB := NewSyncBridge(bus, "instanceB", nil)

	var muA, muB sync.Mutex
	var receivedA, receivedB []byte

	require.NoError(t, mgrA.Register(&Conn{
		ID: "a1", Channel: "room:1",
		SendFn: func(data []byte) error {
			muA.Lock()
			receivedA = data
			muA.Unlock()
			return nil
		},
	}))
	require.NoError(t, mgrB.Register(&Conn{
		ID: "b1", Channel: "room:1",
		SendFn: func(data []byte) error {
			muB.Lock()
			receivedB = data
			muB.Unlock()
			return nil
		},
	}))

	go bridgeA.Run(ctx, "ws-chat", mgrA)
	go bridgeB.Run(ctx, "ws-chat", mgrB)

	// give both subscriptions a moment to register before publishing.
	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"ws-chat"]) == 2
	})

	require.NoError(t, bridgeA.Publish(ctx, "ws-chat", Envelope{
		Kind: "ws", Channel: "room:1", Payload: "hello",
	}))

	waitFor(t, time.Second, func() bool {
		muB.Lock()
		defer muB.Unlock()
		return receivedB != nil
	})
	muB.Lock()
	require.Equal(t, []byte("hello"), receivedB)
	muB.Unlock()

	// self-echo skipped: instanceA's own manager must not receive via the bridge.
	time.Sleep(20 * time.Millisecond)
	muA.Lock()
	require.Nil(t, receivedA)
	muA.Unlock()
}

func TestSyncBridge_SSECrossDelivery(t *testing.T) {
	bus := newFakeBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgrB := NewManager()
	bridgeA := NewSyncBridge(bus, "instanceA", nil)
	bridgeB := NewSyncBridge(bus, "instanceB", nil)

	var mu sync.Mutex
	var gotEvent, gotData, gotID string
	var called bool

	require.NoError(t, mgrB.Register(&Conn{
		ID: "b1", Channel: "feed:1",
		SSEFn: func(event, data, id string) error {
			mu.Lock()
			gotEvent, gotData, gotID = event, data, id
			called = true
			mu.Unlock()
			return nil
		},
	}))

	go bridgeA.Run(ctx, "sse-feed", NewManager())
	go bridgeB.Run(ctx, "sse-feed", mgrB)

	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"sse-feed"]) == 2
	})

	require.NoError(t, bridgeA.Publish(ctx, "sse-feed", Envelope{
		Kind: "sse", Channel: "feed:1", Payload: "update", Event: "tick", ID: "42",
	}))

	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return called
	})
	mu.Lock()
	require.Equal(t, "tick", gotEvent)
	require.Equal(t, "update", gotData)
	require.Equal(t, "42", gotID)
	mu.Unlock()
}

func TestSyncBridge_MalformedEnvelopeDropped(t *testing.T) {
	bus := newFakeBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgrB := NewManager()
	bridgeA := NewSyncBridge(bus, "instanceA", nil)
	bridgeB := NewSyncBridge(bus, "instanceB", nil)

	var mu sync.Mutex
	var received []byte

	require.NoError(t, mgrB.Register(&Conn{
		ID: "b1", Channel: "room:1",
		SendFn: func(data []byte) error {
			mu.Lock()
			received = data
			mu.Unlock()
			return nil
		},
	}))

	go bridgeB.Run(ctx, "ws-chat", mgrB)
	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"ws-chat"]) == 1
	})

	// A bare string round-trips through the bus as a Go string, which fails
	// to unmarshal into the Envelope struct: this is the "malformed envelope"
	// case, and it must be dropped without killing the subscription loop.
	require.NoError(t, bus.Publish(ctx, syncChannelPrefix+"ws-chat", "not-an-envelope"))

	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	require.Nil(t, received)
	mu.Unlock()

	// the loop must still be alive: the next good envelope is delivered.
	require.NoError(t, bridgeA.Publish(ctx, "ws-chat", Envelope{
		Kind: "ws", Channel: "room:1", Payload: "hello",
	}))
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received != nil
	})
	mu.Lock()
	require.Equal(t, []byte("hello"), received)
	mu.Unlock()
}

func TestSyncBridge_UnknownVersionOrKindDropped(t *testing.T) {
	bus := newFakeBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgrB := NewManager()
	bridgeB := NewSyncBridge(bus, "instanceB", nil)

	var mu sync.Mutex
	var receivedCount int

	require.NoError(t, mgrB.Register(&Conn{
		ID: "b1", Channel: "room:1",
		SendFn: func(data []byte) error {
			mu.Lock()
			receivedCount++
			mu.Unlock()
			return nil
		},
	}))

	go bridgeB.Run(ctx, "ws-chat", mgrB)
	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"ws-chat"]) == 1
	})

	// unknown version, from a different instance so it isn't skipped as self-echo.
	require.NoError(t, bus.Publish(ctx, syncChannelPrefix+"ws-chat", Envelope{
		V: 99, Instance: "instanceA", Kind: "ws", Channel: "room:1", Payload: "v99",
	}))
	// v1 is no longer accepted: only v2 envelopes are delivered.
	require.NoError(t, bus.Publish(ctx, syncChannelPrefix+"ws-chat", Envelope{
		V: 1, Instance: "instanceA", Kind: "ws", Channel: "room:1", Payload: "v1",
	}))
	// unknown kind.
	require.NoError(t, bus.Publish(ctx, syncChannelPrefix+"ws-chat", Envelope{
		V: 2, Instance: "instanceA", Kind: "carrier-pigeon", Channel: "room:1", Payload: "cp",
	}))

	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	require.Equal(t, 0, receivedCount)
	mu.Unlock()
}

func TestSyncBridge_SubscribeErrorRetriesAndRecovers(t *testing.T) {
	bus := newFakeBus()
	bus.subErrOnce = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgrB := NewManager()
	bridgeA := NewSyncBridge(bus, "instanceA", nil)
	bridgeB := NewSyncBridge(bus, "instanceB", nil)
	bridgeB.backoff = time.Millisecond

	var mu sync.Mutex
	var received []byte

	require.NoError(t, mgrB.Register(&Conn{
		ID: "b1", Channel: "room:1",
		SendFn: func(data []byte) error {
			mu.Lock()
			received = data
			mu.Unlock()
			return nil
		},
	}))

	go bridgeB.Run(ctx, "ws-chat", mgrB)

	// first Subscribe attempt fails immediately (subErrOnce); Run must retry
	// after the shortened backoff and succeed on the second attempt.
	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"ws-chat"]) == 1
	})
	bus.mu.Lock()
	require.True(t, bus.subErrConsumed)
	bus.mu.Unlock()

	require.NoError(t, bridgeA.Publish(ctx, "ws-chat", Envelope{
		Kind: "ws", Channel: "room:1", Payload: "recovered",
	}))
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received != nil
	})
	mu.Lock()
	require.Equal(t, []byte("recovered"), received)
	mu.Unlock()
}

// --- EndpointService + SyncBridge tests (#363) ---

func TestEndpointService_Send_WithBridge_PublishesAndDeliversLocally(t *testing.T) {
	bus := newFakeBus()
	bridge := NewSyncBridge(bus, "instanceA", nil)

	mgr := NewManager()
	var localReceived []byte
	require.NoError(t, mgr.Register(&Conn{
		ID: "c1", Channel: "room:1",
		SendFn: func(data []byte) error { localReceived = data; return nil },
	}))

	// A subscriber on instance B observes the published envelope.
	var published string
	sub := make(chan struct{})
	go func() {
		_ = bus.Subscribe(context.Background(), syncChannelPrefix+"ws-chat", func(payload any) error {
			raw, _ := json.Marshal(payload)
			var env Envelope
			_ = json.Unmarshal(raw, &env)
			published = env.Payload
			close(sub)
			return errors.New("stop") // end the subscribe loop after one message
		})
	}()
	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"ws-chat"]) == 1
	})

	svc := NewEndpointService(mgr, "ws-chat", bridge)
	require.NoError(t, svc.Send(context.Background(), "room:1", "hello"))

	require.Equal(t, []byte("hello"), localReceived)
	waitFor(t, time.Second, func() bool {
		select {
		case <-sub:
			return true
		default:
			return false
		}
	})
	require.Equal(t, "hello", published)
}

func TestEndpointService_Send_PublishFailure_ReturnsError(t *testing.T) {
	bus := newFakeBus()
	bus.failPublish = true
	bridge := NewSyncBridge(bus, "instanceA", nil)

	mgr := NewManager()
	var localReceived []byte
	require.NoError(t, mgr.Register(&Conn{
		ID: "c1", Channel: "room:1",
		SendFn: func(data []byte) error { localReceived = data; return nil },
	}))

	svc := NewEndpointService(mgr, "ws-chat", bridge)
	err := svc.Send(context.Background(), "room:1", "hello")
	require.Error(t, err)
	require.ErrorContains(t, err, "cross-instance sync publish")
	// local delivery already happened before the publish attempt.
	require.Equal(t, []byte("hello"), localReceived)
}

func TestEndpointService_Send_NilBridge_LocalOnly(t *testing.T) {
	mgr := NewManager()
	var localReceived []byte
	require.NoError(t, mgr.Register(&Conn{
		ID: "c1", Channel: "room:1",
		SendFn: func(data []byte) error { localReceived = data; return nil },
	}))

	svc := NewEndpointService(mgr, "ws-chat", nil)
	require.NoError(t, svc.Send(context.Background(), "room:1", "hello"))
	require.Equal(t, []byte("hello"), localReceived)
}

func TestEndpointService_SendSSE_WithBridge_PublishesAndDeliversLocally(t *testing.T) {
	bus := newFakeBus()
	bridge := NewSyncBridge(bus, "instanceA", nil)

	mgr := NewManager()
	var gotEvent, gotData, gotID string
	require.NoError(t, mgr.Register(&Conn{
		ID: "c1", Channel: "feed:1",
		SSEFn: func(event, data, id string) error {
			gotEvent, gotData, gotID = event, data, id
			return nil
		},
	}))

	var published Envelope
	sub := make(chan struct{})
	go func() {
		_ = bus.Subscribe(context.Background(), syncChannelPrefix+"sse-feed", func(payload any) error {
			raw, _ := json.Marshal(payload)
			_ = json.Unmarshal(raw, &published)
			close(sub)
			return errors.New("stop")
		})
	}()
	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"sse-feed"]) == 1
	})

	svc := NewEndpointService(mgr, "sse-feed", bridge)
	require.NoError(t, svc.SendSSE(context.Background(), "feed:1", "tick", "update", "42"))

	require.Equal(t, "tick", gotEvent)
	require.Equal(t, "update", gotData)
	require.Equal(t, "42", gotID)
	waitFor(t, time.Second, func() bool {
		select {
		case <-sub:
			return true
		default:
			return false
		}
	})
	require.Equal(t, "sse", published.Kind)
	require.Equal(t, "feed:1", published.Channel)
	require.Equal(t, "update", published.Payload)
	require.Equal(t, "tick", published.Event)
	require.Equal(t, "42", published.ID)
}

func TestEndpointService_SendSSE_PublishFailure_ReturnsError(t *testing.T) {
	bus := newFakeBus()
	bus.failPublish = true
	bridge := NewSyncBridge(bus, "instanceA", nil)

	mgr := NewManager()
	require.NoError(t, mgr.Register(&Conn{
		ID: "c1", Channel: "feed:1",
		SSEFn: func(event, data, id string) error { return nil },
	}))

	svc := NewEndpointService(mgr, "sse-feed", bridge)
	err := svc.SendSSE(context.Background(), "feed:1", "tick", "update", "42")
	require.Error(t, err)
	require.ErrorContains(t, err, "cross-instance sync publish")
}

func TestEndpointService_SendSSE_NilBridge_LocalOnly(t *testing.T) {
	mgr := NewManager()
	var gotEvent string
	require.NoError(t, mgr.Register(&Conn{
		ID: "c1", Channel: "feed:1",
		SSEFn: func(event, data, id string) error { gotEvent = event; return nil },
	}))

	svc := NewEndpointService(mgr, "sse-feed", nil)
	require.NoError(t, svc.SendSSE(context.Background(), "feed:1", "tick", "update", "42"))
	require.Equal(t, "tick", gotEvent)
}

func TestSyncBridge_CtxCancelReturns(t *testing.T) {
	bus := newFakeBus()
	ctx, cancel := context.WithCancel(context.Background())

	bridge := NewSyncBridge(bus, "instanceA", nil)
	mgr := NewManager()

	done := make(chan struct{})
	go func() {
		bridge.Run(ctx, "ws-chat", mgr)
		close(done)
	}()

	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"ws-chat"]) == 1
	})

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after ctx cancel")
	}
}

// --- Envelope v2: binary payloads (#372) ---

// TestSyncBridge_BinaryPayloadRoundTripsByteExact pins #372: a non-UTF-8
// payload (e.g. a msgpack frame pushed by a wasm guest through
// ConnectionService.Send) must survive the JSON sync envelope byte-exactly.
// json.Marshal replaces invalid UTF-8 with U+FFFD, so raw binary must ride
// base64 in a v2 envelope.
func TestSyncBridge_BinaryPayloadRoundTripsByteExact(t *testing.T) {
	binary := []byte{0x82, 0xa3, 0xfe, 0xff, 0x00, 0x01} // invalid UTF-8
	bus := newFakeBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgrA := NewManager()
	mgrB := NewManager()
	bridgeA := NewSyncBridge(bus, "instanceA", nil)
	bridgeB := NewSyncBridge(bus, "instanceB", nil)

	var mu sync.Mutex
	var receivedB []byte
	require.NoError(t, mgrB.Register(&Conn{
		ID: "b1", Channel: "room:1",
		SendFn: func(data []byte) error {
			mu.Lock()
			receivedB = append([]byte(nil), data...)
			mu.Unlock()
			return nil
		},
	}))

	go bridgeB.Run(ctx, "ws-chat", mgrB)
	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"ws-chat"]) == 1
	})

	svcA := NewEndpointService(mgrA, "ws-chat", bridgeA)
	require.NoError(t, svcA.Send(ctx, "room:1", binary))

	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return receivedB != nil
	})
	mu.Lock()
	require.Equal(t, binary, receivedB, "remote delivery must be byte-exact")
	mu.Unlock()
}

// TestSyncBridge_UTF8PayloadPlainEncoding pins the encoding rule (#372): a
// valid-UTF-8 payload publishes as a plain string (no enc field) inside
// the v2 envelope, avoiding base64 size overhead for normal text/JSON
// traffic.
func TestSyncBridge_UTF8PayloadPlainEncoding(t *testing.T) {
	bus := newFakeBus()
	bridge := NewSyncBridge(bus, "instanceA", nil)

	var published Envelope
	var publishedRaw string
	sub := make(chan struct{})
	go func() {
		_ = bus.Subscribe(context.Background(), syncChannelPrefix+"ws-chat", func(payload any) error {
			raw, _ := json.Marshal(payload)
			publishedRaw = string(raw)
			_ = json.Unmarshal(raw, &published)
			close(sub)
			return errors.New("stop")
		})
	}()
	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"ws-chat"]) == 1
	})

	require.NoError(t, bridge.Publish(context.Background(), "ws-chat", Envelope{
		Kind: "ws", Channel: "room:1", Payload: `{"a":1}`,
	}))
	waitFor(t, time.Second, func() bool {
		select {
		case <-sub:
			return true
		default:
			return false
		}
	})

	require.Equal(t, 2, published.V)
	require.Equal(t, `{"a":1}`, published.Payload)
	require.NotContains(t, publishedRaw, `"enc"`, "plain-string envelopes must not carry an enc field")
}

// TestSyncBridge_MalformedBase64Dropped: a v2 envelope whose payload fails
// base64 decoding is dropped (with the subscription loop intact), never
// delivered corrupted.
func TestSyncBridge_MalformedBase64Dropped(t *testing.T) {
	bus := newFakeBus()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgrB := NewManager()
	bridgeA := NewSyncBridge(bus, "instanceA", nil)
	bridgeB := NewSyncBridge(bus, "instanceB", nil)

	var mu sync.Mutex
	var received []byte
	require.NoError(t, mgrB.Register(&Conn{
		ID: "b1", Channel: "room:1",
		SendFn: func(data []byte) error {
			mu.Lock()
			received = data
			mu.Unlock()
			return nil
		},
	}))

	go bridgeB.Run(ctx, "ws-chat", mgrB)
	waitFor(t, time.Second, func() bool {
		bus.mu.Lock()
		defer bus.mu.Unlock()
		return len(bus.subscribers[syncChannelPrefix+"ws-chat"]) == 1
	})

	require.NoError(t, bus.Publish(ctx, syncChannelPrefix+"ws-chat", Envelope{
		V: 2, Enc: "b64", Instance: "instanceA", Kind: "ws", Channel: "room:1", Payload: "!!!not-base64!!!",
	}))

	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	require.Nil(t, received)
	mu.Unlock()

	// the loop must still be alive: the next good envelope is delivered.
	require.NoError(t, bridgeA.Publish(ctx, "ws-chat", Envelope{
		Kind: "ws", Channel: "room:1", Payload: "hello",
	}))
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return received != nil
	})
}
