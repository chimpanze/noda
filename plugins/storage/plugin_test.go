package storage

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMemService(t *testing.T) *Service {
	t.Helper()
	p := &Plugin{}
	rawSvc, err := p.CreateService(map[string]any{"backend": "memory"})
	require.NoError(t, err)
	svc, ok := rawSvc.(*Service)
	require.True(t, ok)
	return svc
}

func TestPlugin_CreateService_Memory(t *testing.T) {
	svc := newMemService(t)
	assert.NotNil(t, svc)
	assert.Equal(t, "memory", svc.backend)
}

func TestPlugin_CreateService_Local(t *testing.T) {
	dir := t.TempDir()
	p := &Plugin{}
	rawSvc, err := p.CreateService(map[string]any{"backend": "local", "path": dir})
	require.NoError(t, err)
	svc, ok := rawSvc.(*Service)
	require.True(t, ok)
	assert.Equal(t, "local", svc.backend)
}

func TestPlugin_CreateService_MissingPath(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{"backend": "local"})
	assert.Error(t, err)
}

func TestPlugin_CreateService_UnknownBackend(t *testing.T) {
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{"backend": "s3"})
	assert.Error(t, err)
}

func TestPlugin_HealthCheck(t *testing.T) {
	p := &Plugin{}
	svc := newMemService(t)
	assert.NoError(t, p.HealthCheck(svc))
}

func TestPlugin_ImplementsInterface(t *testing.T) {
	svc := newMemService(t)
	var _ api.StorageService = svc
}

func TestService_WriteReadRoundtrip(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	require.NoError(t, svc.Write(ctx, "hello.txt", []byte("hello world")))

	data, err := svc.Read(ctx, "hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestService_ReadMissingFile(t *testing.T) {
	svc := newMemService(t)
	_, err := svc.Read(context.Background(), "nonexistent.txt")
	require.Error(t, err)
	var nfe *api.NotFoundError
	require.ErrorAs(t, err, &nfe)
	assert.Equal(t, "nonexistent.txt", nfe.ID)
}

func TestService_Delete(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	require.NoError(t, svc.Write(ctx, "del.txt", []byte("bye")))
	require.NoError(t, svc.Delete(ctx, "del.txt"))

	_, err := svc.Read(ctx, "del.txt")
	assert.Error(t, err)
}

func TestService_DeleteMissing(t *testing.T) {
	svc := newMemService(t)
	err := svc.Delete(context.Background(), "missing.txt")
	require.Error(t, err)
	var nfe *api.NotFoundError
	assert.ErrorAs(t, err, &nfe)
}

func TestService_List(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	require.NoError(t, svc.Write(ctx, "docs/a.txt", []byte("a")))
	require.NoError(t, svc.Write(ctx, "docs/b.txt", []byte("b")))
	require.NoError(t, svc.Write(ctx, "other/c.txt", []byte("c")))

	paths, err := svc.List(ctx, "docs")
	require.NoError(t, err)
	assert.Len(t, paths, 2)
	assert.Contains(t, paths, "docs/a.txt")
	assert.Contains(t, paths, "docs/b.txt")
}

func TestService_ListEmpty(t *testing.T) {
	svc := newMemService(t)
	paths, err := svc.List(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, paths)
}

func TestService_LocalBackend(t *testing.T) {
	dir := t.TempDir()
	p := &Plugin{}
	rawSvc, err := p.CreateService(map[string]any{"backend": "local", "path": dir})
	require.NoError(t, err)
	svc := rawSvc.(*Service)
	ctx := context.Background()

	require.NoError(t, svc.Write(ctx, "file.txt", []byte("local content")))
	data, err := svc.Read(ctx, "file.txt")
	require.NoError(t, err)
	assert.Equal(t, "local content", string(data))
}

func TestMultipleInstances(t *testing.T) {
	p := &Plugin{}

	rawA, err := p.CreateService(map[string]any{"backend": "memory"})
	require.NoError(t, err)
	svcA := rawA.(*Service)

	rawB, err := p.CreateService(map[string]any{"backend": "memory"})
	require.NoError(t, err)
	svcB := rawB.(*Service)

	ctx := context.Background()
	require.NoError(t, svcA.Write(ctx, "a.txt", []byte("instance A")))

	// svcB should not see svcA's files
	_, err = svcB.Read(ctx, "a.txt")
	assert.Error(t, err)
}
