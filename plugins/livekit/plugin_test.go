package livekit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlugin_Metadata(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "livekit", p.Name())
	assert.Equal(t, "lk", p.Prefix())
	assert.True(t, p.HasServices())
}

func TestPlugin_NodeCount(t *testing.T) {
	p := &Plugin{}
	nodes := p.Nodes()
	assert.Len(t, nodes, 18)
}

func TestPlugin_AllNodesHaveServiceDeps(t *testing.T) {
	p := &Plugin{}
	for _, reg := range p.Nodes() {
		deps := reg.Descriptor.ServiceDeps()
		require.Contains(t, deps, serviceDep, "node %q missing service dep", reg.Descriptor.Name())
		dep := deps[serviceDep]
		assert.Equal(t, "lk", dep.Prefix)
		assert.True(t, dep.Required)
	}
}

func TestPlugin_CreateService_MissingURL(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{
		"api_key":    "key",
		"api_secret": "secret",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "url")
}

func TestPlugin_CreateService_MissingAPIKey(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{
		"url":        "wss://example.livekit.cloud",
		"api_secret": "secret",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_key")
}

func TestPlugin_CreateService_MissingAPISecret(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{
		"url":     "wss://example.livekit.cloud",
		"api_key": "key",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api_secret")
}

func TestPlugin_HealthCheck_WrongType(t *testing.T) {
	p := &Plugin{}
	err := p.HealthCheck("not a service")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid service type")
}

func TestPlugin_Shutdown_NoOp(t *testing.T) {
	p := &Plugin{}
	err := p.Shutdown(&Service{})
	require.NoError(t, err)
}
