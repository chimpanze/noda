package livekit

import (
	"context"
	"errors"
	"testing"
	"time"

	lkproto "github.com/livekit/protocol/livekit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBlockingEgressClient blocks on StartRoomCompositeEgress until the
// caller's context is done, then returns its error — simulating a LiveKit
// server that never responds (e.g. no worker available for egress).
type fakeBlockingEgressClient struct{}

func (f *fakeBlockingEgressClient) StartRoomCompositeEgress(ctx context.Context, req *lkproto.RoomCompositeEgressRequest) (*lkproto.EgressInfo, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (f *fakeBlockingEgressClient) StartTrackEgress(ctx context.Context, req *lkproto.TrackEgressRequest) (*lkproto.EgressInfo, error) {
	return nil, nil
}
func (f *fakeBlockingEgressClient) StopEgress(ctx context.Context, req *lkproto.StopEgressRequest) (*lkproto.EgressInfo, error) {
	return nil, nil
}
func (f *fakeBlockingEgressClient) ListEgress(ctx context.Context, req *lkproto.ListEgressRequest) (*lkproto.ListEgressResponse, error) {
	return nil, nil
}

// twirpStyleErr mimics an error returned promptly by the LiveKit server
// (e.g. "participant not found") that must pass through unchanged.
var errTwirpNotFound = errors.New("twirp error not_found: participant not found")

type fakeFastFailEgressClient struct{}

func (f *fakeFastFailEgressClient) StartRoomCompositeEgress(ctx context.Context, req *lkproto.RoomCompositeEgressRequest) (*lkproto.EgressInfo, error) {
	return nil, errTwirpNotFound
}
func (f *fakeFastFailEgressClient) StartTrackEgress(ctx context.Context, req *lkproto.TrackEgressRequest) (*lkproto.EgressInfo, error) {
	return nil, nil
}
func (f *fakeFastFailEgressClient) StopEgress(ctx context.Context, req *lkproto.StopEgressRequest) (*lkproto.EgressInfo, error) {
	return nil, nil
}
func (f *fakeFastFailEgressClient) ListEgress(ctx context.Context, req *lkproto.ListEgressRequest) (*lkproto.ListEgressResponse, error) {
	return nil, nil
}

func TestCallWithTimeout_DeadlineExceeded(t *testing.T) {
	client := &timeoutEgressClient{inner: &fakeBlockingEgressClient{}, d: 20 * time.Millisecond}

	start := time.Now()
	_, err := client.StartRoomCompositeEgress(context.Background(), &lkproto.RoomCompositeEgressRequest{})
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.Less(t, elapsed, 50*time.Millisecond, "should return promptly after the timeout, not hang")
	assert.Contains(t, err.Error(), "timed out after")
	assert.Contains(t, err.Error(), "StartRoomCompositeEgress")
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "wrapped error must satisfy errors.Is(err, context.DeadlineExceeded)")
}

func TestCallWithTimeout_FastFailPassesThroughUnchanged(t *testing.T) {
	client := &timeoutEgressClient{inner: &fakeFastFailEgressClient{}, d: 5 * time.Second}

	_, err := client.StartRoomCompositeEgress(context.Background(), &lkproto.RoomCompositeEgressRequest{})

	require.Error(t, err)
	assert.Same(t, errTwirpNotFound, err, "server errors returned before the deadline must pass through unchanged")
}

// TestCallWithTimeout_ParentCancellationNotRelabeledAsTimeout pins the
// guard in callWithTimeout that only relabels an error when the deadline
// (not the caller's own cancellation) is what ended the call. When the
// parent context is cancelled, the wrapped call must surface
// context.Canceled unchanged -- not a synthetic "timed out after" error.
func TestCallWithTimeout_ParentCancellationNotRelabeledAsTimeout(t *testing.T) {
	client := &timeoutEgressClient{inner: &fakeBlockingEgressClient{}, d: 5 * time.Second}

	parentCtx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, err := client.StartRoomCompositeEgress(parentCtx, &lkproto.RoomCompositeEgressRequest{})

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled), "parent cancellation must surface as context.Canceled")
	assert.NotContains(t, err.Error(), "timed out after", "parent cancellation must not be relabeled as a service timeout")
}

