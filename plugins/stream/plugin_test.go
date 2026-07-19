package stream

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/internal/registry"
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

// TestServiceConfigSchema_RequiredMatchesCreateService pins schema<->code
// agreement: a config missing "url" must fail BOTH schema validation and
// CreateService (both delegate to internal/plugin.NewRedisClient).
func TestServiceConfigSchema_RequiredMatchesCreateService(t *testing.T) {
	p := &Plugin{}
	schema := p.ServiceConfigSchema()
	require.Empty(t, registry.CheckSchemaVocabulary(schema))
	required, _ := schema["required"].([]any)
	require.Equal(t, []any{"url"}, required)

	cfg := map[string]any{}
	assert.NotEmpty(t, registry.ValidateNodeConfig(schema, cfg), "schema must reject config missing \"url\"")
	_, err := p.CreateService(cfg)
	assert.Error(t, err, "CreateService must reject config missing \"url\"")
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
