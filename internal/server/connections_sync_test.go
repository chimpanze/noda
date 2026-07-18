package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/require"
)

// fakePubSub is a minimal api.PubSubService whose Subscribe blocks until ctx
// is cancelled, recording the channel it was asked to subscribe to and
// whether/why its context ended. Used to assert that StopRealtime actually
// cancels the sync subscriber (via s.syncCancel), not just stops accepting
// new connections.
type fakePubSub struct {
	mu          sync.Mutex
	subscribed  []string
	ctxErr      error
	subscribing chan struct{} // closed once Subscribe has recorded the channel
}

func newFakePubSub() *fakePubSub {
	return &fakePubSub{subscribing: make(chan struct{})}
}

func (f *fakePubSub) Publish(_ context.Context, _ string, _ any) error { return nil }

func (f *fakePubSub) Subscribe(ctx context.Context, channel string, _ func(payload any) error) error {
	f.mu.Lock()
	f.subscribed = append(f.subscribed, channel)
	f.mu.Unlock()
	select {
	case <-f.subscribing:
	default:
		close(f.subscribing)
	}
	<-ctx.Done()
	f.mu.Lock()
	f.ctxErr = ctx.Err()
	f.mu.Unlock()
	return ctx.Err()
}

func (f *fakePubSub) channels() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.subscribed...)
}

func (f *fakePubSub) contextErr() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ctxErr
}

// notAPubSub is registered as a service that does NOT implement
// api.PubSubService, to exercise the "does not implement" boot error.
type notAPubSub struct{}

func syncConnectionsConfig(endpoint, pubsubName string) map[string]map[string]any {
	return map[string]map[string]any{
		"connections/sync.json": {
			"sync": map[string]any{
				"pubsub": pubsubName,
			},
			"endpoints": map[string]any{
				endpoint: map[string]any{
					"type": "websocket",
					"path": "/ws/" + endpoint,
					"channels": map[string]any{
						"pattern": endpoint + ".general",
					},
				},
			},
		},
	}
}

func baseSyncResolvedConfig(connections map[string]map[string]any) *config.ResolvedConfig {
	return &config.ResolvedConfig{
		Root:        map[string]any{},
		Routes:      map[string]map[string]any{},
		Workflows:   map[string]map[string]any{},
		Connections: connections,
		Schemas:     map[string]map[string]any{},
	}
}

// TestRegisterConnections_SyncPubSub_ServiceNotFound covers boot error 1a:
// the connections config names a pubsub service that was never registered.
func TestRegisterConnections_SyncPubSub_ServiceNotFound(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	nodeReg := buildTestNodeRegistry()

	rc := baseSyncResolvedConfig(syncConnectionsConfig("chat", "missing-pubsub"))

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)

	err = srv.Setup()
	require.Error(t, err)
	require.ErrorContains(t, err, "missing-pubsub")
}

// TestRegisterConnections_SyncPubSub_WrongType covers boot error 1b: the
// named service exists but doesn't implement api.PubSubService.
func TestRegisterConnections_SyncPubSub_WrongType(t *testing.T) {
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("not-pubsub", &notAPubSub{}, nil))
	nodeReg := buildTestNodeRegistry()

	rc := baseSyncResolvedConfig(syncConnectionsConfig("chat", "not-pubsub"))

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)

	err = srv.Setup()
	require.Error(t, err)
	require.ErrorContains(t, err, "does not implement PubSubService")
}

// TestRegisterConnections_SyncPubSub_HappyPath_StopRealtimeCancels covers
// 1c: a valid pubsub service produces an active subscriber after Setup(),
// and StopRealtime cancels it (via s.syncCancel) rather than merely
// stopping connManagers. If StopRealtime's syncCancel() call is removed,
// the fake's Subscribe never observes ctx.Done() and this test times out /
// fails on the ctxErr assertion below.
func TestRegisterConnections_SyncPubSub_HappyPath_StopRealtimeCancels(t *testing.T) {
	bus := newFakePubSub()
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("bus", bus, nil))
	nodeReg := buildTestNodeRegistry()

	rc := baseSyncResolvedConfig(syncConnectionsConfig("chat", "bus"))

	srv, err := NewServer(rc, svcReg, nodeReg)
	require.NoError(t, err)
	require.NoError(t, srv.Setup())

	// Wait for the sync bridge's subscriber goroutine to register with the
	// fake bus before asserting on it.
	select {
	case <-bus.subscribing:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sync subscriber to start")
	}

	require.Equal(t, []string{"noda:sync:chat"}, bus.channels())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, srv.StopRealtime(ctx))

	// The fake's Subscribe call must have observed ctx cancellation — proof
	// that StopRealtime actually called s.syncCancel(), not just
	// connManagers.Stop().
	require.Eventually(t, func() bool {
		return bus.contextErr() != nil
	}, 2*time.Second, 10*time.Millisecond, "sync subscriber context was never cancelled by StopRealtime")
	require.ErrorIs(t, bus.contextErr(), context.Canceled)
}
