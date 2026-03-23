package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceRegistry_RegisterAndGet(t *testing.T) {
	reg := NewServiceRegistry()
	plugin := &stubPlugin{name: "test-db", prefix: "db"}
	instance := map[string]string{"host": "localhost"}

	err := reg.Register("main-db", instance, plugin)
	require.NoError(t, err)

	got, ok := reg.Get("main-db")
	assert.True(t, ok)
	assert.Equal(t, instance, got)
}

func TestServiceRegistry_GetPrefix(t *testing.T) {
	reg := NewServiceRegistry()
	plugin := &stubPlugin{name: "test-db", prefix: "db"}
	require.NoError(t, reg.Register("main-db", "instance", plugin))

	prefix, ok := reg.GetPrefix("main-db")
	assert.True(t, ok)
	assert.Equal(t, "db", prefix)
}

func TestServiceRegistry_GetWithPlugin(t *testing.T) {
	reg := NewServiceRegistry()
	plugin := &stubPlugin{name: "test-db", prefix: "db"}
	require.NoError(t, reg.Register("main-db", "instance", plugin))

	inst, p, ok := reg.getWithPlugin("main-db")
	assert.True(t, ok)
	assert.Equal(t, "instance", inst)
	assert.Equal(t, "test-db", p.Name())
}

func TestServiceRegistry_ByPrefix(t *testing.T) {
	reg := NewServiceRegistry()
	dbPlugin := &stubPlugin{name: "db", prefix: "db"}
	cachePlugin := &stubPlugin{name: "cache", prefix: "cache"}

	require.NoError(t, reg.Register("main-db", "db1", dbPlugin))
	require.NoError(t, reg.Register("read-db", "db2", dbPlugin))
	require.NoError(t, reg.Register("redis", "cache1", cachePlugin))

	dbServices := reg.byPrefix("db")
	assert.Len(t, dbServices, 2)

	cacheServices := reg.byPrefix("cache")
	assert.Len(t, cacheServices, 1)
}

func TestServiceRegistry_DuplicateName(t *testing.T) {
	reg := NewServiceRegistry()
	plugin := &stubPlugin{name: "db", prefix: "db"}

	require.NoError(t, reg.Register("main-db", "inst1", plugin))
	err := reg.Register("main-db", "inst2", plugin)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate")
	assert.Contains(t, err.Error(), "main-db")
}

func TestServiceRegistry_GetNonExistent(t *testing.T) {
	reg := NewServiceRegistry()

	_, ok := reg.Get("nonexistent")
	assert.False(t, ok)

	_, ok = reg.GetPrefix("nonexistent")
	assert.False(t, ok)
}

func TestServiceRegistry_All(t *testing.T) {
	reg := NewServiceRegistry()
	plugin := &stubPlugin{name: "db", prefix: "db"}

	require.NoError(t, reg.Register("a", "inst-a", plugin))
	require.NoError(t, reg.Register("b", "inst-b", plugin))

	all := reg.All()
	assert.Len(t, all, 2)
	assert.Equal(t, "inst-a", all["a"])
	assert.Equal(t, "inst-b", all["b"])
}
