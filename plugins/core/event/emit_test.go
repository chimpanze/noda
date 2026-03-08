package event

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockExecCtx struct {
	resolveFunc func(expr string) (any, error)
}

func (m *mockExecCtx) Input() any         { return nil }
func (m *mockExecCtx) Auth() *api.AuthData { return nil }
func (m *mockExecCtx) Trigger() api.TriggerData {
	return api.TriggerData{Type: "test", Timestamp: time.Now(), TraceID: "test-trace"}
}
func (m *mockExecCtx) Resolve(expr string) (any, error) {
	if m.resolveFunc != nil {
		return m.resolveFunc(expr)
	}
	return expr, nil
}
func (m *mockExecCtx) ResolveWithVars(expr string, _ map[string]any) (any, error) {
	return m.Resolve(expr)
}
func (m *mockExecCtx) Log(_ string, _ string, _ map[string]any) {}

// mockStreamService implements api.StreamService for testing.
type mockStreamService struct {
	published []struct {
		topic   string
		payload any
	}
}

func (m *mockStreamService) Publish(_ context.Context, topic string, payload any) (string, error) {
	m.published = append(m.published, struct {
		topic   string
		payload any
	}{topic, payload})
	return "mock-id-1", nil
}

func (m *mockStreamService) Ack(_ context.Context, _, _, _ string) error { return nil }

// mockPubSubService implements api.PubSubService for testing.
type mockPubSubService struct {
	published []struct {
		channel string
		payload any
	}
}

func (m *mockPubSubService) Publish(_ context.Context, channel string, payload any) error {
	m.published = append(m.published, struct {
		channel string
		payload any
	}{channel, payload})
	return nil
}

func TestPlugin_Metadata(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "event", p.Name())
	assert.Equal(t, "event", p.Prefix())
	assert.False(t, p.HasServices())

	nodes := p.Nodes()
	require.Len(t, nodes, 1)
	assert.Equal(t, "emit", nodes[0].Descriptor.Name())
}

func TestEmit_ServiceDeps(t *testing.T) {
	d := &emitDescriptor{}
	deps := d.ServiceDeps()
	require.Contains(t, deps, "stream")
	require.Contains(t, deps, "pubsub")
	assert.False(t, deps["stream"].Required)
	assert.False(t, deps["pubsub"].Required)
}

func TestEmit_StreamMode(t *testing.T) {
	exec := &emitExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) { return expr, nil }}
	streamSvc := &mockStreamService{}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"mode":    "stream",
			"topic":   "user.created",
			"payload": map[string]any{"user_id": "123"},
		},
		map[string]any{"stream": streamSvc},
	)
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, "mock-id-1", result["message_id"])

	require.Len(t, streamSvc.published, 1)
	assert.Equal(t, "user.created", streamSvc.published[0].topic)
}

func TestEmit_PubSubMode(t *testing.T) {
	exec := &emitExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) { return expr, nil }}
	pubsubSvc := &mockPubSubService{}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"mode":    "pubsub",
			"topic":   "notifications",
			"payload": map[string]any{"msg": "hello"},
		},
		map[string]any{"pubsub": pubsubSvc},
	)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, true, data.(map[string]any)["ok"])

	require.Len(t, pubsubSvc.published, 1)
	assert.Equal(t, "notifications", pubsubSvc.published[0].channel)
}

func TestEmit_MissingStreamService(t *testing.T) {
	exec := &emitExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) { return expr, nil }}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"mode": "stream", "topic": "t", "payload": "p"},
		map[string]any{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stream service not configured")
}

func TestEmit_MissingPubSubService(t *testing.T) {
	exec := &emitExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) { return expr, nil }}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"mode": "pubsub", "topic": "t", "payload": "p"},
		map[string]any{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pubsub service not configured")
}

func TestEmit_UnknownMode(t *testing.T) {
	exec := &emitExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) { return expr, nil }}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"mode": "invalid", "topic": "t", "payload": "p"},
		map[string]any{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown mode")
}

func TestEmit_MissingMode(t *testing.T) {
	exec := &emitExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) { return expr, nil }}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"topic": "t", "payload": "p"},
		map[string]any{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'mode'")
}

func TestEmit_TopicExpression(t *testing.T) {
	exec := &emitExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		if expr == "{{ topic_name }}" {
			return "resolved-topic", nil
		}
		return expr, nil
	}}
	streamSvc := &mockStreamService{}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"mode":    "stream",
			"topic":   "{{ topic_name }}",
			"payload": map[string]any{"data": true},
		},
		map[string]any{"stream": streamSvc},
	)
	require.NoError(t, err)
	assert.Equal(t, "resolved-topic", streamSvc.published[0].topic)
}

func TestEmit_WithRealRedisStream(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	// Use a real stream service via the interface
	svc := &realStreamService{client: client}

	exec := &emitExecutor{}
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) { return expr, nil }}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"mode":    "stream",
			"topic":   "real-topic",
			"payload": map[string]any{"key": "value"},
		},
		map[string]any{"stream": svc},
	)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.NotEmpty(t, data.(map[string]any)["message_id"])
}

// realStreamService wraps go-redis for stream testing.
type realStreamService struct {
	client *redis.Client
}

func (s *realStreamService) Publish(ctx context.Context, topic string, payload any) (string, error) {
	return s.client.XAdd(ctx, &redis.XAddArgs{
		Stream: topic,
		Values: map[string]any{"payload": "test"},
	}).Result()
}

func (s *realStreamService) Ack(ctx context.Context, topic, group, msgID string) error {
	return s.client.XAck(ctx, topic, group, msgID).Err()
}
