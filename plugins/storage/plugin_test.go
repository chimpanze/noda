package storage

import (
	"bytes"
	"context"
	"strings"
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

// --- WriteStream tests ---

func TestService_WriteStream(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	content := "streamed content here"
	r := strings.NewReader(content)
	n, err := svc.WriteStream(ctx, "stream.txt", r)
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)), n)

	// Verify the file was written correctly by reading it back
	data, err := svc.Read(ctx, "stream.txt")
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestService_WriteStream_LargePayload(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	// Create a payload larger than typical buffer sizes
	payload := bytes.Repeat([]byte("abcdefgh"), 8192) // 64KB
	r := bytes.NewReader(payload)
	n, err := svc.WriteStream(ctx, "large.bin", r)
	require.NoError(t, err)
	assert.Equal(t, int64(len(payload)), n)

	data, err := svc.Read(ctx, "large.bin")
	require.NoError(t, err)
	assert.Equal(t, payload, data)
}

func TestService_WriteStream_NestedPath(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	content := "nested stream"
	r := strings.NewReader(content)
	n, err := svc.WriteStream(ctx, "a/b/c/streamed.txt", r)
	require.NoError(t, err)
	assert.Equal(t, int64(len(content)), n)

	data, err := svc.Read(ctx, "a/b/c/streamed.txt")
	require.NoError(t, err)
	assert.Equal(t, content, string(data))
}

func TestService_WriteStream_EmptyReader(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	r := strings.NewReader("")
	n, err := svc.WriteStream(ctx, "empty.txt", r)
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)

	data, err := svc.Read(ctx, "empty.txt")
	require.NoError(t, err)
	assert.Empty(t, data)
}

// --- Write with nested paths ---

func TestService_Write_NestedPath(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	// Write to a deeply nested path — parent dirs should be auto-created
	require.NoError(t, svc.Write(ctx, "a/b/c/deep.txt", []byte("deep")))

	data, err := svc.Read(ctx, "a/b/c/deep.txt")
	require.NoError(t, err)
	assert.Equal(t, "deep", string(data))
}

func TestService_Write_RootPath(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	// Write at root level (dir is ".")
	require.NoError(t, svc.Write(ctx, "root.txt", []byte("at root")))

	data, err := svc.Read(ctx, "root.txt")
	require.NoError(t, err)
	assert.Equal(t, "at root", string(data))
}

func TestService_Write_Overwrite(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	require.NoError(t, svc.Write(ctx, "over.txt", []byte("first")))
	require.NoError(t, svc.Write(ctx, "over.txt", []byte("second")))

	data, err := svc.Read(ctx, "over.txt")
	require.NoError(t, err)
	assert.Equal(t, "second", string(data))
}

// --- List with subdirectories ---

func TestService_List_Subdirectories(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	require.NoError(t, svc.Write(ctx, "root/a.txt", []byte("a")))
	require.NoError(t, svc.Write(ctx, "root/sub1/b.txt", []byte("b")))
	require.NoError(t, svc.Write(ctx, "root/sub1/sub2/c.txt", []byte("c")))
	require.NoError(t, svc.Write(ctx, "root/sub3/d.txt", []byte("d")))

	paths, err := svc.List(ctx, "root")
	require.NoError(t, err)
	assert.Len(t, paths, 4)
	assert.Contains(t, paths, "root/a.txt")
	assert.Contains(t, paths, "root/sub1/b.txt")
	assert.Contains(t, paths, "root/sub1/sub2/c.txt")
	assert.Contains(t, paths, "root/sub3/d.txt")
}

func TestService_List_SubdirectoryPrefix(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	require.NoError(t, svc.Write(ctx, "root/sub1/a.txt", []byte("a")))
	require.NoError(t, svc.Write(ctx, "root/sub1/sub2/b.txt", []byte("b")))
	require.NoError(t, svc.Write(ctx, "root/sub3/c.txt", []byte("c")))

	// List only sub1
	paths, err := svc.List(ctx, "root/sub1")
	require.NoError(t, err)
	assert.Len(t, paths, 2)
	assert.Contains(t, paths, "root/sub1/a.txt")
	assert.Contains(t, paths, "root/sub1/sub2/b.txt")
}

func TestService_List_RootPrefix(t *testing.T) {
	svc := newMemService(t)
	ctx := context.Background()

	require.NoError(t, svc.Write(ctx, "top/x.txt", []byte("x")))
	require.NoError(t, svc.Write(ctx, "top/dir/y.txt", []byte("y")))

	paths, err := svc.List(ctx, "top")
	require.NoError(t, err)
	assert.Len(t, paths, 2)
	assert.Contains(t, paths, "top/x.txt")
	assert.Contains(t, paths, "top/dir/y.txt")
}

// --- Fs() accessor ---

func TestService_Fs(t *testing.T) {
	svc := newMemService(t)
	assert.NotNil(t, svc.Fs())
}

// --- Plugin metadata ---

func TestPlugin_Name(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "storage", p.Name())
}

func TestPlugin_Prefix(t *testing.T) {
	p := &Plugin{}
	assert.Equal(t, "storage", p.Prefix())
}

func TestPlugin_HasServices(t *testing.T) {
	p := &Plugin{}
	assert.True(t, p.HasServices())
}

func TestPlugin_Nodes(t *testing.T) {
	p := &Plugin{}
	assert.Nil(t, p.Nodes())
}

func TestPlugin_Shutdown(t *testing.T) {
	p := &Plugin{}
	assert.NoError(t, p.Shutdown(nil))
}

func TestPlugin_HealthCheck_InvalidType(t *testing.T) {
	p := &Plugin{}
	err := p.HealthCheck("not a service")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid service type")
}

func TestPlugin_CreateService_DefaultBackend(t *testing.T) {
	// Empty backend string should default to "local", which requires path
	p := &Plugin{}
	_, err := p.CreateService(map[string]any{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "path")
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
