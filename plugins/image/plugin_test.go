package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlugin_Name(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "image", p.Name())
}

func TestPlugin_Prefix(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "image", p.Prefix())
}

func TestPlugin_HasServices(t *testing.T) {
	p := &Plugin{}
	assert.False(t, p.HasServices())
}

func TestPlugin_Nodes(t *testing.T) {
	p := &Plugin{}
	nodes := p.Nodes()
	assert.Len(t, nodes, 5)

	names := make([]string, len(nodes))
	for i, n := range nodes {
		names[i] = n.Descriptor.Name()
	}
	assert.Contains(t, names, "resize")
	assert.Contains(t, names, "crop")
	assert.Contains(t, names, "watermark")
	assert.Contains(t, names, "convert")
	assert.Contains(t, names, "thumbnail")
}

func TestPlugin_Nodes_FactoriesCreateExecutors(t *testing.T) {
	p := &Plugin{}
	for _, node := range p.Nodes() {
		executor := node.Factory(nil)
		assert.NotNil(t, executor, "factory for %s should return non-nil executor", node.Descriptor.Name())
		assert.NotEmpty(t, executor.Outputs(), "executor for %s should have outputs", node.Descriptor.Name())
	}
}

func TestPlugin_CreateService(t *testing.T) {
	p := &Plugin{}
	svc, err := p.CreateService(nil)
	assert.NoError(t, err)
	assert.Nil(t, svc)
}

func TestPlugin_HealthCheck(t *testing.T) {
	p := &Plugin{}
	assert.NoError(t, p.HealthCheck(nil))
}

func TestPlugin_Shutdown(t *testing.T) {
	p := &Plugin{}
	assert.NoError(t, p.Shutdown(nil))
}
