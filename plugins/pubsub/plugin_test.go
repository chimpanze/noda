package pubsub

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
	assert.Equal(t, "pubsub", p.Name())
	assert.Equal(t, "pubsub", p.Prefix())
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
	_, ok := svc.(*Service)
	assert.True(t, ok)
}

func TestPlugin_HealthCheck(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &Plugin{}
	svc, _ := p.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	assert.NoError(t, p.HealthCheck(svc))
	assert.Error(t, p.HealthCheck("wrong"))
}

func TestPlugin_Shutdown(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &Plugin{}
	svc, _ := p.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	assert.NoError(t, p.Shutdown(svc))
	assert.Error(t, p.Shutdown("wrong"))
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &Service{client: client}
}

func TestService_ImplementsPubSubService(t *testing.T) {
	var _ api.PubSubService = (*Service)(nil)
}

func TestService_PublishAndSubscribe(t *testing.T) {
	// Use a real miniredis for PubSub
	mr := miniredis.RunT(t)

	// Two separate clients needed for PubSub (one subscribes, one publishes)
	subClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	pubClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	subSvc := &Service{client: subClient}
	pubSvc := &Service{client: pubClient}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var received any
	var mu sync.Mutex
	ready := make(chan struct{})

	go func() {
		close(ready)
		_ = subSvc.Subscribe(ctx, "test-channel", func(payload any) error {
			mu.Lock()
			received = payload
			mu.Unlock()
			cancel()
			return nil
		})
	}()

	<-ready
	time.Sleep(100 * time.Millisecond)

	err := pubSvc.Publish(ctx, "test-channel", map[string]any{"event": "created"})
	if err != nil && ctx.Err() == nil {
		t.Fatalf("publish failed: %v", err)
	}

	<-ctx.Done()

	mu.Lock()
	defer mu.Unlock()
	if received != nil {
		m, ok := received.(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "created", m["event"])
	}
}

func TestService_SubscribeCancellation(t *testing.T) {
	svc := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- svc.Subscribe(ctx, "cancel-ch", func(_ any) error {
			return nil
		})
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	err := <-done
	assert.ErrorIs(t, err, context.Canceled)
}
