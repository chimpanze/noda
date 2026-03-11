package db

import (
	"testing"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlugin_Metadata(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "postgres", p.Name())
	assert.Equal(t, "db", p.Prefix())
	assert.True(t, p.HasServices())
}

func TestPlugin_RegistersAllNodes(t *testing.T) {
	p := &Plugin{}
	nodes := p.Nodes()
	require.Len(t, nodes, 9)

	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Descriptor.Name()
	}
	assert.Contains(t, names, "query")
	assert.Contains(t, names, "exec")
	assert.Contains(t, names, "create")
	assert.Contains(t, names, "update")
	assert.Contains(t, names, "delete")
	assert.Contains(t, names, "find")
	assert.Contains(t, names, "findOne")
	assert.Contains(t, names, "count")
	assert.Contains(t, names, "upsert")
}

func TestPlugin_NodeServiceDeps(t *testing.T) {
	p := &Plugin{}
	for _, node := range p.Nodes() {
		deps := node.Descriptor.ServiceDeps()
		require.Contains(t, deps, "database", "node %s should have database dep", node.Descriptor.Name())
		assert.Equal(t, "db", deps["database"].Prefix)
		assert.True(t, deps["database"].Required)
	}
}

func TestPlugin_CreateService_MissingURL(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing connection 'url'")
}

func TestPlugin_CreateService_InvalidURL(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{
		"url": "postgres://invalid:5432/nonexistent?sslmode=disable",
	})
	// GORM may fail to connect or may defer connection — we test pool settings separately
	// The important thing is it doesn't panic
	if err != nil {
		assert.Contains(t, err.Error(), "connect")
	}
}

func TestPlugin_HealthCheck_InvalidType(t *testing.T) {
	p := &Plugin{}
	err := p.HealthCheck("not a db")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid service type")
}

func TestPlugin_Shutdown_InvalidType(t *testing.T) {
	p := &Plugin{}
	err := p.Shutdown("not a db")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid service type")
}

func TestToInt(t *testing.T) {
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
