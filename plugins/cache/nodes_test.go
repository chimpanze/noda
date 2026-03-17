package cache

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockExecCtx implements api.ExecutionContext for testing.
type mockExecCtx struct {
	resolveFunc func(expr string) (any, error)
}

func (m *mockExecCtx) Input() any          { return nil }
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

func identityResolve(expr string) (any, error) {
	return expr, nil
}

func newTestService(t *testing.T) (*Service, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return &Service{client: client}, mr
}

func testServices(svc *Service) map[string]any {
	return map[string]any{"cache": svc}
}

// --- Service tests ---

func TestService_SetAndGet(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "key1", "hello", 0)
	require.NoError(t, err)

	val, err := svc.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "hello", val)
}

func TestService_SetAndGet_ComplexValue(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	data := map[string]any{"name": "test", "count": float64(42)}
	err := svc.Set(ctx, "obj", data, 0)
	require.NoError(t, err)

	val, err := svc.Get(ctx, "obj")
	require.NoError(t, err)

	m, ok := val.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "test", m["name"])
	assert.Equal(t, float64(42), m["count"])
}

func TestService_Get_NotFound(t *testing.T) {
	svc, _ := newTestService(t)

	_, err := svc.Get(context.Background(), "nonexistent")
	require.Error(t, err)
	var notFound *api.NotFoundError
	assert.ErrorAs(t, err, &notFound)
}

func TestService_Set_WithTTL(t *testing.T) {
	svc, mr := newTestService(t)
	ctx := context.Background()

	err := svc.Set(ctx, "ttl-key", "value", 60)
	require.NoError(t, err)

	// miniredis allows checking TTL
	assert.True(t, mr.Exists("ttl-key"))
	ttl := mr.TTL("ttl-key")
	assert.Equal(t, 60*time.Second, ttl)
}

func TestService_Del(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	_ = svc.Set(ctx, "to-delete", "val", 0)
	err := svc.Del(ctx, "to-delete")
	require.NoError(t, err)

	_, err = svc.Get(ctx, "to-delete")
	require.Error(t, err)
}

func TestService_Exists(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	exists, err := svc.Exists(ctx, "no-key")
	require.NoError(t, err)
	assert.False(t, exists)

	_ = svc.Set(ctx, "yes-key", "val", 0)
	exists, err = svc.Exists(ctx, "yes-key")
	require.NoError(t, err)
	assert.True(t, exists)
}

// --- cache.get node tests ---

func TestGetNode_Success(t *testing.T) {
	svc, _ := newTestService(t)
	_ = svc.Set(context.Background(), "mykey", "myvalue", 0)

	exec := &getExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "mykey"}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)

	result := data.(map[string]any)
	assert.Equal(t, "myvalue", result["value"])
}

func TestGetNode_NotFound(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &getExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "missing"}, testServices(svc))
	require.Error(t, err)
}

func TestGetNode_MissingService(t *testing.T) {
	exec := &getExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "k"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service not configured")
}

func TestGetNode_MissingKey(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &getExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

// --- cache.set node tests ---

func TestSetNode_Success(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &setExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "k1", "value": "v1"}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, true, data.(map[string]any)["ok"])

	// Verify stored
	val, err := svc.Get(context.Background(), "k1")
	require.NoError(t, err)
	assert.Equal(t, "v1", val)
}

func TestSetNode_WithTTL(t *testing.T) {
	svc, mr := newTestService(t)

	exec := &setExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "ttl-k", "value": "v", "ttl": float64(120)}, testServices(svc))
	require.NoError(t, err)

	ttl := mr.TTL("ttl-k")
	assert.Equal(t, 120*time.Second, ttl)
}

func TestSetNode_ComplexValue(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &setExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{
			"key":   "obj-key",
			"value": map[string]any{"nested": true},
		}, testServices(svc))
	require.NoError(t, err)

	val, err := svc.Get(context.Background(), "obj-key")
	require.NoError(t, err)
	m := val.(map[string]any)
	assert.Equal(t, true, m["nested"])
}

func TestSetNode_MissingValue(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &setExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "k"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

// --- cache.del node tests ---

func TestDelNode_Success(t *testing.T) {
	svc, _ := newTestService(t)
	_ = svc.Set(context.Background(), "del-key", "val", 0)

	exec := &delExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "del-key"}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, true, data.(map[string]any)["ok"])

	// Verify deleted
	_, err = svc.Get(context.Background(), "del-key")
	require.Error(t, err)
}

func TestDelNode_NonexistentKey(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &delExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "no-such-key"}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

// --- cache.exists node tests ---

