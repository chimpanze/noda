package cache

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlugin_Metadata(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "cache", p.Name())
	assert.Equal(t, "cache", p.Prefix())
	assert.True(t, p.HasServices())
}

func TestPlugin_RegistersAllNodes(t *testing.T) {
	p := &Plugin{}
	nodes := p.Nodes()
	require.Len(t, nodes, 4)

	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Descriptor.Name()
	}
	assert.Contains(t, names, "get")
	assert.Contains(t, names, "set")
	assert.Contains(t, names, "del")
	assert.Contains(t, names, "exists")
}

func TestPlugin_NodeServiceDeps(t *testing.T) {
	p := &Plugin{}
	for _, node := range p.Nodes() {
		deps := node.Descriptor.ServiceDeps()
		require.Contains(t, deps, "cache", "node %s should have cache dep", node.Descriptor.Name())
		assert.Equal(t, "cache", deps["cache"].Prefix)
		assert.True(t, deps["cache"].Required)
	}
}

func TestPlugin_CreateService_MissingURL(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'url'")
}

func TestPlugin_CreateService_InvalidURL(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{"url": "not-a-redis-url"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse url")
}

func TestPlugin_CreateService_Success(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &Plugin{}

	svc, err := p.CreateService(map[string]any{
		"url": "redis://" + mr.Addr(),
	})
	require.NoError(t, err)
	require.NotNil(t, svc)

	s, ok := svc.(*Service)
	require.True(t, ok)
	assert.NotNil(t, s.client)
}

func TestPlugin_CreateService_PoolSettings(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &Plugin{}

	svc, err := p.CreateService(map[string]any{
		"url":       "redis://" + mr.Addr(),
		"pool_size": float64(20),
		"min_idle":  float64(5),
	})
	require.NoError(t, err)

	s := svc.(*Service)
	assert.Equal(t, 20, s.client.Options().PoolSize)
	assert.Equal(t, 5, s.client.Options().MinIdleConns)
}

func TestPlugin_HealthCheck_Success(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &Plugin{}

	svc, err := p.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)

	err = p.HealthCheck(svc)
	assert.NoError(t, err)
}

func TestPlugin_HealthCheck_InvalidType(t *testing.T) {
	p := &Plugin{}
	err := p.HealthCheck("not a service")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid service type")
}

func TestPlugin_Shutdown_Success(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &Plugin{}

	svc, err := p.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)

	err = p.Shutdown(svc)
	assert.NoError(t, err)

	// After shutdown, ping should fail
	s := svc.(*Service)
	assert.Error(t, s.client.Ping(context.Background()).Err())
}

func TestPlugin_Shutdown_InvalidType(t *testing.T) {
	p := &Plugin{}
	err := p.Shutdown("not a service")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid service type")
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

// TestServiceConfigSchema_PoolSizeAcceptsNumericString pins the fix for
// pool_size/min_idle: plugin.ToInt accepts numeric strings (the $env()
// substitution path always produces strings), so the schema must accept
// them too — not just JSON integers.
func TestServiceConfigSchema_PoolSizeAcceptsNumericString(t *testing.T) {
	mr := miniredis.RunT(t)
	p := &Plugin{}
	schema := p.ServiceConfigSchema()

	cfg := map[string]any{
		"url":       "redis://" + mr.Addr(),
		"pool_size": "20",
	}
	assert.Empty(t, registry.ValidateNodeConfig(schema, cfg), "schema must accept pool_size as a numeric string")

	svc, err := p.CreateService(cfg)
	require.NoError(t, err)
	s := svc.(*Service)
	assert.Equal(t, 20, s.client.Options().PoolSize)
}

func TestToInt_Variants(t *testing.T) {
	tests := []struct {
		input    any
		expected int
		ok       bool
	}{
		{float64(10), 10, true},
		{int(5), 5, true},
		{int64(3), 3, true},
		{"string", 0, false},
		{nil, 0, false},
	}
	for _, tt := range tests {
		v, ok := plugin.ToInt(tt.input)
		assert.Equal(t, tt.ok, ok)
		if ok {
			assert.Equal(t, tt.expected, v)
		}
	}
}
