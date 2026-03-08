package stream

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlugin_Metadata(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "stream", p.Name())
	assert.Equal(t, "stream", p.Prefix())
	assert.True(t, p.HasServices())
	assert.Nil(t, p.Nodes())
}

func TestPlugin_CreateService_MissingURL(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'url'")
}

func TestPlugin_CreateService_Success(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &Plugin{}
	svc, err := p.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)
	require.NotNil(t, svc)
	_, ok := svc.(*Service)
	assert.True(t, ok)
}

func TestPlugin_HealthCheck(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &Plugin{}
	svc, _ := p.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	assert.NoError(t, p.HealthCheck(svc))
	assert.Error(t, p.HealthCheck("wrong type"))
}

func TestPlugin_Shutdown(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &Plugin{}
	svc, _ := p.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	assert.NoError(t, p.Shutdown(svc))
	assert.Error(t, p.Shutdown("wrong type"))
}

// --- Service tests ---

func newTestService(t *testing.T) (*Service, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &Service{client: client}, mr
}

func TestService_ImplementsStreamService(t *testing.T) {
	var _ api.StreamService = (*Service)(nil)
}

func TestService_Publish(t *testing.T) {
	svc, mr := newTestService(t)
	ctx := context.Background()

	id, err := svc.Publish(ctx, "events", map[string]any{"user": "alice"})
	require.NoError(t, err)
	assert.NotEmpty(t, id)

	// Verify message is in the stream
	assert.True(t, mr.Exists("events"))
}

func TestService_PublishAndSubscribe(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Publish a message
	_, err := svc.Publish(ctx, "test-topic", map[string]any{"msg": "hello"})
	require.NoError(t, err)

	// Subscribe and read
	subCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var received any
	var receivedID string
	var mu sync.Mutex

	go func() {
		_ = svc.Subscribe(subCtx, "test-topic", "test-group", "consumer-1", func(msgID string, payload any) error {
			mu.Lock()
			received = payload
			receivedID = msgID
			mu.Unlock()
			cancel() // stop after first message
			return nil
		})
	}()

	<-subCtx.Done()

	mu.Lock()
	defer mu.Unlock()
	require.NotNil(t, received)
	assert.NotEmpty(t, receivedID)

	m, ok := received.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hello", m["msg"])
}

func TestService_Ack(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Publish
	msgID, err := svc.Publish(ctx, "ack-topic", "data")
	require.NoError(t, err)

	// Create group and read (so message enters pending)
	svc.client.XGroupCreateMkStream(ctx, "ack-topic", "grp", "0")
	svc.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    "grp",
		Consumer: "c1",
		Streams:  []string{"ack-topic", ">"},
		Count:    1,
	})

	// Check pending
	pending, err := svc.PendingCount(ctx, "ack-topic", "grp")
	require.NoError(t, err)
	assert.Equal(t, int64(1), pending)

	// Ack
	err = svc.Ack(ctx, "ack-topic", "grp", msgID)
	require.NoError(t, err)

	// Check pending is 0
	pending, err = svc.PendingCount(ctx, "ack-topic", "grp")
	require.NoError(t, err)
	assert.Equal(t, int64(0), pending)
}

func TestService_ConsumerGroupAutoCreation(t *testing.T) {
	svc, _ := newTestService(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Subscribe auto-creates group — should not error
	go func() {
		_ = svc.Subscribe(ctx, "auto-topic", "auto-group", "c1", func(_ string, _ any) error {
			return nil
		})
	}()

	// Give it a moment to set up
	time.Sleep(100 * time.Millisecond)

	// Publish after subscription started
	_, err := svc.Publish(ctx, "auto-topic", "test")
	require.NoError(t, err)

	<-ctx.Done()
}

func TestService_SubscribeCancellation(t *testing.T) {
	svc, _ := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- svc.Subscribe(ctx, "cancel-topic", "grp", "c1", func(_ string, _ any) error {
			return nil
		})
	}()

	// Cancel immediately
	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done
	assert.ErrorIs(t, err, context.Canceled)
}