// TestCallWithTimeout_ParentDeadlineShorterNotRelabeled pins the same guard
// as TestCallWithTimeout_ParentCancellationNotRelabeledAsTimeout, but for a
// parent deadline (context.WithTimeout) rather than an explicit cancel. When
// the caller's own deadline is what expires -- shorter than the service
// timeout -- the error must surface as an unlabeled context.DeadlineExceeded,
// not a synthetic "timed out after" service-timeout error. This is the
// `ctx.Err() == nil` clause of callWithTimeout's guard: deleting it makes the
// call see tctx.Err() == context.DeadlineExceeded (derived from the expired
// parent) and relabel the error even though the *service* timeout never
// fired.
func TestCallWithTimeout_ParentDeadlineShorterNotRelabeled(t *testing.T) {
	client := &timeoutEgressClient{inner: &fakeBlockingEgressClient{}, d: 5 * time.Second}

	parentCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.StartRoomCompositeEgress(parentCtx, &lkproto.RoomCompositeEgressRequest{})

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "parent deadline expiry must surface as context.DeadlineExceeded")
	assert.NotContains(t, err.Error(), "timed out after", "a parent deadline shorter than the service timeout must not be relabeled as a service timeout")
}

func TestPlugin_CreateService_WithValidTimeout(t *testing.T) {
	p := &Plugin{}
	svc, err := p.CreateService(map[string]any{
		"url":        "wss://example.livekit.cloud",
		"api_key":    "key",
		"api_secret": "secret",
		"timeout":    "5s",
	})
	require.NoError(t, err)

	s, ok := svc.(*Service)
	require.True(t, ok)

	_, ok = s.Room.(*timeoutRoomClient)
	assert.True(t, ok, "Room client should be wrapped with a timeout")
	_, ok = s.Egress.(*timeoutEgressClient)
	assert.True(t, ok, "Egress client should be wrapped with a timeout")
	_, ok = s.Ingress.(*timeoutIngressClient)
	assert.True(t, ok, "Ingress client should be wrapped with a timeout")
}

func TestPlugin_CreateService_WithInvalidTimeout(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{
		"url":        "wss://example.livekit.cloud",
		"api_key":    "key",
		"api_secret": "secret",
		"timeout":    "nope",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid timeout")
}

func TestPlugin_CreateService_WithNonPositiveTimeout(t *testing.T) {
	cases := []struct {
		name    string
		timeout string
	}{
		{name: "negative", timeout: "-1s"},
		{name: "zero", timeout: "0s"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Plugin{}
			_, err := p.CreateService(map[string]any{
				"url":        "wss://example.livekit.cloud",
				"api_key":    "key",
				"api_secret": "secret",
				"timeout":    tc.timeout,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "timeout must be positive")
		})
	}
}

func TestPlugin_CreateService_WithoutTimeoutLeavesBareClients(t *testing.T) {
	p := &Plugin{}
	svc, err := p.CreateService(map[string]any{
		"url":        "wss://example.livekit.cloud",
		"api_key":    "key",
		"api_secret": "secret",
	})
	require.NoError(t, err)

	s, ok := svc.(*Service)
	require.True(t, ok)

	_, wrapped := s.Room.(*timeoutRoomClient)
	assert.False(t, wrapped, "Room client should not be wrapped when no timeout is configured")
	_, wrapped = s.Egress.(*timeoutEgressClient)
	assert.False(t, wrapped, "Egress client should not be wrapped when no timeout is configured")
	_, wrapped = s.Ingress.(*timeoutIngressClient)
	assert.False(t, wrapped, "Ingress client should not be wrapped when no timeout is configured")
}
