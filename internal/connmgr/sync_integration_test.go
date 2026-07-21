package connmgr

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/plugins/pubsub"
	"github.com/stretchr/testify/require"
)

// TestSyncBridge_Integration_RealPubSub exercises SyncBridge against a real
// plugins/pubsub.Service (real go-redis client + wire protocol) backed by
// miniredis, instead of the in-memory fakeBus used in sync_test.go. This
// covers the one thing the fake can't: that the actual Redis PUBLISH/
// SUBSCRIBE round-trip (JSON string payloads, real channel semantics)
// behaves the same way the unit tests assume (#363).
func TestSyncBridge_Integration_RealPubSub(t *testing.T) {
	mr := miniredis.RunT(t)

	pubsubPlugin := &pubsub.Plugin{}
	svcAny, err := pubsubPlugin.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)
	svc, ok := svcAny.(*pubsub.Service)
	require.True(t, ok)

	ctx := t.Context()

	mgrA := NewManager()
	mgrB := NewManager()
	bridgeA := NewSyncBridge(svc, "instanceA", nil)
	bridgeB := NewSyncBridge(svc, "instanceB", nil)

	var muA, muB sync.Mutex
	var receivedA, receivedB []byte
	var countA atomic.Int32

	require.NoError(t, mgrA.Register(&Conn{
		ID: "a1", Channel: "room:1",
		SendFn: func(data []byte) error {
			muA.Lock()
			receivedA = data
			muA.Unlock()
			countA.Add(1)
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

	go bridgeA.Run(ctx, "chat", mgrA)
	go bridgeB.Run(ctx, "chat", mgrB)

	// Wait until both subscriptions are registered with real Redis before
	// publishing, so the message isn't lost to a subscriber that hasn't
	// attached yet.
	require.Eventually(t, func() bool {
		return mr.PubSubNumSub(syncChannelPrefix + "chat")["noda:sync:chat"] == 2
	}, 5*time.Second, 5*time.Millisecond)

	endpoint := NewEndpointService(mgrA, "chat", bridgeA)
	require.NoError(t, endpoint.Send(ctx, "room:1", map[string]any{"n": float64(1)}))

	require.Eventually(t, func() bool {
		muB.Lock()
		defer muB.Unlock()
		return receivedB != nil
	}, 5*time.Second, 5*time.Millisecond)

	muB.Lock()
	require.Equal(t, []byte(`{"n":1}`), receivedB)
	muB.Unlock()

	// self-echo must be suppressed: instanceA subscribed too (bridgeA.Run is
	// running) but must skip delivering its own envelope back into mgrA —
	// mgrA already got the message via the direct local Send inside
	// EndpointService.Send. Assert the delivery count (not just a captured
	// value, which a duplicate send would silently overwrite) is exactly 1:
	// a self-echo regression would deliver it a second time, bumping this
	// to 2.
	time.Sleep(50 * time.Millisecond)
	muA.Lock()
	require.Equal(t, []byte(`{"n":1}`), receivedA)
	muA.Unlock()
	require.Equal(t, int32(1), countA.Load())
}

// TestSyncBridge_Integration_RealPubSub_SSE covers the SSE delivery path
// (event/id preserved) over the same real Redis pubsub.Service.
func TestSyncBridge_Integration_RealPubSub_SSE(t *testing.T) {
	mr := miniredis.RunT(t)

	pubsubPlugin := &pubsub.Plugin{}
	svcAny, err := pubsubPlugin.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)
	svc, ok := svcAny.(*pubsub.Service)
	require.True(t, ok)

	ctx := t.Context()

	mgrA := NewManager()
	mgrB := NewManager()
	bridgeA := NewSyncBridge(svc, "instanceA", nil)
	bridgeB := NewSyncBridge(svc, "instanceB", nil)

	var muA, muB sync.Mutex
	var gotEvent, gotData, gotID string
	var calledB, calledA bool
	var countA atomic.Int32

	require.NoError(t, mgrA.Register(&Conn{
		ID: "a1", Channel: "feed:1",
		SSEFn: func(_, _, _ string) error {
			muA.Lock()
			calledA = true
			muA.Unlock()
			countA.Add(1)
			return nil
		},
	}))
	require.NoError(t, mgrB.Register(&Conn{
		ID: "b1", Channel: "feed:1",
		SSEFn: func(event, data, id string) error {
			muB.Lock()
			gotEvent, gotData, gotID = event, data, id
			calledB = true
			muB.Unlock()
			return nil
		},
	}))

	go bridgeA.Run(ctx, "chat", mgrA)
	go bridgeB.Run(ctx, "chat", mgrB)

	require.Eventually(t, func() bool {
		return mr.PubSubNumSub(syncChannelPrefix + "chat")["noda:sync:chat"] == 2
	}, 5*time.Second, 5*time.Millisecond)

	endpoint := NewEndpointService(mgrA, "chat", bridgeA)
	require.NoError(t, endpoint.SendSSE(ctx, "feed:1", "tick", map[string]any{"n": float64(1)}, "42"))

	require.Eventually(t, func() bool {
		muB.Lock()
		defer muB.Unlock()
		return calledB
	}, 5*time.Second, 5*time.Millisecond)

	muB.Lock()
	require.Equal(t, "tick", gotEvent)
	require.JSONEq(t, `{"n":1}`, gotData)
	require.Equal(t, "42", gotID)
	muB.Unlock()

	// mgrA's own conn received exactly the local delivery, no duplicate via
	// the bridge self-echo suppression. Assert the call count (not just the
	// captured bool, which a duplicate call would silently leave at true)
	// is exactly 1: a self-echo regression would call it a second time,
	// bumping this to 2.
	time.Sleep(50 * time.Millisecond)
	muA.Lock()
	require.True(t, calledA)
	muA.Unlock()
	require.Equal(t, int32(1), countA.Load())
}
