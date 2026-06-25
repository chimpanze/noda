//go:build integration

package event

import (
	"context"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/testing/containers"
	pubsubplugin "github.com/chimpanze/noda/plugins/pubsub"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func emitRegistries(t *testing.T, url string) (*registry.ServiceRegistry, *registry.NodeRegistry, *redis.Client) {
	t.Helper()
	streamSvc, err := (&streamplugin.Plugin{}).CreateService(map[string]any{"url": url})
	require.NoError(t, err)
	pubsubSvc, err := (&pubsubplugin.Plugin{}).CreateService(map[string]any{"url": url})
	require.NoError(t, err)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("stream", streamSvc, nil))
	require.NoError(t, svcReg.Register("pubsub", pubsubSvc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	rc := streamSvc.(plugin.RedisClientProvider).Client()
	return svcReg, nodeReg, rc
}

func emit(t *testing.T, svcReg *registry.ServiceRegistry, nodeReg *registry.NodeRegistry,
	wf engine.WorkflowConfig) {
	t.Helper()
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
}

func TestEventEmit_Stream_Engine(t *testing.T) {
	url := containers.StartRedis(t)
	svcReg, nodeReg, rc := emitRegistries(t, url)

	wf := engine.WorkflowConfig{
		ID: "emit-stream",
		Nodes: map[string]engine.NodeConfig{
			"e": {
				Type:     "event.emit",
				Services: map[string]string{"stream": "stream", "pubsub": "pubsub"},
				Config: map[string]any{
					"mode":    "stream",
					"topic":   "orders",
					"payload": map[string]any{"id": "42"},
				},
			},
		},
	}
	emit(t, svcReg, nodeReg, wf)

	// Effect asserted directly: the message is on the stream.
	// stream.Service JSON-encodes the payload, so the raw field is a JSON string.
	msgs, err := rc.XRange(context.Background(), "orders", "-", "+").Result()
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	// The "payload" field holds the JSON-encoded map: {"id":"42"}
	payloadRaw, ok := msgs[0].Values["payload"]
	require.True(t, ok, "expected 'payload' field in stream message")
	payloadStr, ok := payloadRaw.(string)
	require.True(t, ok, "expected 'payload' field to be a string")
	assert.Contains(t, payloadStr, "42")
}

func TestEventEmit_PubSub_Engine(t *testing.T) {
	url := containers.StartRedis(t)
	svcReg, nodeReg, rc := emitRegistries(t, url)

	sub := rc.Subscribe(context.Background(), "alerts")
	defer sub.Close()
	subCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := sub.Receive(subCtx) // wait for subscription confirmation
	require.NoError(t, err)
	ch := sub.Channel()

	wf := engine.WorkflowConfig{
		ID: "emit-pubsub",
		Nodes: map[string]engine.NodeConfig{
			"e": {
				Type:     "event.emit",
				Services: map[string]string{"stream": "stream", "pubsub": "pubsub"},
				Config: map[string]any{
					"mode":    "pubsub",
					"topic":   "alerts",
					"payload": map[string]any{"level": "warn"},
				},
			},
		},
	}
	emit(t, svcReg, nodeReg, wf)

	select {
	case msg := <-ch:
		// pubsub.Service JSON-encodes the payload, so msg.Payload is {"level":"warn"}
		assert.Contains(t, msg.Payload, "warn")
	case <-time.After(5 * time.Second):
		t.Fatal("did not receive pubsub message")
	}
}

func TestEventEmit_BadMode_Engine(t *testing.T) {
	url := containers.StartRedis(t)
	svcReg, nodeReg, _ := emitRegistries(t, url)

	wf := engine.WorkflowConfig{
		ID: "emit-bad",
		Nodes: map[string]engine.NodeConfig{
			"e": {
				Type:     "event.emit",
				Services: map[string]string{"stream": "stream", "pubsub": "pubsub"},
				Config: map[string]any{
					"mode":    "carrier-pigeon",
					"topic":   "x",
					"payload": map[string]any{"a": 1},
				},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err)
}