func TestExistsNode_Found(t *testing.T) {
	svc, _ := newTestService(t)
	_ = svc.Set(context.Background(), "exist-key", "val", 0)

	exec := &existsExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "exist-key"}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, true, data.(map[string]any)["exists"])
}

func TestExistsNode_NotFound(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &existsExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	output, data, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "nope"}, testServices(svc))
	require.NoError(t, err)
	assert.Equal(t, "success", output)
	assert.Equal(t, false, data.(map[string]any)["exists"])
}

// --- helpers tests ---

func TestGetCacheService_Missing(t *testing.T) {
	_, err := plugin.GetService[api.CacheService](map[string]any{}, "cache")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestGetCacheService_WrongType(t *testing.T) {
	_, err := plugin.GetService[api.CacheService](map[string]any{"cache": "not a service"}, "cache")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not implement")
}

func TestResolveString_Missing(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}
	_, err := plugin.ResolveString(nCtx, map[string]any{}, "field")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field")
}

func TestResolveAny_Expression(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return "resolved-" + expr, nil
	}}

	val, err := plugin.ResolveAny(nCtx, map[string]any{"data": "expr1"}, "data")
	require.NoError(t, err)
	assert.Equal(t, "resolved-expr1", val)
}

func TestResolveAny_NonString(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	val, err := plugin.ResolveAny(nCtx, map[string]any{"data": float64(42)}, "data")
	require.NoError(t, err)
	assert.Equal(t, float64(42), val)
}

func TestResolveInt_Variants(t *testing.T) {
	nCtx := &mockExecCtx{resolveFunc: func(expr string) (any, error) {
		return float64(99), nil
	}}

	v, err := plugin.ResolveRawInt(nCtx, float64(10))
	require.NoError(t, err)
	assert.Equal(t, 10, v)

	v, err = plugin.ResolveRawInt(nCtx, int(5))
	require.NoError(t, err)
	assert.Equal(t, 5, v)

	v, err = plugin.ResolveRawInt(nCtx, "expr")
	require.NoError(t, err)
	assert.Equal(t, 99, v)

	_, err = plugin.ResolveRawInt(nCtx, true)
	require.Error(t, err)
}

// --- Descriptor and factory tests ---

func TestDescriptors(t *testing.T) {
	descriptors := []struct {
		name        string
		d           api.NodeDescriptor
		factory     func(map[string]any) api.NodeExecutor
		outputs     []string
	}{
		{"get", &getDescriptor{}, newGetExecutor, api.DefaultOutputs()},
		{"set", &setDescriptor{}, newSetExecutor, api.DefaultOutputs()},
		{"del", &delDescriptor{}, newDelExecutor, api.DefaultOutputs()},
		{"exists", &existsDescriptor{}, newExistsExecutor, api.DefaultOutputs()},
	}
	for _, tt := range descriptors {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.name, tt.d.Name())
			assert.NotEmpty(t, tt.d.Description())
			assert.NotNil(t, tt.d.ConfigSchema())
			assert.NotNil(t, tt.d.OutputDescriptions())
			assert.Contains(t, tt.d.OutputDescriptions(), "success")

			exec := tt.factory(nil)
			require.NotNil(t, exec)
			assert.Equal(t, tt.outputs, exec.Outputs())
		})
	}
}

// --- Service.Client() test ---

func TestService_Client(t *testing.T) {
	svc, _ := newTestService(t)
	assert.NotNil(t, svc.Client())
}

// --- Service.Get non-JSON string fallback ---

func TestService_Get_NonJSONString(t *testing.T) {
	svc, mr := newTestService(t)
	// Set a raw non-JSON string directly in miniredis
	mr.Set("raw-key", "not-json-{broken")

	val, err := svc.Get(context.Background(), "raw-key")
	require.NoError(t, err)
	assert.Equal(t, "not-json-{broken", val)
}

// --- Missing service error paths ---

func TestDelNode_MissingService(t *testing.T) {
	exec := &delExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "k"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache.del")
}

func TestDelNode_MissingKey(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &delExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache.del")
}

func TestExistsNode_MissingService(t *testing.T) {
	exec := &existsExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "k"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache.exists")
}

func TestExistsNode_MissingKey(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &existsExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache.exists")
}

func TestSetNode_MissingService(t *testing.T) {
	exec := &setExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "k", "value": "v"}, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestSetNode_MissingKey(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &setExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"value": "v"}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cache.set")
}

func TestSetNode_InvalidTTL(t *testing.T) {
	svc, _ := newTestService(t)

	exec := &setExecutor{}
	nCtx := &mockExecCtx{resolveFunc: identityResolve}

	_, _, err := exec.Execute(context.Background(), nCtx,
		map[string]any{"key": "k", "value": "v", "ttl": true}, testServices(svc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ttl")
}
